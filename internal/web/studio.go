package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ---------------------------------------------------------------------------
// Studio page: AI video creation (affiliate / short film) + VN trending sounds
// ---------------------------------------------------------------------------

// studioJobView is the template/JSON projection of a studio job.
// Label/Class are rendered server-side so clients never redefine status
// labels (R2-W3: một nguồn nhãn duy nhất).
type studioJobView struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"`
	Output     string `json:"output"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at"`
	KindLabel  string `json:"kind_label"`
	Label      string `json:"label"`
	Class      string `json:"class"`
}

func kindLabel(kind string) string {
	switch kind {
	case studio.KindAffiliate:
		return "Affiliate"
	case studio.KindFilm:
		return "Phim ngắn"
	default:
		return kind
	}
}

func (s *Server) studioJobViews(jobs []studio.Job) []studioJobView {
	out := make([]studioJobView, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, studioJobView{
			ID: j.ID, Kind: j.Kind, Title: j.Title, Status: j.Status,
			Progress: j.Progress, Output: j.Output,
			CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
			KindLabel: kindLabel(j.Kind),
			Label:     statusLabel(j.Status), Class: statusClass(j.Status),
		})
	}
	return out
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// handleStudio renders the Studio page: creation forms, job list, trends.
// Tab "ai" (mặc định) là Studio AI; tab "chu" là Video chữ động (kinetic —
// trang /content cũ đã gộp vào đây, Đợt 2; job cũ trong store vẫn hiện).
func (s *Server) handleStudio(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	if tab != "chu" {
		tab = "ai"
	}
	var contentJobs []Job
	if s.Jobs != nil {
		contentJobs = s.Jobs.All()
	}
	if s.Studio == nil {
		s.render(w, "studio", s.ctx(
			"Tab", tab,
			"ContentJobs", contentJobs,
			"Error", "Studio chưa được khởi tạo."))
		return
	}
	jobs := s.studioJobViews(s.Studio.ListJobs(30))
	// Zero-touch (Đợt 3 / A6): a stale chart refreshes itself in the
	// background — nobody has to remember the refresh button.
	if studio.TrendsStale() {
		studio.RefreshVNTrendingAsync()
	}
	sounds, trendsErr := studio.FetchVNTrending(r.Context())
	activeJobs, jobCount := 0, len(jobs)
	for _, j := range jobs {
		if j.Status == "running" || j.Status == "queued" {
			activeJobs++
		}
	}
	mgOK := false
	mgKeys := 0
	if mg := s.Studio.MediaGen(); mg != nil {
		mgOK = mg.Healthy(r.Context())
		if g, ok := mg.(interface{ KeyCount() int }); ok {
			mgKeys = g.KeyCount()
		}
	}
	s.render(w, "studio", s.ctx(
		"Tab", tab,
		"ContentJobs", contentJobs,
		"Jobs", jobs, // giữ field cho tương thích: danh sách job ở /studio/jobs
		"JobCount", jobCount,
		"ActiveJobs", activeJobs,
		"Sounds", sounds,
		"SoundCount", len(sounds),
		"TopSound", topSound(sounds),
		"TrendsSource", studio.TrendsSource,
		"TrendsErr", errText(trendsErr),
		"MediaGenOK", mgOK,
		"MediaGenKeys", mgKeys,
	))
}

// topSound returns "Artist – Title" of the top-ranked trending sound.
func topSound(sounds []studio.TrendingSound) string {
	if len(sounds) == 0 {
		return ""
	}
	return sounds[0].Artist + " – " + sounds[0].Title
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// saveUpload stores one multipart file into dir, returning its path.
func saveUpload(r *http.Request, field, dir string) (string, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return "", nil // field absent is fine
	}
	defer f.Close()
	if hdr.Size == 0 {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".mp3", ".m4a", ".wav", ".ogg":
	default:
		return "", fmt.Errorf("file %s: định dạng %s không hỗ trợ", field, ext)
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), field, ext)
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(f, 50<<20)); err != nil {
		out.Close()
		return "", err
	}
	out.Close()
	return dst, nil
}

// handleStudioAffiliateCreate queues an affiliate video job.
func (s *Server) handleStudioAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.fail(w, fmt.Errorf("studio nil"), "studio affiliate")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		s.fail(w, err, "parse affiliate form")
		return
	}
	upDir := s.Studio.UploadDir()
	modelPhoto, err := saveUpload(r, "model_photo", upDir)
	if err != nil {
		s.fail(w, err, "upload model photo")
		return
	}
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		s.fail(w, err, "upload product photo")
		return
	}
	musicPath, err := saveUpload(r, "music_file", upDir)
	if err != nil {
		s.fail(w, err, "upload music")
		return
	}
	// Optional: direct audio URL for the trending sound.
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr != nil {
				s.fail(w, derr, "download music")
				return
			}
			musicPath = dst
		}
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)
	p := studio.AffiliateParams{
		Mode:         r.FormValue("mode"),
		Niche:        strings.TrimSpace(r.FormValue("niche")),
		ProductName:  strings.TrimSpace(r.FormValue("product_name")),
		ModelPhoto:   modelPhoto,
		ProductPhoto: productPhoto,
		Seconds:      seconds,
		BPM:          bpm,
		MusicPath:    musicPath,
		MusicStart:   musicStart,
		MusicTitle:   strings.TrimSpace(r.FormValue("music_title")),
		MusicArtist:  strings.TrimSpace(r.FormValue("music_artist")),
	}
	if p.ProductName == "" {
		p.ProductName = "Sản phẩm"
	}
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		s.fail(w, err, "create affiliate job")
		return
	}
	http.Redirect(w, r, "/studio/jobs", http.StatusSeeOther)
}

// handleStudioFilmCreate queues a short-film job.
func (s *Server) handleStudioFilmCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.fail(w, fmt.Errorf("studio nil"), "studio film")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse film form")
		return
	}
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" {
		http.Redirect(w, r, "/studio", http.StatusSeeOther)
		return
	}
	seconds, _ := strconv.Atoi(r.PostFormValue("seconds"))
	if _, err := s.Studio.CreateFilmJob(studio.FilmParams{Topic: topic, Seconds: seconds}); err != nil {
		s.fail(w, err, "create film job")
		return
	}
	http.Redirect(w, r, "/studio/jobs", http.StatusSeeOther)
}

// handleStudioJobs serves the job list: HTML page for deep links
// (R2-W3), JSON for polling when Accept: application/json.
func (s *Server) handleStudioJobs(w http.ResponseWriter, r *http.Request) {
	var jobs []studioJobView
	if s.Studio != nil {
		jobs = s.studioJobViews(s.Studio.ListJobs(30))
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{"jobs": jobs})
		return
	}
	s.render(w, "studio_jobs", s.ctx(
		"Title", "Job Studio", "Path", "/studio/jobs", "Jobs", jobs,
	))
}

// studioAssetView projects a storyboard asset for JSON.
// Label/Class are server-rendered so clients never redefine them (R2-W3).
type studioAssetView struct {
	ID      int64  `json:"id"`
	Idx     int    `json:"idx"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Prompt  string `json:"prompt"`
	Preview string `json:"preview"`
	Label   string `json:"label"`
	Class   string `json:"class"`
}

// handleStudioJobDetail returns one job + its storyboard assets as JSON.
func (s *Server) handleStudioJobDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	assets := s.Studio.ListAssets(id)
	views := make([]studioAssetView, 0, len(assets))
	for _, a := range assets {
		v := studioAssetView{ID: a.ID, Idx: a.Idx, Kind: a.Kind, Status: a.Status, Prompt: a.Prompt,
			Label: statusLabel(a.Status), Class: statusClass(a.Status)}
		if a.Status == "done" && a.Path != "" {
			v.Preview = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
		}
		views = append(views, v)
	}
	s.writeJSON(w, map[string]any{
		"job":    j,
		"assets": views,
	})
}

var studioAssetRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4)$`)

// handleStudioAsset serves storyboard asset files (photos/clips) with a
// strict allowlist: job id prefix + safe file name, contained in the studio
// work dir.
func (s *Server) handleStudioAsset(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	job := r.PathValue("job")
	file := r.PathValue("file")
	if !studioAssetRe.MatchString(file) || strings.Contains(job, "..") || strings.Contains(job, "/") {
		http.NotFound(w, r)
		return
	}
	// The job id is a unix-nano timestamp; be lenient but safe.
	for _, c := range job {
		if c < '0' || c > '9' {
			http.NotFound(w, r)
			return
		}
	}
	path := filepath.Join(s.Studio.WorkDir(), job, file)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.Studio.WorkDir())) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

// handleStudioTrends serves the VN trending chart: HTML page (R2-W3),
// JSON when Accept: application/json.
func (s *Server) handleStudioTrends(w http.ResponseWriter, r *http.Request) {
	sounds, err := studio.FetchVNTrending(r.Context())
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source": studio.TrendsSource,
			"sounds": sounds,
			"error":  errText(err),
		})
		return
	}
	s.render(w, "studio_trends", s.ctx(
		"Title", "Nhạc thịnh hành", "Path", "/studio/trends",
		"Sounds", sounds, "TrendsSource", studio.TrendsSource, "TrendsErr", errText(err),
	))
}

// handleStudioTrendsRefresh forces a chart refresh.
func (s *Server) handleStudioTrendsRefresh(w http.ResponseWriter, r *http.Request) {
	_, _ = studio.RefreshVNTrending(r.Context())
	http.Redirect(w, r, "/studio/trends", http.StatusSeeOther)
}

// handleStudioMediaGenHealth reports media-gen provider health as JSON.
func (s *Server) handleStudioMediaGenHealth(w http.ResponseWriter, r *http.Request) {
	healthy, keys, name := false, 0, ""
	if s.Studio != nil {
		if mg := s.Studio.MediaGen(); mg != nil {
			name = mg.Name()
			healthy = mg.Healthy(r.Context())
			if g, ok := mg.(interface{ KeyCount() int }); ok {
				keys = g.KeyCount()
			}
		}
	}
	s.writeJSON(w, map[string]any{
		"provider": name, "healthy": healthy, "keys": keys,
	})
}

var mediaNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.mp4$`)

// handleStudioKineticCreate queues one kinetic-typography render from the
// Studio "Video chữ động" tab (trang /content cũ đã gộp vào đây — Đợt 2).
// Old jobs stay in the same JSON store, so videos created before the merge
// remain listed and playable under the tab.
func (s *Server) handleStudioKineticCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse kinetic form")
		return
	}
	var captions []string
	for _, c := range strings.Split(r.PostFormValue("captions"), "\n") {
		if c = strings.TrimSpace(c); c != "" {
			captions = append(captions, c)
		}
	}
	job := Job{
		ID:        NewJobID(),
		Title:     strings.TrimSpace(r.PostFormValue("title")),
		Captions:  captions,
		Narration: strings.TrimSpace(r.PostFormValue("narration")),
		Status:    "queued",
		CreatedAt: s.nowISO(),
		Log:       "Đang chờ…",
	}
	if job.Title == "" {
		seeOther(w, r, "/studio?tab=chu")
		return
	}
	s.Jobs.Add(job)
	go s.runJob(job.ID) // never blocks the dashboard; panics are contained
	seeOther(w, r, "/studio?tab=chu")
}

// runJob renders one content job in the background and records the
// decision. It never crashes the dashboard.
func (s *Server) runJob(jobID string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("web: job %s panic: %v", jobID, rec)
			s.Jobs.Update(jobID, func(j *Job) {
				j.Status = "failed"
				j.FinishedAt = s.nowISO()
			})
		}
	}()
	job, ok := s.Jobs.Get(jobID)
	if !ok {
		return
	}
	s.Jobs.Update(jobID, func(j *Job) {
		j.Status = "running"
		j.Log = "Bắt đầu dựng video…"
	})
	ctx := context.Background()
	res, err := s.renderer.Render(ctx, job, s.TTS, func(line string) {
		s.Jobs.AppendLog(jobID, line)
	})
	if err != nil {
		log.Printf("web: render job %s: %v", jobID, err)
		s.Jobs.Update(jobID, func(j *Job) {
			j.Status = "failed"
			j.FinishedAt = s.nowISO()
		})
		s.Jobs.AppendLog(jobID, fmt.Sprintf("Lỗi: %v", err))
		return
	}
	title := job.Title
	if err := s.Ledger.Decide("content", "video_rendered", &title,
		fmt.Sprintf("kinetic video %.1fs", res.Seconds),
		map[string]any{"job": job.ID, "voice": res.HasAudio}); err != nil {
		log.Printf("web: decide video_rendered: %v", err)
	}
	s.Jobs.Update(jobID, func(j *Job) {
		j.Status = "done"
		j.Output = res.Output
		j.FinishedAt = s.nowISO()
	})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Strict allowlist: a single path segment, .mp4 only. Anything else
	// (including "..") is a 404.
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, s.OutDir+"/"+name)
}

// --------------------------------------------------------------- publishers

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ---------------------------------------------------------------------------
// Studio page: AI video creation (affiliate) + VN trending sounds
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
	default:
		return kind
	}
}

func (s *Server) studioJobViews(jobs []studio.Job) []studioJobView {
	out := make([]studioJobView, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, studioJobView{
			ID: j.ID, Kind: j.Kind, Title: j.Title, Status: j.Status,
			Progress: j.Progress, Output: mediaName(j.Output),
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

// handleStudio renders the Studio page: creation form, job links, trends.
// Đợt AK: tab "Video chữ động" (kinetic) đã gỡ — trái format affiliate đã
// chốt (ảnh + nhạc, không chữ) và không tự động hóa nào dùng.
func (s *Server) handleStudio(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.render(w, "studio", s.ctx(
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
	if mg := s.Studio.MediaGen(); mg != nil {
		mgOK = mg.Healthy(r.Context())
	}
	s.render(w, "studio", s.ctx(
		"Jobs", jobs, // giữ field cho tương thích: danh sách job ở /studio/jobs
		"JobCount", jobCount,
		"ActiveJobs", activeJobs,
		"Sounds", sounds,
		"TopSound", topSound(sounds),
		"TrendsSource", studio.TrendsSource,
		"TrendsErr", errText(trendsErr),
		"MediaGenOK", mgOK,
		"Error", r.URL.Query().Get("err"),
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
	// Đợt AC: báo rõ khi file quá lớn thay vì cắt im lặng 50MB.
	if hdr.Size > 50<<20 {
		return "", fmt.Errorf("file %s quá lớn (%.1fMB, tối đa 50MB)", field, float64(hdr.Size)/(1<<20))
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

// saveUploads lưu tối đa max ảnh từ 1 ô chọn nhiều file (chỉ nhận ảnh).
func saveUploads(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	var out []string
	for i, hdr := range r.MultipartForm.File[field] {
		if i >= max {
			break
		}
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > 50<<20 {
			return nil, fmt.Errorf("ảnh %s quá lớn (tối đa 50MB)", hdr.Filename)
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			return nil, fmt.Errorf("ảnh %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		f, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, fmt.Sprintf("%d-%s-%d%s", time.Now().UnixNano(), field, i, ext))
		out2, err := os.Create(dst)
		if err != nil {
			f.Close()
			return nil, err
		}
		_, err = io.Copy(out2, f)
		out2.Close()
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, nil
}

// handleStudioAffiliateCreate queues an affiliate video job.
func (s *Server) handleStudioAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		studioErr("Không đọc được form — thử lại.")
		return
	}
	upDir := s.Studio.UploadDir()
	modelPhoto, err := saveUpload(r, "model_photo", upDir)
	if err != nil {
		studioErr("Ảnh mẫu lỗi: " + err.Error())
		return
	}
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 6)
	if err != nil {
		studioErr("Ảnh thêm lỗi: " + err.Error())
		return
	}
	musicPath, err := saveUpload(r, "music_file", upDir)
	if err != nil {
		studioErr("File nhạc lỗi: " + err.Error())
		return
	}
	// Optional: direct audio URL for the trending sound.
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr != nil {
				studioErr("Không tải được nhạc từ link: " + derr.Error())
				return
			}
			musicPath = dst
		}
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)
	// Đợt AC: clamp biên server-side (template có min/max nhưng POST tay
	// bypass được; nhập chữ → Atoi trả 0).
	seconds = clampInt(seconds, 10, 120)
	bpm = clampInt(bpm, 60, 200)
	if musicStart < 0 {
		musicStart = 0
	}
	p := studio.AffiliateParams{
		Mode:         r.FormValue("mode"),
		Niche:        strings.TrimSpace(r.FormValue("niche")),
		ProductName:  strings.TrimSpace(r.FormValue("product_name")),
		ModelPhoto:   modelPhoto,
		ProductPhoto: productPhoto,
		ExtraPhotos:  extraPhotos,
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
		studioErr("Không tạo được job: " + err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs")
}

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
		"Msg", r.URL.Query().Get("msg"), "Err", r.URL.Query().Get("err"),
	))
}

// handleStudioJobCancel cancels a running/queued job.
func (s *Server) handleStudioJobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio != nil {
		s.Studio.CancelRunning(id)
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobDelete removes a job and all its files.
func (s *Server) handleStudioJobDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	if err := s.Studio.DeleteJob(id); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không xoá được job: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobRetry creates a new affiliate job from an old job's params.
func (s *Server) handleStudioJobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	j, ok := s.Studio.GetJob(id)
	if !ok {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy job "+id))
		return
	}
	if j.Kind != studio.KindAffiliate {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Chỉ thử lại được job affiliate"))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không đọc được tham số job cũ"))
		return
	}
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job mới: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
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
	resp := map[string]any{
		"job":    j,
		"assets": views,
	}
	// Checklist lên xu hướng cho video affiliate đã xong (đo từ file thật).
	if j.Kind == studio.KindAffiliate && j.Status == studio.StatusDone && j.Output != "" {
		var p studio.AffiliateParams
		_ = json.Unmarshal([]byte(j.Params), &p)
		outPath := j.Output
		if s.Studio != nil {
			outPath = s.Studio.OutputPath(j.Output)
		}
		resp["trend"] = studio.TrendReadiness(r.Context(), outPath, p, j.Caption)
	}
	s.writeJSON(w, resp)
}

var studioAssetRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4)$`)

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
	serveMediaFile(w, r, path)
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

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Strict allowlist: a single path segment, .mp4 only. Anything else
	// (including "..") is a 404.
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, filepath.Join(s.OutDir, name))
}

// --------------------------------------------------------------- publishers

// capcutExportRoot là nơi xuất gói chỉnh sửa (~/Movies/AI Creator OS).
// Biến để test trỏ về thư mục tạm.
var capcutExportRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Movies", "AI Creator OS")
}

// openOnMac chạy lệnh `open` của macOS (seam cho test).
var openOnMac = func(args ...string) error { return exec.Command("open", args...).Run() }

// handleStudioJobCapCut xuất gói chỉnh sửa, mở Finder tới thư mục và bật
// CapCut nếu máy có (CapCut không có API — người dùng kéo thả file vào).
func (s *Server) handleStudioJobCapCut(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	dir, err := s.Studio.ExportForEditing(r.PathValue("id"), capcutExportRoot())
	if err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Xuất cho CapCut lỗi: "+err.Error()))
		return
	}
	_ = openOnMac("-R", dir)
	msg := "Đã xuất vào " + dir + " — kéo các file vào timeline CapCut."
	if openOnMac("-a", "CapCut") != nil {
		msg += " (Chưa thấy app CapCut — cài từ capcut.com rồi mở thư mục này.)"
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape(msg))
}

// mediaName trả tên file để phát qua /media/{name} (route chỉ nhận 1
// đoạn tên). j.Output là đường dẫn đầy đủ — dùng thẳng sẽ ra URL hỏng.
func mediaName(output string) string {
	if output == "" {
		return ""
	}
	return filepath.Base(output)
}

// handleStudioJobRemix dựng lại 1 video affiliate với tham số đã sửa
// (thời lượng, nhịp, đoạn nhạc, file nhạc, thứ tự ảnh, chú thích). Bản cũ
// giữ nguyên — đây là "chỉnh sửa" trong app vì CapCut không có API.
func (s *Server) handleStudioJobRemix(w http.ResponseWriter, r *http.Request) {
	back := func(q string) { seeOther(w, r, "/studio/jobs?"+q) }
	if s.Studio == nil {
		back("err=" + url.QueryEscape("Studio chưa sẵn sàng."))
		return
	}
	j, ok := s.Studio.GetJob(r.PathValue("id"))
	if !ok || j.Kind != studio.KindAffiliate {
		back("err=" + url.QueryEscape("Chỉ chỉnh sửa được video affiliate đã có."))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		back("err=" + url.QueryEscape("Không đọc được tham số video cũ."))
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		back("err=" + url.QueryEscape("Không đọc được form — thử lại."))
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds"))); err == nil {
		p.Seconds = clampInt(v, 10, 120)
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("bpm"))); err == nil {
		p.BPM = clampInt(v, 60, 200)
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("music_start")), 64); err == nil && v >= 0 {
		p.MusicStart = v
	}
	music, err := saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		back("err=" + url.QueryEscape("File nhạc lỗi: "+err.Error()))
		return
	}
	if music != "" {
		p.MusicPath = music
	}
	p = reorderPhotos(p, r.FormValue("order"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("caption"))
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		back("err=" + url.QueryEscape("Không dựng lại được: "+err.Error()))
		return
	}
	back("msg=" + url.QueryEscape("Đang dựng bản chỉnh sửa — bản cũ vẫn giữ để so sánh."))
}

// reorderPhotos đổi thứ tự ảnh sản phẩm + ảnh thêm (ảnh người mẫu giữ
// nguyên vì là ảnh khóa khuôn mặt). order: "reverse" | "rotate" | "".
func reorderPhotos(p studio.AffiliateParams, order string) studio.AffiliateParams {
	var all []string
	if p.ProductPhoto != "" {
		all = append(all, p.ProductPhoto)
	}
	all = append(all, p.ExtraPhotos...)
	if len(all) < 2 {
		return p
	}
	switch order {
	case "reverse":
		for i, k := 0, len(all)-1; i < k; i, k = i+1, k-1 {
			all[i], all[k] = all[k], all[i]
		}
	case "rotate":
		all = append(all[1:], all[0])
	default:
		return p
	}
	p.ProductPhoto, p.ExtraPhotos = all[0], append([]string(nil), all[1:]...)
	return p
}

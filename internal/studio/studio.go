package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)

	"github.com/ninhlee99/ai-creator-os/internal/engines"
)

// LLM is the text-generation backend for the director. It matches
// engines.LLMProvider so the chain can be passed directly.
type LLM interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
	Name() string
	Healthy(ctx context.Context) bool
}

// Narrator synthesizes narration WAV bytes (TTS chain), or nil for silent.
type Narrator func(ctx context.Context, text string) ([]byte, error)

// Job kinds.
const (
	KindAffiliate = "affiliate"
	KindFilm      = "film"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// AffiliateModePhoto = list ảnh + nhạc (format Ninh đã chốt cho thời
// trang/phụ kiện). AffiliateModeShots = nhiều cảnh video nối nhau.
const (
	AffiliateModePhoto = "photo"
	AffiliateModeShots = "shots"
)

// AffiliateParams describes one affiliate-video job.
type AffiliateParams struct {
	Mode         string `json:"mode"` // photo | shots
	Niche        string `json:"niche"`
	ProductName  string `json:"product_name"`
	ModelPhoto   string `json:"model_photo"`   // local path (identity lock)
	ProductPhoto string `json:"product_photo"` // local path (product lock)
	Seconds      int    `json:"seconds"`
	MusicPath    string `json:"music_path"`  // local audio file, "" = silent
	MusicStart   float64 `json:"music_start"` // seconds into the track
	MusicTitle   string `json:"music_title"`
	MusicArtist  string `json:"music_artist"`
}

// FilmParams describes one short-film job.
type FilmParams struct {
	Topic   string `json:"topic"`
	Seconds int    `json:"seconds"`
}

// Job is one studio render job.
type Job struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Progress   int    `json:"progress"` // 0-100
	Log        string `json:"log"`
	Output     string `json:"output"` // final file, "" until done
	Params     string `json:"params"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at"`
}

// Asset is one generated photo/clip inside a job (the storyboard row).
type Asset struct {
	ID     int64  `json:"id"`
	JobID  string `json:"job_id"`
	Idx    int    `json:"idx"`
	Kind   string `json:"kind"` // photo | clip
	Path   string `json:"path"`
	Prompt string `json:"prompt"`
	Status string `json:"status"` // queued|running|done|failed
}

// Studio orchestrates creation jobs: director -> shoot -> assemble ->
// review. Jobs run in background goroutines; state lives in SQLite so the
// dashboard can poll progress.
type Studio struct {
	db       *sql.DB
	mu       sync.Mutex
	llm      LLM
	mg       MediaGen
	narrator Narrator
	outDir   string
	workRoot string
	running  map[string]context.CancelFunc
}

// New opens (or creates) the studio database and returns the orchestrator.
// outDir is where final videos land (served at /media/).
func New(dbPath string, llm LLM, mg MediaGen, narrator Narrator, outDir string) (*Studio, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("studio db: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	s := &Studio{
		db:       db,
		llm:      llm,
		mg:       mg,
		narrator: narrator,
		outDir:   outDir,
		workRoot: filepath.Join(outDir, "studio"),
		running:  map[string]context.CancelFunc{},
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	for _, d := range []string{s.workRoot, filepath.Join(s.workRoot, "uploads")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Studio) migrate() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS studio_jobs(
		id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued', progress INTEGER NOT NULL DEFAULT 0,
		log TEXT NOT NULL DEFAULT '', output TEXT NOT NULL DEFAULT '',
		params TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '');
	CREATE TABLE IF NOT EXISTS studio_assets(
		id INTEGER PRIMARY KEY AUTOINCREMENT, job_id TEXT NOT NULL,
		idx INTEGER NOT NULL, kind TEXT NOT NULL,
		path TEXT NOT NULL DEFAULT '', prompt TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'queued');
	CREATE INDEX IF NOT EXISTS idx_studio_assets_job ON studio_assets(job_id);`)
	return err
}

// Close releases the database handle.
func (s *Studio) Close() error { return s.db.Close() }

// UploadDir is where the web layer saves uploaded model/product photos and
// music files.
func (s *Studio) UploadDir() string { return filepath.Join(s.workRoot, "uploads") }

// WorkDir is the per-job work root (storyboard assets live under it).
func (s *Studio) WorkDir() string { return s.workRoot }

// MediaGen exposes the provider (dashboard health card).
func (s *Studio) MediaGen() MediaGen { return s.mg }

// ---------------------------------------------------------------------------
// job bookkeeping
// ---------------------------------------------------------------------------

func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Studio) insertJob(kind, title string, params any) (string, error) {
	pj, _ := json.Marshal(params)
	id := newID()
	_, err := s.db.Exec(
		`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		id, kind, title, StatusQueued, string(pj), time.Now().Format("2006-01-02T15:04:05"))
	return id, err
}

func (s *Studio) setStatus(id, status string, progress int) {
	fin := ""
	if status == StatusDone || status == StatusFailed {
		fin = time.Now().Format("2006-01-02T15:04:05")
	}
	s.db.Exec(`UPDATE studio_jobs SET status=?, progress=?, finished_at=CASE WHEN ? != '' THEN ? ELSE finished_at END WHERE id=?`,
		status, progress, fin, fin, id)
}

func (s *Studio) setOutput(id, output string) {
	s.db.Exec(`UPDATE studio_jobs SET output=? WHERE id=?`, output, id)
}

func (s *Studio) appendLog(id, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur string
	_ = s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&cur)
	if cur != "" {
		cur += "\n"
	}
	cur += time.Now().Format("15:04:05") + " " + line
	s.db.Exec(`UPDATE studio_jobs SET log=? WHERE id=?`, cur, id)
}

func (s *Studio) addAsset(jobID string, idx int, kind, prompt string) int64 {
	res, _ := s.db.Exec(`INSERT INTO studio_assets(job_id,idx,kind,prompt,status) VALUES(?,?,?,?,?)`,
		jobID, idx, kind, prompt, StatusQueued)
	id, _ := res.LastInsertId()
	return id
}

func (s *Studio) setAsset(id int64, status, path string) {
	s.db.Exec(`UPDATE studio_assets SET status=?, path=? WHERE id=?`, status, path, id)
}

// GetJob returns one job.
func (s *Studio) GetJob(id string) (Job, bool) {
	var j Job
	err := s.db.QueryRow(`SELECT id,kind,title,status,progress,log,output,params,created_at,finished_at FROM studio_jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Log, &j.Output, &j.Params, &j.CreatedAt, &j.FinishedAt)
	return j, err == nil
}

// ListJobs returns jobs newest first.
func (s *Studio) ListJobs(limit int) []Job {
	rows, err := s.db.Query(`SELECT id,kind,title,status,progress,output,created_at,finished_at FROM studio_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Output, &j.CreatedAt, &j.FinishedAt); err == nil {
			out = append(out, j)
		}
	}
	return out
}

// ListAssets returns a job's storyboard assets in order.
func (s *Studio) ListAssets(jobID string) []Asset {
	rows, err := s.db.Query(`SELECT id,job_id,idx,kind,path,prompt,status FROM studio_assets WHERE job_id=? ORDER BY idx`, jobID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.JobID, &a.Idx, &a.Kind, &a.Path, &a.Prompt, &a.Status); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// affiliate jobs
// ---------------------------------------------------------------------------

// CreateAffiliateJob queues an affiliate video job and starts it.
func (s *Studio) CreateAffiliateJob(p AffiliateParams) (string, error) {
	if p.Seconds <= 0 {
		p.Seconds = 30
	}
	if p.Mode == "" {
		p.Mode = AffiliateModePhoto
	}
	title := p.ProductName
	if title == "" {
		title = "Video affiliate"
	}
	id, err := s.insertJob(KindAffiliate, title, p)
	if err != nil {
		return "", err
	}
	go s.runAffiliate(id, p)
	return id, nil
}

// directorPhotoPlan asks the LLM for the photo list (the storyboard).
func (s *Studio) directorPhotoPlan(ctx context.Context, p AffiliateParams) ([]string, error) {
	n := p.Seconds / 5
	if n < 4 {
		n = 4
	}
	if n > 8 {
		n = 8
	}
	sys := "Bạn là đạo diễn ảnh thời trang TikTok Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Sản phẩm: %s. Niche: %s. Viết %d prompt chụp ảnh mẫu nữ Việt Nam với sản phẩm (KHÔNG chữ, KHÔNG watermark).
Yêu cầu: ảnh 1 là hook (mẫu giơ/cầm sản phẩm cười với camera), các ảnh giữa là lifestyle (phố, café) + macro chất liệu + khoảnh khắc dùng sản phẩm thật, ảnh cuối ấm áp ôm sản phẩm.
Mỗi prompt bằng tiếng Anh, photorealistic, vertical 9:16, mô tả chi tiết người mẫu + sản phẩm + ánh sáng.
Chỉ trả JSON: {"photos": ["prompt1", "prompt2", ...]}`, p.ProductName, p.Niche, n)
	text, err := s.llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director: %w", err)
	}
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "```"))
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var plan struct {
		Photos []string `json:"photos"`
	}
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, fmt.Errorf("director JSON: %w", err)
	}
	var out []string
	for _, ph := range plan.Photos {
		if strings.TrimSpace(ph) != "" {
			out = append(out, ph)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("director: no photos planned")
	}
	return out, nil
}

func (s *Studio) runAffiliate(id string, p AffiliateParams) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}

	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	s.appendLog(id, "Director đang viết shot list…")
	prompts, err := s.directorPhotoPlan(ctx, p)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Shot list: %d ảnh", len(prompts)))

	refs := []ImageRef{}
	if p.ModelPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ModelPhoto})
	}
	if p.ProductPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ProductPhoto})
	}

	// Shoot each photo (identity/product lock via reference images).
	var photos []string
	for i, pr := range prompts {
		aid := s.addAsset(id, i, "photo", pr)
		out := filepath.Join(work, fmt.Sprintf("photo-%02d.png", i))
		s.appendLog(id, fmt.Sprintf("Chụp ảnh %d/%d…", i+1, len(prompts)))
		s.setAsset(aid, StatusRunning, "")
		if err := s.mg.GenerateImage(ctx, pr, refs, out); err != nil {
			s.appendLog(id, fmt.Sprintf("Ảnh %d lỗi: %v", i+1, err))
			s.setAsset(aid, StatusFailed, "")
			continue
		}
		s.setAsset(aid, StatusDone, out)
		photos = append(photos, out)
		s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(prompts))))
	}
	if len(photos) < 3 {
		fail(fmt.Errorf("chỉ chụp được %d/%d ảnh — kiểm tra API key", len(photos), len(prompts)))
		return
	}

	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	secsPer := float64(p.Seconds) / float64(len(photos))
	if p.Mode == AffiliateModeShots {
		s.appendLog(id, "Chế độ nhiều cảnh: đang dựng clip từng shot (Veo)…")
		var clips []string
		for i, pr := range prompts {
			aid := s.addAsset(id, 100+i, "clip", pr)
			out := filepath.Join(work, fmt.Sprintf("shot-%02d.mp4", i))
			s.setAsset(aid, StatusRunning, "")
			// Image-to-video from the photo keeps identity locked.
			if err := s.mg.GenerateVideo(ctx, pr, photos[i], 6, out); err != nil {
				s.appendLog(id, fmt.Sprintf("Shot %d lỗi: %v (giữ ảnh tĩnh)", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
			s.setAsset(aid, StatusDone, out)
			clips = append(clips, out)
		}
		silent := filepath.Join(work, "silent.mp4")
		if len(clips) > 0 {
			s.appendLog(id, "Nối các shot…")
			if err := ConcatClips(ctx, clips, silent); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Veo không khả dụng — dùng bản list ảnh.")
			if err := AssemblePhotoList(ctx, photos, secsPer, "", 0, silent); err != nil {
				fail(err)
				return
			}
		}
		if p.MusicPath != "" {
			s.appendLog(id, "Ghép nhạc trending…")
			if err := MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else if err := os.Rename(silent, final); err != nil {
			fail(err)
			return
		}
	} else {
		s.appendLog(id, "Dựng list ảnh + nhạc…")
		if p.MusicPath != "" {
			if err := AssemblePhotoList(ctx, photos, secsPer, p.MusicPath, p.MusicStart, final); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Chưa có file nhạc — xuất bản không nhạc (tải sound trending ở tab Trending rồi tạo lại).")
			if err := AssemblePhotoList(ctx, photos, secsPer, "", 0, filepath.Join(work, "silent.mp4")); err != nil {
				fail(err)
				return
			}
			if err := os.Rename(filepath.Join(work, "silent.mp4"), final); err != nil {
				fail(err)
				return
			}
		}
	}

	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
}

// ---------------------------------------------------------------------------
// short-film jobs
// ---------------------------------------------------------------------------

// CreateFilmJob queues a short-film job and starts it.
func (s *Studio) CreateFilmJob(p FilmParams) (string, error) {
	if p.Seconds <= 0 {
		p.Seconds = 90
	}
	id, err := s.insertJob(KindFilm, p.Topic, p)
	if err != nil {
		return "", err
	}
	go s.runFilm(id, p)
	return id, nil
}

func (s *Studio) runFilm(id string, p FilmParams) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}
	if s.llm == nil {
		fail(fmt.Errorf("thiếu LLM"))
		return
	}
	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	s.appendLog(id, "Director đang viết kịch bản…")
	plan, err := engines.PlanFilm(ctx, s.llm, p.Topic, p.Seconds, 6)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Kịch bản: %q — %d cảnh", plan.Title, len(plan.Scenes)))

	var scenes []string
	for i, sc := range plan.Scenes {
		aid := s.addAsset(id, i, "clip", sc.ImagePrompt)
		mp4 := filepath.Join(work, fmt.Sprintf("scene%02d.mp4", i))
		s.setAsset(aid, StatusRunning, "")
		made := false
		// Try real video generation first (Veo).
		if s.mg != nil && s.mg.Healthy(ctx) {
			s.appendLog(id, fmt.Sprintf("Quay cảnh %d/%d (Veo)…", i+1, len(plan.Scenes)))
			vprompt := sc.ImagePrompt + ". Vertical 9:16 cinematic video, subtle natural motion."
			if err := s.mg.GenerateVideo(ctx, vprompt, "", 8, mp4); err == nil {
				made = true
			} else {
				s.appendLog(id, fmt.Sprintf("Veo lỗi (%v) — dùng ảnh + voiceover", err))
			}
		}
		if !made {
			// Honest fallback: still image + Ken Burns + narration.
			img := filepath.Join(work, fmt.Sprintf("scene%02d.png", i))
			if s.mg != nil {
				if err := s.mg.GenerateImage(ctx, sc.ImagePrompt, nil, img); err != nil {
					s.appendLog(id, fmt.Sprintf("Cảnh %d lỗi ảnh: %v", i+1, err))
					s.setAsset(aid, StatusFailed, "")
					continue
				}
			} else {
				fail(fmt.Errorf("media-gen chưa sẵn sàng"))
				return
			}
			wavPath := ""
			if s.narrator != nil && strings.TrimSpace(sc.Narration) != "" {
				wav, err := s.narrator(ctx, sc.Narration)
				if err == nil && len(wav) > 0 {
					wavPath = filepath.Join(work, fmt.Sprintf("scene%02d.wav", i))
					_ = os.WriteFile(wavPath, wav, 0o644)
				}
			}
			dur := float64(sc.Seconds)
			if wavPath != "" {
				if ws, err := engines.WavSeconds(wavPath); err == nil && ws > dur {
					dur = ws
				}
			}
			if wavPath == "" {
				// Silent still: hold the image.
				if err := AssemblePhotoList(ctx, []string{img}, dur, "", 0, mp4); err != nil {
					s.appendLog(id, fmt.Sprintf("Cảnh %d lỗi dựng: %v", i+1, err))
					s.setAsset(aid, StatusFailed, "")
					continue
				}
			} else if err := engines.RenderScene(ctx, img, wavPath, dur, mp4); err != nil {
				s.appendLog(id, fmt.Sprintf("Cảnh %d lỗi dựng: %v", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
		}
		s.setAsset(aid, StatusDone, mp4)
		scenes = append(scenes, mp4)
		s.setStatus(id, StatusRunning, 10+int(80*float64(i+1)/float64(len(plan.Scenes))))
	}
	if len(scenes) == 0 {
		fail(fmt.Errorf("không dựng được cảnh nào"))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	s.appendLog(id, "Nối các cảnh…")
	if err := ConcatClips(ctx, scenes, final); err != nil {
		fail(err)
		return
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// DownloadAudio fetches a direct audio URL (mp3/m4a) to a local file — used
// when the user pastes a TikTok CDN / direct link for the trending sound.
func DownloadAudio(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("audio HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 100<<20))
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}

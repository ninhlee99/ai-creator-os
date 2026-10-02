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
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)

	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
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
	Mode         string  `json:"mode"` // photo | shots
	AccountID    int64   `json:"account_id,omitempty"` // set by autopilot
	Niche        string  `json:"niche"`
	ProductName  string  `json:"product_name"`
	ModelPhoto   string  `json:"model_photo"`   // local path (identity lock)
	ProductPhoto string  `json:"product_photo"` // local path (product lock)
	Seconds      int     `json:"seconds"`
	BPM          int     `json:"bpm"`         // beat tempo for the giật-giật cut; 0 = 120
	MusicPath    string  `json:"music_path"`  // local audio file, "" = silent
	MusicStart   float64 `json:"music_start"` // seconds into the track
	MusicTitle   string  `json:"music_title"`
	MusicArtist  string  `json:"music_artist"`
}

// FilmParams describes one short-film job.
type FilmParams struct {
	Topic   string `json:"topic"`
	Seconds int    `json:"seconds"`
	// Aspect is the delivery frame: "9:16" (Shorts/TikTok) or "16:9"
	// (YouTube long-form). Empty means "9:16" — every pre-existing flow
	// keeps its old behavior.
	Aspect string `json:"aspect"`
}

// AspectDims maps a job aspect to ffmpeg output dimensions. Unknown or
// empty aspects fall back to 9:16 so old jobs/params keep rendering.
func AspectDims(aspect string) (w, h int) {
	if aspect == "16:9" {
		return 1920, 1080
	}
	return 1080, 1920
}

// orientationWord renders the aspect for image/video generation prompts
// (director + Veo keyframe/video prompts must ask for the real frame,
// otherwise a "long" YouTube cut would be generated vertical).
func orientationWord(aspect string) string {
	if aspect == "16:9" {
		return "horizontal 16:9"
	}
	return "vertical 9:16"
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
	Caption    string `json:"caption"`
	Params     string `json:"params"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at"`
}

// Asset is one generated photo/clip inside a job (the storyboard row).
type Asset struct {
	ID     int64  `json:"id"`
	JobID  string `json:"job_id"`
	Idx    int    `json:"idx"`
	Kind   string `json:"kind"` // photo | clip | portrait
	Path   string `json:"path"`
	Prompt string `json:"prompt"`
	Status string `json:"status"` // queued|running|done|failed
	// Method is how this asset was really rendered: "veo" or "anh-tts"
	// (keyframe + TTS fallback). "" = unknown/legacy.
	Method string `json:"method"`
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
	onDone   OnDoneFunc
	// jobSem caps concurrent renders (see jobSlot); lazily built because
	// tests also construct Studio as a struct literal.
	jobSem chan struct{}
}

// maxConcurrentStudioJobs bounds how many jobs render at once. Each job
// runs heavy local ffmpeg work (1080x1920 zoompan graphs plus a lanczos
// 4K upscale per photo) on the same machine as the llama-server and
// VieNeu sidecars; on the target Mac (M1, 32 GB unified memory) more
// than 2 parallel renders thrash CPU and memory for no wall-clock win.
// Extra jobs wait with status "queued" (their DB state until they start).
const maxConcurrentStudioJobs = 2

// jobSlot returns the render semaphore, creating it on first use.
func (s *Studio) jobSlot() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobSem == nil {
		s.jobSem = make(chan struct{}, maxConcurrentStudioJobs)
	}
	return s.jobSem
}

// New opens (or creates) the studio database and returns the orchestrator.
// outDir is where final videos land (served at /media/).
func New(dbPath string, llm LLM, mg MediaGen, narrator Narrator, outDir string) (*Studio, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("studio db: %w", err)
	}
	// One connection, like the ledger/products stores: busy_timeout is a
	// per-connection pragma, so a pooled second connection would hit
	// SQLITE_BUSY while a render goroutine is writing its log.
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := stampUserVersion(db, 1); err != nil {
		db.Close()
		return nil, fmt.Errorf("studio db: pragma user_version: %w", err)
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
	if err != nil {
		return err
	}
	// Additive migrations: tolerate "duplicate column" on re-run.
	for _, q := range []string{
		`ALTER TABLE studio_jobs ADD COLUMN caption TEXT NOT NULL DEFAULT ''`,
		// Film Wave 1 (P0-7): per-scene render method ("veo" | "anh-tts" |
		// ""), shown on the storyboard so the UI never overstates Veo usage.
		`ALTER TABLE studio_assets ADD COLUMN method TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(q); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	return nil
}

// Close releases the database handle.
func (s *Studio) Close() error { return s.db.Close() }

// UploadDir is where the web layer saves uploaded model/product photos and
// music files.
func (s *Studio) UploadDir() string { return filepath.Join(s.workRoot, "uploads") }

// ModelLibrary returns the per-account model photo library (Ninh's uploads).
func (s *Studio) ModelLibrary() (*ModelStore, error) {
	return NewModelStore(s.db, s.workRoot)
}

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

func (s *Studio) setCaption(id, caption string) {
	s.db.Exec(`UPDATE studio_jobs SET caption=? WHERE id=?`, caption, id)
}

// AppendLog adds a line to a job's log from outside the studio package
// (e.g. the auto-publish hook in cmd/aicos).
func (s *Studio) AppendLog(id, line string) { s.appendLog(id, line) }

// OnDoneFunc runs after a studio job finishes successfully. It is called
// from the job's goroutine; keep it quick or spawn its own goroutine.
type OnDoneFunc func(jobID string)

// SetOnDone registers the completion hook (nil clears it).
func (s *Studio) SetOnDone(fn OnDoneFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDone = fn
}

func (s *Studio) fireOnDone(id string) {
	s.mu.Lock()
	fn := s.onDone
	s.mu.Unlock()
	if fn != nil {
		fn(id)
	}
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

// setAssetMethod records how an asset was really rendered ("veo" or
// "anh-tts") so the storyboard stays honest about Veo usage.
func (s *Studio) setAssetMethod(id int64, method string) {
	s.db.Exec(`UPDATE studio_assets SET method=? WHERE id=?`, method, id)
}

// GetJob returns one job.
func (s *Studio) GetJob(id string) (Job, bool) {
	var j Job
	err := s.db.QueryRow(`SELECT id,kind,title,status,progress,log,output,caption,params,created_at,finished_at FROM studio_jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Log, &j.Output, &j.Caption, &j.Params, &j.CreatedAt, &j.FinishedAt)
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
	rows, err := s.db.Query(`SELECT id,job_id,idx,kind,path,prompt,status,method FROM studio_assets WHERE job_id=? ORDER BY idx`, jobID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.JobID, &a.Idx, &a.Kind, &a.Path, &a.Prompt, &a.Status, &a.Method); err == nil {
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
	if p.BPM < 60 || p.BPM > 200 {
		p.BPM = 120
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

// photoPlan is the director's locked shoot plan: ONE location shared by
// every photo (Ninh's rule 2026-10-01 — e.g. all 3-5 shots inside the same
// café, just different angles/setups), plus the per-photo prompts that
// already embed that location verbatim.
type photoPlan struct {
	Location string
	Prompts  []string
}

// composePhotoPrompt fuses the quality bar, the locked location and the
// photo-specific angle/setup into the final generation prompt. The
// location lock is embedded in every prompt so the model keeps one
// consistent place across all photos of the video.
func composePhotoPrompt(location, photoPart string) string {
	return "Ultra-detailed professional photograph, 4K quality, tack-sharp focus, " +
		"photorealistic, vertical 9:16. " +
		"LOCATION (identical in every photo of this video): " + location + " " +
		photoPart +
		" No text, no watermark, no logo."
}

// directorPhotoPlan asks the LLM for the photo list (the storyboard).
// Style Ninh chốt 2026-10-01: CapCut "giật giật" — chỉ 3–5 ảnh mẫu dùng
// sản phẩm thật, dựng beat-bounce cắt cứng theo nhịp; 30s ≈ 5 ảnh × 6s.
// Địa điểm KHÓA NHẤT QUÁN cho cả video: mọi ảnh cùng một nơi, chỉ khác
// góc máy / vị trí / setup bên trong nơi đó.
func (s *Studio) directorPhotoPlan(ctx context.Context, p AffiliateParams) (*photoPlan, error) {
	n := p.Seconds / 6
	if n < 3 {
		n = 3
	}
	if n > 5 {
		n = 5
	}
	sys := "Bạn là đạo diễn ảnh thời trang TikTok Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Sản phẩm: %s. Niche: %s.

Nhiệm vụ 2 bước:
1. Chọn MỘT địa điểm duy nhất cho cả video (ví dụ: một quán café cụ thể — mô tả chi tiết bằng tiếng Anh: phong cách nội thất, màu sắc, ánh sáng, chi tiết nhận diện; đủ cụ thể để 5 ảnh khác nhau vẫn nhận ra là CÙNG MỘT NƠI).
2. Viết %d prompt ảnh (tiếng Anh), MỖI prompt mô tả: góc máy + vị trí của mẫu TRONG địa điểm đó + setup khác nhau + hành động dùng sản phẩm thật. Các ảnh phải đa dạng góc (cận cảnh, toàn cảnh, macro, low-angle, qua vai…) nhưng TUYỆT ĐỐI cùng một địa điểm.

Video dựng kiểu CapCut "giật giật": cắt cứng theo nhịp, mỗi ảnh chỉ hiện vài giây nên MỖI ẢNH phải là một khoảnh khắc đắt giá.
Ảnh 1 là hook (mẫu giơ/cầm sản phẩm cười với camera), các ảnh giữa là lifestyle trong địa điểm + macro chất liệu + khoảnh khắc dùng sản phẩm thật, ảnh cuối ấm áp ôm sản phẩm.
KHÔNG chữ, KHÔNG watermark trong ảnh.

Chỉ trả JSON: {"location": "<mô tả địa điểm chi tiết bằng tiếng Anh>",
"photos": ["<góc máy + vị trí + setup + hành động, tiếng Anh>", ...]}`, p.ProductName, p.Niche, n)
	text, err := s.llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director: %w", err)
	}
	var raw struct {
		Location string   `json:"location"`
		Photos   []string `json:"photos"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, err
	}
	location := strings.TrimSpace(raw.Location)
	if location == "" {
		return nil, fmt.Errorf("director: missing location lock")
	}
	plan := &photoPlan{Location: location}
	for _, ph := range raw.Photos {
		ph = strings.TrimSpace(ph)
		if ph == "" {
			continue
		}
		plan.Prompts = append(plan.Prompts, composePhotoPrompt(location, ph))
	}
	if len(plan.Prompts) == 0 {
		return nil, fmt.Errorf("director: no photos planned")
	}
	return plan, nil
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

	// Render-slot gate: wait for one of maxConcurrentStudioJobs slots
	// before marking the job running, so queued jobs keep their honest
	// "queued" status while heavier renders finish ahead of them.
	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

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

	refs := []ImageRef{}
	if p.ModelPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ModelPhoto})
	}
	if p.ProductPhoto != "" {
		refs = append(refs, ImageRef{Path: p.ProductPhoto})
	}

	// Photo mode: shoot the 3–5 photo list (scene-locked, 4K masters).
	// Shots mode has its own cinematic shot list below (no photo shoot
	// here — saves quota).
	var photos []string
	var secsPer float64
	if p.Mode != AffiliateModeShots {
		s.appendLog(id, "Director đang khóa địa điểm + viết shot list…")
		plan, err := s.directorPhotoPlan(ctx, p)
		if err != nil {
			fail(err)
			return
		}
		s.appendLog(id, fmt.Sprintf("Địa điểm: %s", plan.Location))
		s.appendLog(id, fmt.Sprintf("Shot list: %d ảnh", len(plan.Prompts)))

		// Shoot each photo (identity/product lock via reference images),
		// then master to 4K.
		for i, pr := range plan.Prompts {
			aid := s.addAsset(id, i, "photo", pr)
			out := filepath.Join(work, fmt.Sprintf("photo-%02d.png", i))
			s.appendLog(id, fmt.Sprintf("Chụp ảnh %d/%d…", i+1, len(plan.Prompts)))
			s.setAsset(aid, StatusRunning, "")
			if err := s.mg.GenerateImage(ctx, pr, refs, out); err != nil {
				s.appendLog(id, fmt.Sprintf("Ảnh %d lỗi: %v", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
			master := filepath.Join(work, fmt.Sprintf("photo-%02d-4k.png", i))
			s.appendLog(id, fmt.Sprintf("Nâng ảnh %d lên 4K…", i+1))
			if err := UpscalePhoto(ctx, out, master, PhotoWidth4K); err != nil {
				s.appendLog(id, fmt.Sprintf("Upscale ảnh %d lỗi: %v (dùng bản gốc)", i+1, err))
				master = out
			}
			s.setAsset(aid, StatusDone, master)
			photos = append(photos, master)
			s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(plan.Prompts))))
		}
		if len(photos) < 3 {
			fail(fmt.Errorf("chỉ chụp được %d/%d ảnh — kiểm tra API key", len(photos), len(plan.Prompts)))
			return
		}
		secsPer = float64(p.Seconds) / float64(len(photos))
	}

	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.Mode == AffiliateModeShots {
		s.appendLog(id, "Director điện ảnh đang viết shot list…")
		shots, err := WriteProShotList(ctx, s.llm, p.ProductName, p.Niche, p.Seconds)
		if err != nil {
			fail(fmt.Errorf("shot list: %w", err))
			return
		}
		s.appendLog(id, fmt.Sprintf("Shot list điện ảnh: %d shot", len(shots)))
		var clips []string
		for i, sh := range shots {
			aid := s.addAsset(id, 100+i, "clip",
				fmt.Sprintf("[%s/%s] %s", sh.ShotSize, sh.CameraMove, sh.Action))
			// Keyframe: khóa identity (ảnh mẫu) + sản phẩm.
			key := filepath.Join(work, fmt.Sprintf("key-%02d.png", i))
			kprompt := sh.ImagePrompt + ". Photorealistic, vertical 9:16, no text, no watermark."
			s.setAsset(aid, StatusRunning, "")
			if err := s.mg.GenerateImage(ctx, kprompt, refs, key); err != nil {
				s.appendLog(id, fmt.Sprintf("Shot %d lỗi keyframe: %v", i+1, err))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
			out := filepath.Join(work, fmt.Sprintf("shot-%02d.mp4", i))
			vprompt := fmt.Sprintf("%s. Camera: %s. Lens/lighting: %s. "+
				"Vertical 9:16 cinematic product video, smooth professional motion, no text, no watermark.",
				sh.VideoPrompt, sh.CameraMove, sh.LensLight)
			if err := s.mg.GenerateVideo(ctx, vprompt, key, sh.Seconds, "9:16", out); err != nil {
				s.appendLog(id, fmt.Sprintf("Shot %d lỗi video: %v (giữ keyframe Ken Burns)", i+1, err))
				still := filepath.Join(work, fmt.Sprintf("shot-%02d-still.mp4", i))
				if kerr := AssemblePhotoList(ctx, []string{key}, float64(sh.Seconds), "", 0, still, "9:16"); kerr == nil {
					s.setAsset(aid, StatusDone, still)
					clips = append(clips, still)
				} else {
					s.setAsset(aid, StatusFailed, "")
				}
				continue
			}
			s.setAsset(aid, StatusDone, out)
			clips = append(clips, out)
			s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(shots))))
		}
		silent := filepath.Join(work, "silent.mp4")
		if len(clips) > 0 {
			s.appendLog(id, "Nối các shot…")
			if err := ConcatClips(ctx, clips, "9:16", silent); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Veo không khả dụng — dùng bản list ảnh.")
			if err := AssemblePhotoList(ctx, photos, secsPer, "", 0, silent, "9:16"); err != nil {
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
		s.appendLog(id, "Dựng giật-giật theo nhịp nhạc…")
		if p.MusicPath != "" {
			if err := AssembleBeatBounce(ctx, photos, secsPer, p.BPM, p.MusicPath, p.MusicStart, final, "9:16"); err != nil {
				fail(err)
				return
			}
		} else {
			s.appendLog(id, "Chưa có file nhạc — xuất bản không nhạc (tải sound trending ở tab Trending rồi tạo lại).")
			if err := AssembleBeatBounce(ctx, photos, secsPer, p.BPM, "", 0, filepath.Join(work, "silent.mp4"), "9:16"); err != nil {
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
	// Caption + hashtag for the post (used by auto-publish, and shown in
	// the UI for manual posting).
	if s.llm != nil {
		s.appendLog(id, "Viết caption + hashtag…")
		if cap, cerr := WriteCaption(ctx, s.llm, p.ProductName, p.Niche); cerr == nil {
			s.setCaption(id, cap)
		} else {
			s.appendLog(id, "Caption lỗi: "+cerr.Error())
		}
	}
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
	s.fireOnDone(id)
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

// MarkInterruptedJobs fails every job left "running" from a previous process
// (their render goroutines died with it). Call once at startup; returns the
// number of jobs swept. The user can resume film jobs with RerunFilmJob.
func (s *Studio) MarkInterruptedJobs() int {
	rows, err := s.db.Query(`SELECT id FROM studio_jobs WHERE status=?`, StatusRunning)
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.appendLog(id, "Gián đoạn khi khởi động lại — bấm Chạy tiếp để tiếp tục")
		s.setStatus(id, StatusFailed, 100)
	}
	return len(ids)
}

// RerunFilmJob resumes a film job from its stored params. Scenes whose output
// asset is done and still on disk are skipped by runFilm (see
// resumeAssetPath), so this is a true resume, not a from-scratch restart.
func (s *Studio) RerunFilmJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	if j.Kind != KindFilm {
		return fmt.Errorf("chỉ job phim mới chạy tiếp được")
	}
	s.mu.Lock()
	_, already := s.running[id]
	s.mu.Unlock()
	if already {
		return fmt.Errorf("job đang chạy")
	}
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	s.appendLog(id, "Chạy tiếp từ cảnh chưa xong…")
	s.setStatus(id, StatusRunning, 5)
	go s.runFilm(id, p)
	return nil
}

// resumeAssetPath returns the on-disk output of an already-finished asset
// (same job, index and kind), or "" when the scene must be rendered again.
// A "done" asset whose file is missing or empty does NOT count — the scene
// is rebuilt instead of silently linking a dead file.
func (s *Studio) resumeAssetPath(jobID string, idx int, kind string) string {
	var path, status string
	_ = s.db.QueryRow(
		`SELECT path, status FROM studio_assets WHERE job_id=? AND idx=? AND kind=? ORDER BY id DESC LIMIT 1`,
		jobID, idx, kind).Scan(&path, &status)
	if status != StatusDone || path == "" {
		return ""
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return ""
	}
	return path
}

// maxTTSChunkChars caps one TTS call: long narration is split by sentence so
// a film never becomes a single giant synthesis request (P0-6).
const maxTTSChunkChars = 800

// portraitIdxBase keeps portrait assets in their own index space, after the
// scene clips (which use 0..n), in the storyboard ordering.
const portraitIdxBase = 1000

// splitTextChunks splits text into sentence-boundary chunks of at most
// maxChars runes, preserving order. Sentence ends are . ! ? and newlines;
// "…" is kept inside chunks as a pause cue for the TTS voice.
func splitTextChunks(text string, maxChars int) []string {
	var sents []string
	var cur strings.Builder
	flush := func() {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			sents = append(sents, t)
		}
		cur.Reset()
	}
	for _, r := range text {
		cur.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			flush()
		}
	}
	flush()
	var chunks []string
	var acc strings.Builder
	push := func() {
		t := strings.TrimSpace(acc.String())
		if t != "" {
			chunks = append(chunks, t)
		}
		acc.Reset()
	}
	for _, sn := range sents {
		if acc.Len() > 0 &&
			utf8.RuneCountInString(acc.String())+1+utf8.RuneCountInString(sn) > maxChars {
			push()
		}
		if acc.Len() > 0 {
			acc.WriteByte(' ')
		}
		acc.WriteString(sn)
	}
	push()
	if len(chunks) == 0 && strings.TrimSpace(text) != "" {
		chunks = []string{strings.TrimSpace(text)}
	}
	return chunks
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

	// Render-slot gate: same cap as affiliate jobs (see runAffiliate).
	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

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

	s.appendLog(id, "Biên kịch đang viết kịch bản phim…")
	orient := orientationWord(p.Aspect)
	// P0-3: the script is cached in the work dir so a resumed run renders
	// the SAME scenes (index-based resume stays aligned) and doesn't pay
	// the LLM twice.
	var script FilmScriptPro
	scriptPath := filepath.Join(work, "script.json")
	if raw, rerr := os.ReadFile(scriptPath); rerr == nil {
		if jerr := json.Unmarshal(raw, &script); jerr == nil && len(script.Scenes) > 0 {
			s.appendLog(id, "Dùng lại kịch bản đã viết (resume)…")
		} else {
			script = FilmScriptPro{}
		}
	}
	if len(script.Scenes) == 0 {
		var err error
		script, err = WriteFilmScript(ctx, s.llm, p.Topic, p.Seconds, p.Aspect)
		if err != nil {
			fail(err)
			return
		}
		if raw, jerr := json.Marshal(script); jerr == nil {
			_ = os.WriteFile(scriptPath, raw, 0o644)
		}
	}
	s.appendLog(id, fmt.Sprintf("Kịch bản: %q — %d nhân vật, %d cảnh",
		script.Title, len(script.Characters), len(script.Scenes)))

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}

	// Character portraits: khóa identity — mọi cảnh sau dùng làm reference.
	// Portraits stay vertical 9:16 on purpose: they are identity references
	// for generation, not delivery frames (the scenes themselves render in
	// the job's aspect).
	for i := range script.Characters {
		c := &script.Characters[i]
		if done := s.resumeAssetPath(id, portraitIdxBase+i, "portrait"); done != "" {
			s.appendLog(id, fmt.Sprintf("Chân dung %q đã có — bỏ qua", c.Name))
			c.Portrait = done
			continue
		}
		pp := fmt.Sprintf("Cinematic character portrait, %s. Wardrobe: %s. "+
			"Photorealistic, vertical 9:16, neutral expression, plain background, no text, no watermark.",
			c.Appearance, c.Wardrobe)
		out := filepath.Join(work, fmt.Sprintf("char-%02d.png", i))
		pid := s.addAsset(id, portraitIdxBase+i, "portrait", pp)
		s.setAsset(pid, StatusRunning, "")
		s.appendLog(id, fmt.Sprintf("Vẽ chân dung nhân vật %q (khóa identity)…", c.Name))
		if err := s.mg.GenerateImage(ctx, pp, nil, out); err != nil {
			// P2-4: a broken portrait must show up failed on the storyboard,
			// never vanish silently — scenes still render from the text
			// description via charLocks.
			s.appendLog(id, fmt.Sprintf("⚠ Chân dung %q lỗi: %v — cảnh vẫn quay bằng mô tả chữ", c.Name, err))
			s.setAsset(pid, StatusFailed, "")
			continue
		}
		s.setAsset(pid, StatusDone, out)
		c.Portrait = out
	}
	charRefs := script.CharacterRefs()
	charLocks := ""
	for _, c := range script.Characters {
		charLocks += "\n" + c.LockBlock()
	}

	var scenes []string
	for i, sc := range script.Scenes {
		// P0-3: resume — skip scenes that already rendered in a previous run.
		if done := s.resumeAssetPath(id, i, "clip"); done != "" {
			s.appendLog(id, fmt.Sprintf("Cảnh %d đã dựng xong — bỏ qua", i+1))
			scenes = append(scenes, done)
			s.setStatus(id, StatusRunning, 10+int(80*float64(i+1)/float64(len(script.Scenes))))
			continue
		}
		aid := s.addAsset(id, i, "clip", sc.ImagePrompt)
		mp4 := filepath.Join(work, fmt.Sprintf("scene%02d.mp4", i))
		s.setAsset(aid, StatusRunning, "")
		made := false
		method := ""
		// Keyframe: khóa nhân vật + khóa bối cảnh (địa điểm/thời gian/ánh sáng).
		keyPrompt := sc.ImagePrompt + charLocks + "\n" + sc.SceneLockBlock() +
			" Cinematic photorealistic, " + orient + ", no text, no watermark."
		img := filepath.Join(work, fmt.Sprintf("scene%02d.png", i))
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, img); kerr != nil {
			s.appendLog(id, fmt.Sprintf("Cảnh %d lỗi keyframe: %v", i+1, kerr))
			s.setAsset(aid, StatusFailed, "")
			continue
		}
		// Quay: gắn camera language của từng shot + khóa bối cảnh.
		var camBits []string
		for _, sh := range sc.Shots {
			camBits = append(camBits, fmt.Sprintf("%s %s", sh.ShotSize, sh.CameraMove))
		}
		vprompt := fmt.Sprintf("%s. Shots: %s. %s cinematic film, natural motion, no text.",
			sc.ImagePrompt, strings.Join(camBits, "; "), orient) + charLocks + "\n" + sc.SceneLockBlock()
		s.appendLog(id, fmt.Sprintf("Quay cảnh %d/%d…", i+1, len(script.Scenes)))
		// P0-7: Veo 3 renders at most ~8s/clip — clampVeoSeconds enforces it
		// inside GenerateVideo; the log states the real duration honestly.
		if err := s.mg.GenerateVideo(ctx, vprompt, img, sc.Seconds, p.Aspect, mp4); err == nil {
			made = true
			method = "veo"
			s.appendLog(id, fmt.Sprintf("Cảnh %d: quay bằng Veo (%ds)", i+1, min(sc.Seconds, veoMaxSeconds)))
		} else {
			s.appendLog(id, fmt.Sprintf("Cảnh %d: Veo lỗi (%v) — dùng ảnh + giọng đọc", i+1, err))
		}
		if !made {
			method = "anh-tts"
			// Thoại: nối các câu thoại (kèm lời dẫn nếu có), TTS từng đoạn
			// ≤800 ký tự rồi nối lại — không gọi 1 lần cho cả phim (P0-6).
			var lines []string
			for _, d := range sc.Dialogue {
				lines = append(lines, d.Text)
			}
			speech := strings.Join(lines, " … ")
			if strings.TrimSpace(sc.Narration) != "" {
				speech = sc.Narration + " … " + speech
			}
			wavPath := ""
			if s.narrator != nil && strings.TrimSpace(speech) != "" {
				var wavs [][]byte
				ttsOK := true
				for _, ch := range splitTextChunks(speech, maxTTSChunkChars) {
					w, err := s.narrator(ctx, ch)
					if err != nil || len(w) == 0 {
						s.appendLog(id, fmt.Sprintf("Cảnh %d: TTS lỗi (%v) — dựng bản câm", i+1, err))
						ttsOK = false
						break
					}
					wavs = append(wavs, w)
				}
				if ttsOK {
					if joined, jerr := tts.ConcatWavs(wavs); jerr == nil {
						wavPath = filepath.Join(work, fmt.Sprintf("scene%02d.wav", i))
						if werr := os.WriteFile(wavPath, joined, 0o644); werr != nil {
							s.appendLog(id, fmt.Sprintf("Cảnh %d: không ghi được WAV: %v", i+1, werr))
							wavPath = ""
						}
					} else {
						s.appendLog(id, fmt.Sprintf("Cảnh %d: nối WAV lỗi (%v) — dựng bản câm", i+1, jerr))
					}
				}
			}
			dur := float64(sc.Seconds)
			if wavPath != "" {
				if ws, err := engines.WavSeconds(wavPath); err == nil && ws > dur {
					dur = ws
				}
			}
			var rerr error
			if wavPath == "" {
				rerr = AssemblePhotoList(ctx, []string{img}, dur, "", 0, mp4, p.Aspect)
			} else {
				rerr = engines.RenderScene(ctx, img, wavPath, dur, p.Aspect, mp4)
			}
			if rerr != nil {
				s.appendLog(id, fmt.Sprintf("Cảnh %d lỗi dựng: %v", i+1, rerr))
				s.setAsset(aid, StatusFailed, "")
				continue
			}
		}
		s.setAssetMethod(aid, method)
		s.setAsset(aid, StatusDone, mp4)
		scenes = append(scenes, mp4)
		s.setStatus(id, StatusRunning, 10+int(80*float64(i+1)/float64(len(script.Scenes))))
	}
	if len(scenes) == 0 {
		fail(fmt.Errorf("không dựng được cảnh nào"))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	s.appendLog(id, "Nối các cảnh…")
	if err := ConcatClips(ctx, scenes, p.Aspect, final); err != nil {
		fail(err)
		return
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
	s.fireOnDone(id)
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

// stampUserVersion records the schema generation for future migrations
// (R2-W7): a fresh database is marked with v, an existing stamp is never
// overwritten here.
func stampUserVersion(db *sql.DB, v int) error {
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		return err
	}
	if cur == 0 {
		_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
		return err
	}
	return nil
}

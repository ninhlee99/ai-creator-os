package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	Mode         string  `json:"mode"`                 // photo | shots
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
// FilmParams describes one film job. Ninh 2026-10-02 (final): every film
// is 16:9 (cinematic standard) — Aspect is always "16:9"; vertical 9:16
// trailers are center-cropped automatically, never re-shot.
type FilmParams struct {
	Topic   string `json:"topic"`
	Seconds int    `json:"seconds"`
	// Aspect is the delivery frame: always "16:9" for films. Empty means
	// "16:9" — every pre-existing flow keeps working.
	Aspect string `json:"aspect"`
	// Genre drives the director's 3-act script (tâm lý, hành động…).
	Genre string `json:"genre,omitempty"`
	// MusicPath is an optional music bed (uploaded like affiliate music).
	MusicPath string `json:"music_path,omitempty"`
	// UpscaleFinal renders an extra lanczos-upscaled master. This is an
	// UPSCALE, not native 4K — the UI labels it honestly.
	UpscaleFinal bool `json:"upscale_final,omitempty"`
	// RenderMode: "auto" (default, khuyên dùng — Veo nếu key quay được,
	// ngược lại điện ảnh từ ảnh), "cinematic" (chỉ dựng từ ảnh, miễn phí),
	// "veo" (luôn thử Veo từng shot, tốn phí). "" = "auto".
	RenderMode string `json:"render_mode,omitempty"`
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
	// BudgetUSD returns the API spend ceiling in USD (0/nil = no ceiling).
	// Wired by the app from the automation settings facade (R2-W4 knob).
	BudgetUSD func() float64
	// QC checks rendered shots against identity drift. Default is the
	// honest no-op (see qc.go) — the storyboard UI is the manual QC loop.
	QC ShotQCChecker
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
		// QC defaults to the honest no-op (qc.go): no fake "AI approved"
		// badges. The storyboard UI is the real QC loop for now.
		QC: NoopQCChecker{},
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
	// Film Wave 3: bảng capabilities (khả năng AI: vẽ ảnh / quay video).
	if err := s.ensureCapabilitiesTable(); err != nil {
		return err
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
			if err := UpscalePhoto(ctx, out, master, PhotoWidth4K, PhotoWidth4K*16/9); err != nil {
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

// CreateFilmJob queues a film job and starts it. Aspect is forced to 16:9:
// every film is the cinematic standard (Ninh 2026-10-02); vertical 9:16
// trailers are center-cropped automatically, never re-shot.
func (s *Studio) CreateFilmJob(p FilmParams) (string, error) {
	if p.Seconds <= 0 {
		p.Seconds = 90
	}
	p.Aspect = "16:9"
	if strings.TrimSpace(p.Genre) == "" {
		p.Genre = defaultFilmGenre
	}
	// Film Wave 3: chuẩn hoá chế độ dựng — giá trị lạ → "auto".
	switch p.RenderMode {
	case "", RenderModeAuto, RenderModeCinematic, RenderModeVeo:
	default:
		p.RenderMode = RenderModeAuto
	}
	id, err := s.insertJob(KindFilm, p.Topic, p)
	if err != nil {
		return "", err
	}
	go s.runFilm(id, p)
	return id, nil
}

// veoRateUSD reads the Veo price per billed second from the environment.
// The default is explicitly an UNVERIFIED ESTIMATE until Ninh confirms real
// pricing — every surface showing money must carry that label.
func veoRateUSD() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("VEO_USD_PER_SEC")), 64); err == nil && v > 0 {
		return v
	}
	return 0.05
}

// veoBudget tracks Veo spend inside one film job against the API budget
// ceiling (R2-W4 knob ops.api_budget_usd, wired via Studio.BudgetUSD).
type veoBudget struct {
	spentSecs float64
	rate      float64
	cap       float64 // 0 = no ceiling
}

// check refuses another `next` billed seconds when it would cross the cap.
func (b *veoBudget) check(next float64) error {
	if b.cap > 0 && (b.spentSecs+next)*b.rate > b.cap {
		return fmt.Errorf("vượt trần chi phí API $%.2f (ước tính chưa kiểm chứng) — dừng để không phát sinh thêm. Nâng trần ở Cài đặt · Hệ thống rồi bấm Chạy tiếp", b.cap)
	}
	return nil
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

// errBudgetExceeded stops a film job cleanly when the next Veo call would
// cross the API spend ceiling (P0-5). Wrapped by renderFilmShot; runFilm
// treats it as fatal, per-shot errors only skip the shot.
var errBudgetExceeded = errors.New("vượt trần chi phí API")

// trailerIdxBase keeps trailer assets in their own index space, after the
// shot clips (0..n) and portraits (1000+i).
const trailerIdxBase = 2000

// loadOrWriteScript returns the cached script.json when a previous run wrote
// it (resume renders the SAME scenes so index-based resume stays aligned),
// otherwise asks the director to write a new one.
func (s *Studio) loadOrWriteScript(ctx context.Context, id string, p FilmParams, work string) (FilmScriptPro, error) {
	// Director 3 pha: "truyện → kịch bản → breakdown → storyboard → quay →
	// dựng". Output từng pha lưu vào script.json (FilmScriptBundle) để
	// storyboard UI xem lại khi cần; quay + dựng dùng bản ráp cuối.
	if b, rerr := loadScriptBundle(work); rerr == nil && len(b.Script.Scenes) > 0 {
		s.appendLog(id, "Dùng lại kịch bản đã viết (resume)…")
		return b.Script, nil
	}
	s.appendLog(id, "Biên kịch 3 pha: truyện → kịch bản → breakdown…")
	bundle, err := WriteFilmScriptBundle(ctx, s.llm, p.Topic, p.Genre, p.Seconds, p.Aspect,
		func(line string) { s.appendLog(id, line) })
	if err != nil {
		return FilmScriptPro{}, err
	}
	if raw, jerr := json.Marshal(bundle); jerr == nil {
		_ = os.WriteFile(filepath.Join(work, "script.json"), raw, 0o644)
	}
	return bundle.Script, nil
}

// loadCachedScript reads script.json written by a previous run (used by
// "Quay lại shot này" and re-assembly — never re-rolls the director).
// Tương thích cả bundle mới lẫn FilmScriptPro cũ (Wave 1–2).
func (s *Studio) loadCachedScript(work string) (FilmScriptPro, error) {
	b, err := loadScriptBundle(work)
	if err != nil {
		return FilmScriptPro{}, err
	}
	if len(b.Script.Scenes) == 0 {
		return FilmScriptPro{}, fmt.Errorf("kịch bản rỗng")
	}
	return b.Script, nil
}

// ScriptBundle trả bundle từng pha của job (truyện/kịch bản/breakdown) để
// storyboard UI xem lại khi cần.
func (s *Studio) ScriptBundle(jobID string) (FilmScriptBundle, error) {
	var b FilmScriptBundle
	if _, ok := s.GetJob(jobID); !ok {
		return b, fmt.Errorf("job không tồn tại")
	}
	return loadScriptBundle(filepath.Join(s.workRoot, jobID))
}

// expandScriptShots flattens every scene into render shots (≤8s each),
// assigning global sequence numbers. Deterministic: resume, re-render and
// re-assembly all recompute the identical list.
func expandScriptShots(script FilmScriptPro) []RenderShot {
	var shots []RenderShot
	for _, sc := range script.Scenes {
		shots = append(shots, expandSceneShots(sc, len(shots))...)
	}
	return shots
}

func sceneMap(script FilmScriptPro) map[int]FilmScenePro {
	m := make(map[int]FilmScenePro, len(script.Scenes))
	for _, sc := range script.Scenes {
		m[sc.Index] = sc
	}
	return m
}

func characterLocks(script FilmScriptPro) string {
	var sb strings.Builder
	for _, c := range script.Characters {
		sb.WriteString("\n")
		sb.WriteString(c.LockBlock())
	}
	return sb.String()
}

func firstPortraitPath(refs []ImageRef) string {
	if len(refs) > 0 {
		return refs[0].Path
	}
	return ""
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(dst); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(dst, b, 0o644)
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

	// 1. Kịch bản (3 hồi cho phim dài — xem director.go).
	script, err := s.loadOrWriteScript(ctx, id, p, work)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Kịch bản: %q — %d nhân vật, %d cảnh, thể loại %s",
		script.Title, len(script.Characters), len(script.Scenes), script.Genre))

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}

	// 2. Character portraits: khóa identity — mọi shot sau dùng làm reference.
	// Portraits stay vertical 9:16 on purpose: they are identity references
	// for generation, not delivery frames.
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
	charLocks := characterLocks(script)

	// 3. Mở cảnh thành shot render ≤8s (đơn vị thật của Veo).
	scenes := sceneMap(script)
	shots := expandScriptShots(script)
	s.appendLog(id, fmt.Sprintf("Chia %d cảnh → %d shot (mỗi shot ≤%ds)",
		len(script.Scenes), len(shots), veoMaxSeconds))

	// 4. Chế độ dựng (Film Wave 3): "auto" → Veo nếu key quay được
	// (probe/lịch sử), ngược lại đi thẳng điện ảnh từ ảnh — không thử Veo,
	// đỡ tốn phút poll API chết.
	renderMode := s.resolveRenderMode(p)
	s.appendLog(id, "Chế độ dựng: "+renderModeLabel(renderMode))

	// 5. Trần chi phí Veo (P0-5): nối knob ops.api_budget_usd. Chế độ điện
	// ảnh không gọi Veo nên chỉ ghi nhận, không tính toán chi phí.
	budget := &veoBudget{rate: veoRateUSD()}
	if s.BudgetUSD != nil {
		budget.cap = s.BudgetUSD()
	}
	if renderMode == RenderModeCinematic {
		s.appendLog(id, "Không tốn Veo — chỉ tốn image gen cho keyframe (rẻ)")
	} else if budget.cap > 0 {
		s.appendLog(id, fmt.Sprintf("Trần chi phí API: $%.2f — vượt trần sẽ dừng (giá Veo ước tính chưa kiểm chứng)", budget.cap))
	} else {
		s.appendLog(id, fmt.Sprintf("Chưa đặt trần chi phí API — %.0f shot × ≤%ds × $%.3f/s (ước tính chưa kiểm chứng)",
			float64(len(shots)), veoMaxSeconds, budget.rate))
	}

	// 6. Render từng shot theo chế độ.
	var rendered []renderedShot
	var subTexts []string
	var subDurs []float64
	prevMp4 := ""
	for _, sh := range shots {
		if done := s.resumeAssetPath(id, sh.Seq, "shot"); done != "" {
			d := ProbeDuration(ctx, done)
			if d <= 0 {
				d = float64(sh.Seconds)
			}
			s.appendLog(id, fmt.Sprintf("Shot %d/%d đã dựng — bỏ qua", sh.Seq+1, len(shots)))
			rendered = append(rendered, renderedShot{Shot: sh, MP4: done, Dur: d})
			if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
				subTexts = append(subTexts, t)
				subDurs = append(subDurs, d)
			}
			prevMp4 = done
			s.setStatus(id, StatusRunning, 10+int(70*float64(len(rendered))/float64(len(shots))))
			continue
		}
		rs, rerr := s.renderFilmShot(ctx, id, p, work, scenes[sh.SceneIdx], charLocks, charRefs, sh, prevMp4, budget, renderMode)
		if rerr != nil {
			if errors.Is(rerr, errBudgetExceeded) {
				fail(rerr)
				return
			}
			s.appendLog(id, fmt.Sprintf("Shot %d/%d lỗi (%v) — bỏ qua", sh.Seq+1, len(shots), rerr))
			continue
		}
		rendered = append(rendered, *rs)
		prevMp4 = rs.MP4
		if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
			subTexts = append(subTexts, t)
			subDurs = append(subDurs, rs.Dur)
		}
		s.setStatus(id, StatusRunning, 10+int(70*float64(len(rendered))/float64(len(shots))))
	}
	if len(rendered) == 0 {
		fail(fmt.Errorf("không dựng được shot nào"))
		return
	}

	// 7. Dựng phim cuối: nối → phụ đề → nhạc → upscale.
	final, err := s.assembleFilmFinal(ctx, id, p, work, rendered, subTexts, subDurs, renderMode)
	if err != nil {
		fail(err)
		return
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))

	// 8. Trailer dọc 9:16 từ shot trailer_worthy (Ninh 2026-10-02).
	s.buildTrailers(ctx, id, p, work, rendered)

	s.fireOnDone(id)
}

// renderFilmShot renders one ≤8s shot and records it as a "shot" asset.
// Film Wave 3 — hai chế độ dựng:
//   - "veo": firstFrame chaining (shot 0 dùng keyframe của chính nó; các
//     shot sau đưa Veo frame cuối của shot trước) → Veo 8s; Veo lỗi thì rơi
//     về điện ảnh từ ảnh, không fail job.
//   - "cinematic" (CHẾ ĐỘ CHÍNH — key Gemini của Ninh không quay được
//     video): keyframe (bố cục cho chuyển động) → chuyển động điện ảnh theo
//     camera_move → voice/silence audio.
func (s *Studio) renderFilmShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget, renderMode string) (*renderedShot, error) {

	orient := orientationWord(p.Aspect)
	aid := s.addAsset(id, sh.Seq, "shot", sh.ImagePrompt)
	mp4 := filepath.Join(work, fmt.Sprintf("shot%04d.mp4", sh.Seq))
	s.setAsset(aid, StatusRunning, "")

	keyPath := filepath.Join(work, fmt.Sprintf("shot%04d.png", sh.Seq))
	// Continuity bible: khối bất di bất dịch của cảnh — kế thừa NGUYÊN VĂN
	// vào mọi prompt, không diễn đạt lại (continuity lock từng khung hình).
	contBlock := ""
	if strings.TrimSpace(sc.Continuity) != "" {
		contBlock = "\nCONTINUITY BIBLE — copy verbatim into the frame, do not paraphrase or alter:\n" +
			strings.TrimSpace(sc.Continuity)
	}
	keyPrompt := sh.ImagePrompt + charLocks + "\n" + sc.SceneLockBlock() + contBlock +
		" Cinematic photorealistic, " + orient + ", no text, no watermark."

	if renderMode == RenderModeVeo {
		if rs, ok, ferr := s.tryVeoShot(ctx, id, p, work, sc, charLocks, charRefs,
			sh, prevMp4, budget, aid, mp4, keyPath, keyPrompt, orient, contBlock); ferr != nil {
			return nil, ferr
		} else if ok {
			return rs, nil
		}
		// Veo lỗi → rơi về điện ảnh từ ảnh (log đã ghi trong tryVeoShot).
	}
	return s.renderCinematicShot(ctx, id, p, work, sc, charLocks, charRefs,
		sh, aid, mp4, keyPath, keyPrompt)
}

// tryVeoShot attempts one ≤8s Veo render with firstFrame chaining.
// (rs, true, nil) = quay được; (nil, false, nil) = Veo lỗi → caller rơi về
// điện ảnh từ ảnh; (nil, false, err) = lỗi nghiêm trọng (vượt trần chi phí).
func (s *Studio) tryVeoShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, prevMp4 string, budget *veoBudget,
	aid int64, mp4, keyPath, keyPrompt, orient, contBlock string) (*renderedShot, bool, error) {

	// FirstFrame: shot đầu dùng keyframe của chính nó; các shot sau dùng
	// frame cuối của shot trước (không cắt được frame → vẽ keyframe mới).
	firstFrame := ""
	if sh.Seq == 0 || prevMp4 == "" {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else if err := extractLastFrame(ctx, prevMp4, keyPath); err != nil {
		if kerr := s.mg.GenerateImage(ctx, keyPrompt, charRefs, keyPath); kerr != nil {
			s.setAsset(aid, StatusFailed, "")
			return nil, false, fmt.Errorf("keyframe: %w", kerr)
		}
		firstFrame = keyPath
	} else {
		firstFrame = keyPath
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Trần chi phí: kiểm tra TRƯỚC mỗi lần gọi Veo (P0-5).
	billed := float64(min(sh.Seconds, veoMaxSeconds))
	if err := budget.check(billed); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, false, fmt.Errorf("%w: %v", errBudgetExceeded, err)
	}
	vprompt := fmt.Sprintf("%s. Camera: %s, %s, %s. %s cinematic film, natural motion, no text.",
		sh.ImagePrompt, sh.ShotSize, sh.CameraMove, sh.LensLight, orient) +
		charLocks + "\n" + sc.SceneLockBlock() + contBlock + performanceBlock(sh)
	s.appendLog(id, fmt.Sprintf("Quay shot %d (%ds)…", sh.Seq+1, sh.Seconds))
	if err := s.mg.GenerateVideo(ctx, vprompt, firstFrame, sh.Seconds, p.Aspect, mp4); err != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: Veo lỗi (%v) — dùng điện ảnh từ ảnh", sh.Seq+1, err))
		return nil, false, nil
	}
	budget.spentSecs += billed
	s.appendLog(id, fmt.Sprintf("Shot %d: Veo ✓ (~$%.2f ước tính)", sh.Seq+1, billed*budget.rate))
	// Veo thành công → key quay được video (capability học từ lần chạy thật).
	_ = s.SetCapability(CapVideoGen, CapOK, fmt.Sprintf("shot %d quay bằng Veo thành công", sh.Seq+1))
	dur := ProbeDuration(ctx, mp4)
	if dur <= 0 {
		dur = float64(sh.Seconds)
	}
	s.setAssetMethod(aid, "veo")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: dur}, true, nil
}

// renderCinematicShot renders one shot in the primary "cinematic stills"
// mode (Film Wave 3): keyframe (bố cục chừa biên cho chuyển động —
// prompts/film_keyframe.txt) → chuyển động điện ảnh theo đúng camera_move
// của đạo diễn → voice/silence audio. Không tốn Veo.
func (s *Studio) renderCinematicShot(ctx context.Context, id string, p FilmParams, work string,
	sc FilmScenePro, charLocks string, charRefs []ImageRef,
	sh RenderShot, aid int64, mp4, keyPath, keyPrompt string) (*renderedShot, error) {

	cinePrompt := keyPrompt + keyframeCompBlock()
	move := strings.ToLower(strings.TrimSpace(sh.CameraMove))
	if move == "" {
		move = "drift"
	}
	s.appendLog(id, fmt.Sprintf("Shot %d: dựng điện ảnh từ ảnh (%s)…", sh.Seq+1, move))
	if kerr := s.mg.GenerateImage(ctx, cinePrompt, charRefs, keyPath); kerr != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("keyframe: %w", kerr)
	}

	// QC chống drift (mặc định no-op trung thực — xem qc.go).
	if s.QC != nil {
		_ = s.QC.CheckShot(ctx, firstPortraitPath(charRefs), keyPath, "")
	}

	// Giọng đọc của shot (TTS từng đoạn ≤800 ký tự — P0-6); lỗi → bản câm.
	wavPath := s.synthShotVoice(ctx, id, sh, work)
	dur := float64(sh.Seconds)
	if wavPath != "" {
		if ws, err := engines.WavSeconds(wavPath); err == nil && ws > dur {
			dur = ws
		}
	}
	tmpV := filepath.Join(work, fmt.Sprintf("shot%04d.cine.mp4", sh.Seq))
	if err := RenderCinematicShot(ctx, CinematicShot{
		ImagePath: keyPath, Seconds: dur,
		CameraMove: sh.CameraMove, Mood: sc.Atmosphere,
	}, tmpV); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("dựng điện ảnh: %w", err)
	}
	defer os.Remove(tmpV)
	if err := AddShotAudio(ctx, tmpV, wavPath, dur, mp4); err != nil {
		s.setAsset(aid, StatusFailed, "")
		return nil, fmt.Errorf("gắn giọng đọc: %w", err)
	}
	d := ProbeDuration(ctx, mp4)
	if d <= 0 {
		d = dur
	}
	s.setAssetMethod(aid, "cinematic")
	s.setAsset(aid, StatusDone, mp4)
	return &renderedShot{Shot: sh, MP4: mp4, Dur: d}, nil
}

// synthShotVoice synthesizes a shot's dialogue + narration in ≤800-char
// chunks (P0-6) and joins them into one WAV. Returns "" when there is
// nothing to say or synthesis fails — fail-soft: silent shot, logged.
func (s *Studio) synthShotVoice(ctx context.Context, id string, sh RenderShot, work string) string {
	speech := shotSpokenText(sh)
	if s.narrator == nil || strings.TrimSpace(speech) == "" {
		return ""
	}
	var wavs [][]byte
	for _, ch := range splitTextChunks(speech, maxTTSChunkChars) {
		w, err := s.narrator(ctx, ch)
		if err != nil || len(w) == 0 {
			s.appendLog(id, fmt.Sprintf("Shot %d: TTS lỗi (%v) — dựng bản câm", sh.Seq+1, err))
			return ""
		}
		wavs = append(wavs, w)
	}
	joined, jerr := tts.ConcatWavs(wavs)
	if jerr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: nối WAV lỗi (%v) — dựng bản câm", sh.Seq+1, jerr))
		return ""
	}
	wavPath := filepath.Join(work, fmt.Sprintf("shot%04d.wav", sh.Seq))
	if werr := os.WriteFile(wavPath, joined, 0o644); werr != nil {
		s.appendLog(id, fmt.Sprintf("Shot %d: không ghi được WAV: %v", sh.Seq+1, werr))
		return ""
	}
	return wavPath
}

// assembleFilmFinal nối các shot → mux phụ đề → mix nhạc bed → upscale
// (nếu chọn) → file cuối trong outDir. Dùng chung cho runFilm và dựng lại
// sau "Quay lại shot này".
//
// Film Wave 3: chế độ "cinematic" dựng bằng xfade (chuyển cảnh mượt) +
// letterbox 2.35:1 + phụ đề theo đúng timeline xfade; chế độ "veo" giữ
// đường nối cứng cũ.
func (s *Studio) assembleFilmFinal(ctx context.Context, id string, p FilmParams, work string,
	rendered []renderedShot, subTexts []string, subDurs []float64, renderMode string) (string, error) {

	var clips []string
	for _, r := range rendered {
		clips = append(clips, r.MP4)
	}
	var cur string
	if renderMode == RenderModeCinematic {
		s.appendLog(id, fmt.Sprintf("Dựng điện ảnh: chuyển cảnh mượt %d shot…", len(clips)))
		var cc []CinematicClip
		for _, r := range rendered {
			cc = append(cc, CinematicClip{Path: r.MP4, CameraMove: r.Shot.CameraMove})
		}
		xfade := filepath.Join(work, "film_xfade.mp4")
		if err := AssembleCinematic(ctx, cc, xfade); err != nil {
			return "", err
		}
		cur = xfade
		// Letterbox 2.35:1 — viền điện ảnh, chỉ ở bản phim cuối (trailer
		// cắt từ shot gốc nên không dính viền).
		lb := filepath.Join(work, "film_letterbox.mp4")
		if err := ApplyLetterbox(ctx, cur, lb); err != nil {
			s.appendLog(id, "⚠ Letterbox lỗi ("+err.Error()+") — giữ bản không viền")
		} else {
			cur = lb
		}
		// Phụ đề theo đúng timeline xfade (shot sau bắt đầu sớm hơn 0.7s).
		if cues := cinematicCues(subTexts, subDurs); len(cues) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(cues)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, formatSRT(cues), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	} else {
		s.appendLog(id, fmt.Sprintf("Nối %d shot…", len(clips)))
		concat := filepath.Join(work, "film_concat.mp4")
		if err := ConcatClips(ctx, clips, p.Aspect, concat); err != nil {
			return "", err
		}
		cur = concat
		// Phụ đề (P1-1): SRT từ thoại + lời dẫn, khớp thời lượng từng shot.
		var st []string
		var sd []float64
		for i, t := range subTexts {
			if strings.TrimSpace(t) != "" && i < len(subDurs) {
				st = append(st, t)
				sd = append(sd, subDurs[i])
			}
		}
		if len(st) > 0 {
			s.appendLog(id, fmt.Sprintf("Mux phụ đề (%d câu)…", len(st)))
			subbed := filepath.Join(work, "film_subs.mp4")
			if err := MuxSubtitles(ctx, cur, engines.BuildSRT(st, sd), subbed); err != nil {
				return "", fmt.Errorf("mux phụ đề: %w", err)
			}
			cur = subbed
		}
	}
	// Nhạc bed (P1-2): duck −8dB khi có voice + loudnorm −14 LUFS.
	if strings.TrimSpace(p.MusicPath) != "" {
		s.appendLog(id, "Mix nhạc nền…")
		mixed := filepath.Join(work, "film_music.mp4")
		if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
			s.appendLog(id, "⚠ Mix nhạc lỗi ("+err.Error()+") — giữ bản không nhạc")
		} else {
			cur = mixed
		}
	}
	// Upscale cuối (P1-5): lanczos — KHÔNG phải 4K native, UI ghi rõ.
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.UpscaleFinal {
		w, h := AspectDims(p.Aspect)
		s.appendLog(id, "Upscale lanczos (không phải 4K native) — bước này lâu…")
		if err := UpscaleVideo(ctx, cur, final, w*2, h*2); err != nil {
			s.appendLog(id, "⚠ Upscale lỗi ("+err.Error()+") — giữ bản gốc")
			if err := copyFile(cur, final); err != nil {
				return "", err
			}
		}
	} else if err := copyFile(cur, final); err != nil {
		return "", err
	}
	return final, nil
}

// buildTrailers cắt tối đa 2 trailer dọc 9:16 từ các shot trailer_worthy
// (Ninh 2026-10-02): center-crop từ phim 16:9, không quay lại. Không có shot
// nào được đánh dấu → bỏ qua, không lỗi.
func (s *Studio) buildTrailers(ctx context.Context, id string, p FilmParams, work string, rendered []renderedShot) {
	var marked []renderedShot
	for _, r := range rendered {
		if r.Shot.TrailerWorthy {
			marked = append(marked, r)
		}
	}
	groups := selectTrailerGroups(marked)
	if len(groups) == 0 {
		s.appendLog(id, "Không có shot trailer_worthy — bỏ qua cắt trailer")
		return
	}
	for gi, g := range groups {
		var vertical []string
		var total float64
		ok := true
		for _, m := range g {
			dst := filepath.Join(work, fmt.Sprintf("trailer%d_%04d.mp4", gi, m.Shot.Seq))
			if err := CropCenterVertical(ctx, m.MP4, dst); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: crop shot %d lỗi (%v) — bỏ trailer này", gi+1, m.Shot.Seq+1, err))
				ok = false
				break
			}
			vertical = append(vertical, dst)
			total += m.Dur
		}
		if !ok || len(vertical) == 0 {
			continue
		}
		// Clips đã 1080x1920 sau crop → ConcatClips "9:16" nối thẳng,
		// không scale thừa.
		raw := filepath.Join(work, fmt.Sprintf("trailer%d_raw.mp4", gi))
		if err := ConcatClips(ctx, vertical, "9:16", raw); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: nối lỗi (%v)", gi+1, err))
			continue
		}
		cur := raw
		if strings.TrimSpace(p.MusicPath) != "" {
			mixed := filepath.Join(work, fmt.Sprintf("trailer%d.mp4", gi))
			if err := MixMusicBed(ctx, cur, p.MusicPath, mixed); err != nil {
				s.appendLog(id, fmt.Sprintf("Trailer %d: mix nhạc lỗi — giữ bản không nhạc", gi+1))
			} else {
				cur = mixed
			}
		}
		final := filepath.Join(s.outDir, fmt.Sprintf("studio-%s-trailer%d.mp4", id, gi+1))
		if err := copyFile(cur, final); err != nil {
			s.appendLog(id, fmt.Sprintf("Trailer %d: lưu lỗi (%v)", gi+1, err))
			continue
		}
		aid := s.addAsset(id, trailerIdxBase+gi, "trailer",
			fmt.Sprintf("Trailer %d — %d shot (%.0fs), crop dọc 9:16 từ phim", gi+1, len(g), total))
		s.setAssetMethod(aid, "trailer")
		s.setAsset(aid, StatusDone, final)
		s.appendLog(id, fmt.Sprintf("Trailer %d xong (%.0fs, 9:16)", gi+1, total))
	}
}

// TrailerShotSeqs returns the render-shot sequence numbers the director
// marked trailer_worthy, recomputed deterministically from the cached
// script.json — the storyboard UI uses it for the trailer badge.
func (s *Studio) TrailerShotSeqs(id string) []int {
	script, err := s.loadCachedScript(filepath.Join(s.workRoot, id))
	if err != nil {
		return nil
	}
	var out []int
	for _, sh := range expandScriptShots(script) {
		if sh.TrailerWorthy {
			out = append(out, sh.Seq)
		}
	}
	return out
}

// RerenderShot quay lại đúng một shot rồi dựng lại phim (storyboard UI /
// P1-3). Chạy đồng bộ — handler web dùng QueueRerenderShot để chạy nền.
func (s *Studio) RerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	j, _ := s.GetJob(id)
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	shots := expandScriptShots(script)
	sh := shots[seq]
	scenes := sceneMap(script)
	var charRefs []ImageRef
	for i := range script.Characters {
		if pp := s.resumeAssetPath(id, portraitIdxBase+i, "portrait"); pp != "" {
			charRefs = append(charRefs, ImageRef{Path: pp})
		}
	}
	prevMp4 := ""
	if seq > 0 {
		prevMp4 = s.resumeAssetPath(id, seq-1, "shot")
	}
	budget := &veoBudget{rate: veoRateUSD()}
	if s.BudgetUSD != nil {
		budget.cap = s.BudgetUSD()
	}
	ctx := context.Background()
	s.appendLog(id, fmt.Sprintf("Quay lại shot %d…", seq+1))
	s.setStatus(id, StatusRunning, 50)
	renderMode := s.resolveRenderMode(p)
	if _, err := s.renderFilmShot(ctx, id, p, work, scenes[sh.SceneIdx], characterLocks(script), charRefs, sh, prevMp4, budget, renderMode); err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	return s.ReassembleFilm(id)
}

// validateRerender checks everything cheap before a shot re-render starts
// (job exists, is a film, idle, seq valid, script cached).
func (s *Studio) validateRerender(id string, seq int) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	if j.Kind != KindFilm {
		return fmt.Errorf("chỉ job phim mới quay lại shot được")
	}
	s.mu.Lock()
	_, already := s.running[id]
	s.mu.Unlock()
	if already {
		return fmt.Errorf("job đang chạy — đợi xong rồi quay lại")
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return fmt.Errorf("không đọc được kịch bản: %w", err)
	}
	if seq < 0 || seq >= len(expandScriptShots(script)) {
		return fmt.Errorf("shot %d không tồn tại", seq+1)
	}
	return nil
}

// QueueRerenderShot validates synchronously, then re-renders the shot and
// re-assembles the film in the background (a Veo shot takes minutes — the
// HTTP handler must not block).
func (s *Studio) QueueRerenderShot(id string, seq int) error {
	if err := s.validateRerender(id, seq); err != nil {
		return err
	}
	go func() {
		if err := s.RerenderShot(id, seq); err != nil {
			s.appendLog(id, "Quay lại shot lỗi: "+err.Error())
			s.setStatus(id, StatusFailed, 100)
		}
	}()
	return nil
}

// ReassembleFilm dựng lại phim từ các shot đã xong (sau "Quay lại shot này").
func (s *Studio) ReassembleFilm(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("job không tồn tại")
	}
	var p FilmParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		return fmt.Errorf("đọc tham số job: %w", err)
	}
	work := filepath.Join(s.workRoot, id)
	script, err := s.loadCachedScript(work)
	if err != nil {
		return err
	}
	shots := expandScriptShots(script)
	ctx := context.Background()
	var rendered []renderedShot
	var subTexts []string
	var subDurs []float64
	for _, sh := range shots {
		mp4 := s.resumeAssetPath(id, sh.Seq, "shot")
		if mp4 == "" {
			return fmt.Errorf("shot %d chưa dựng xong — không dựng lại được", sh.Seq+1)
		}
		d := ProbeDuration(ctx, mp4)
		if d <= 0 {
			d = float64(sh.Seconds)
		}
		rendered = append(rendered, renderedShot{Shot: sh, MP4: mp4, Dur: d})
		if t := shotSpokenText(sh); strings.TrimSpace(t) != "" {
			subTexts = append(subTexts, t)
			subDurs = append(subDurs, d)
		}
	}
	s.appendLog(id, "Dựng lại phim từ các shot…")
	final, err := s.assembleFilmFinal(ctx, id, p, work, rendered, subTexts, subDurs, s.resolveRenderMode(p))
	if err != nil {
		s.setStatus(id, StatusFailed, 100)
		return err
	}
	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Dựng lại xong: "+filepath.Base(final))
	s.buildTrailers(ctx, id, p, work, rendered)
	s.fireOnDone(id)
	return nil
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

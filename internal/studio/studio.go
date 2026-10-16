package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LLM is the text-generation backend for the director. It matches
// engines.LLMProvider so the chain can be passed directly.
type LLM interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
	Name() string
	Healthy(ctx context.Context) bool
}

// Narrator synthesizes narration WAV bytes (TTS chain), or nil for silent.
type Narrator func(ctx context.Context, text, voice string) ([]byte, error)

// Job kinds.
const (
	KindAffiliate = "affiliate"
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
	Mode         string `json:"mode"`                 // photo | shots
	AccountID    int64  `json:"account_id,omitempty"` // set by autopilot
	Niche        string `json:"niche"`
	ProductName  string `json:"product_name"`
	ModelPhoto   string `json:"model_photo"`   // local path (identity lock)
	ProductPhoto string `json:"product_photo"` // local path (product lock)
	// ExtraPhotos: ảnh thêm (góc khác, chi tiết, ảnh listing) — chế độ
	// không key dựng từ cả bộ để video không lặp 1 ảnh.
	ExtraPhotos []string `json:"extra_photos,omitempty"`
	// FixedCaption: chú thích người dùng tự viết (Chỉnh sửa & dựng lại) —
	// có thì dùng nguyên văn, không gọi AI/mẫu.
	FixedCaption string  `json:"fixed_caption,omitempty"`
	Seconds      int     `json:"seconds"`
	BPM          int     `json:"bpm"`         // beat tempo for the giật-giật cut; 0 = 120
	MusicPath    string  `json:"music_path"`  // local audio file, "" = silent
	MusicStart   float64 `json:"music_start"` // seconds into the track
	MusicTitle   string  `json:"music_title"`
	MusicArtist  string  `json:"music_artist"`
	// AffLink: link affiliate Accesstrade của sản phẩm (đợt Q). "" = không
	// có — video vẫn đăng được, chỉ thiếu link kiếm tiền trong mô tả.
	AffLink string `json:"aff_link,omitempty"`
	// Clips: danh sách video clip (tạo từ Muse AI, Gemini video, tải lên)
	Clips []string `json:"clips,omitempty"`
	// VoiceoverPath: file âm thanh lồng tiếng (từ VieNeu-TTS hoặc tải lên)
	VoiceoverPath string `json:"voiceover_path,omitempty"`
	VoiceoverText string `json:"voiceover_text,omitempty"`
	// AspectRatio: tỉ lệ khung hình ("9:16" hoặc "16:9")
	AspectRatio string `json:"aspect_ratio,omitempty"`
	// AssignedDepts: danh sách phòng ban được phân bổ cho job (DEP-01, DEP-02,...)
	AssignedDepts []string `json:"assigned_depts,omitempty"`
}

// HasClips kiểm tra xem job có video clip sẵn (từ Muse AI / video gen) hay không.
func (p AffiliateParams) HasClips() bool {
	for _, c := range p.Clips {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}

// AspectDims maps a job aspect to ffmpeg output dimensions. Unknown or
// empty aspects fall back to 9:16 so old jobs/params keep rendering.
func AspectDims(aspect string) (w, h int) {
	if aspect == "16:9" {
		return 1920, 1080
	}
	return 1080, 1920
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
	// Method is how this asset was really rendered: "veo", "cinematic"
	// (điện ảnh từ ảnh, chế độ chính Wave 3), "anh-tts" (keyframe + TTS
	// fallback, legacy), "trailer". "" = unknown/legacy.
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
	// DiskGuard, nếu khác nil, trả về % đĩa đã dùng của workRoot (Đợt V).
	// insertJob từ chối job mới khi vượt MaxDiskPct (mặc định 95).
	DiskGuard  func() (int, error)
	MaxDiskPct int
	// PromptEnhancer tiêm các kinh nghiệm thực chiến từ Agent Harness vào prompt sinh kịch bản.
	PromptEnhancer func(ctx context.Context, niche, channel string) string
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
	// Đợt Y: bảng chân đăng — idempotency cho auto-publish affiliate.
	// Mỗi (job, platform) ghi trạng thái: "done" (kèm remote_id) hoặc
	// "failed" (lỗi tạm thời, tick sau thử lại). Không có dòng = chưa đăng.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS studio_publish_legs(
		job_id TEXT NOT NULL, platform TEXT NOT NULL,
		remote_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY(job_id, platform))`); err != nil {
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

// OutputPath resolves a job's output filename to its full filesystem path.
func (s *Studio) OutputPath(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		return name
	}
	if s.outDir != "" {
		cand := filepath.Join(s.outDir, name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	if _, err := os.Stat(name); err == nil {
		return name
	}
	if s.outDir != "" {
		return filepath.Join(s.outDir, name)
	}
	return name
}

// ---------------------------------------------------------------------------
// job bookkeeping
// ---------------------------------------------------------------------------

func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Studio) insertJob(kind, title string, params any) (string, error) {
	// Đợt V: pre-flight đĩa — từ chối job mới khi đĩa gần đầy thay vì để
	// job render dở rồi kẹt (video + backup zip có thể lấp đĩa rất nhanh).
	if s.DiskGuard != nil {
		limit := s.MaxDiskPct
		if limit <= 0 {
			limit = 95
		}
		if pct, err := s.DiskGuard(); err == nil && pct >= limit {
			return "", fmt.Errorf("đĩa đã dùng %d%% (ngưỡng %d%%) — dọn đĩa rồi tạo job mới", pct, limit)
		}
	}
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

// PendingJobCount đếm job đang chờ hoặc đang chạy (đợt AA — trang growth
// hiện số này để user biết hàng đợi dài bao nhiêu).
func (s *Studio) PendingJobCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM studio_jobs WHERE status IN (?,?)`,
		StatusQueued, StatusRunning).Scan(&n)
	return n
}

// OnDoneFunc runs after a studio job finishes successfully. It is called
// from the job's goroutine; keep it quick or spawn its own goroutine.

// Shutdown chờ các job đang chạy xong (tối đa timeout) trước khi app thoát.
// Đợt AM: graceful drain — SIGTERM không giết job đang render giữa chừng.
func (s *Studio) Shutdown(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		n := len(s.running)
		s.mu.Unlock()
		if n == 0 || time.Now().After(deadline) {
			if n > 0 {
				// Hết timeout: hủy các job còn lại để không treo shutdown.
				s.mu.Lock()
				cancels := make([]func(), 0, len(s.running))
				ids := make([]string, 0, len(s.running))
				for id, cancel := range s.running {
					cancels = append(cancels, cancel)
					ids = append(ids, id)
				}
				s.mu.Unlock()
				for i, cancel := range cancels {
					cancel()
					s.appendLog(ids[i], "App đang tắt — job bị hủy.")
				}
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

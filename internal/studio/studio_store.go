package studio

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
)

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
	CREATE TABLE IF NOT EXISTS studio_affiliate_plans(
		id TEXT PRIMARY KEY, payload TEXT NOT NULL,
		created_at TEXT NOT NULL);
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

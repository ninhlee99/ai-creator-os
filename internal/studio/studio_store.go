package studio

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxConcurrentStudioJobs = 2
const maxPendingStudioJobs = 20

// jobSlot returns the render semaphore, creating it on first use.
func (s *Studio) jobSlot() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobSem == nil {
		s.jobSem = make(chan struct{}, maxConcurrentStudioJobs)
	}
	return s.jobSem
}

// reserveJobAdmission bounds persisted queued+running work before accepting a
// new goroutine. Count-and-insert are atomic against other in-process callers.
func (s *Studio) reserveJobAdmission(insert func() error) error {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.shutdownStarted.Load() {
		return fmt.Errorf("studio is shutting down")
	}
	pending := 0
	if s.db != nil {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM studio_jobs WHERE status IN (?,?)`, StatusQueued, StatusRunning).Scan(&pending); err != nil {
			return fmt.Errorf("đếm job đang chờ: %w", err)
		}
	}
	pending += s.queuedFlows
	if pending >= maxPendingStudioJobs {
		return fmt.Errorf("hàng đợi Studio đã đầy (%d job); chờ job hoàn tất rồi thử lại", maxPendingStudioJobs)
	}
	return insert()
}

func (s *Studio) reserveFlowAdmission(insert func() error) error {
	s.queueMu.Lock()
	if s.shutdownStarted.Load() {
		s.queueMu.Unlock()
		return fmt.Errorf("studio is shutting down")
	}
	pending := s.queuedFlows
	if s.db != nil {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM studio_jobs WHERE status IN (?,?)`, StatusQueued, StatusRunning).Scan(&pending); err != nil {
			return fmt.Errorf("đếm job đang chờ: %w", err)
		}
		pending += s.queuedFlows
	}
	if pending >= maxPendingStudioJobs {
		s.queueMu.Unlock()
		return fmt.Errorf("hàng đợi Studio đã đầy (%d job); chờ job hoàn tất rồi thử lại", maxPendingStudioJobs)
	}
	// Reserve capacity before releasing queueMu. Run registration may take
	// Studio.mu, while other admission paths can be called with Studio.mu held.
	s.queuedFlows++
	s.queueMu.Unlock()
	if err := insert(); err != nil {
		s.queueMu.Lock()
		s.queuedFlows--
		s.queueMu.Unlock()
		return err
	}
	return nil
}

type renderSlotContextKey struct{}

// WithRenderSlotHeld marks ctx as already holding a renderSlot, preventing nested deadlocks.
func WithRenderSlotHeld(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, renderSlotContextKey{}, true)
}

// IsRenderSlotHeld checks if ctx already holds a renderSlot.
func IsRenderSlotHeld(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	held, _ := ctx.Value(renderSlotContextKey{}).(bool)
	return held
}

func (s *Studio) renderSlot(ctx context.Context) (chan struct{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if IsRenderSlotHeld(ctx) {
		// Slot is already held upstream in this call chain. Provide a pre-filled channel
		// so defer func() { <-sem }() succeeds without re-locking or deadlocking.
		bypass := make(chan struct{}, 1)
		bypass <- struct{}{}
		return bypass, nil
	}
	// Admission bounds accepted work; render semaphore caps active CPU/RAM use.
	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		return sem, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// New opens (or creates) the studio database and returns the orchestrator.
// outDir is where final videos land (served at /media/).
func New(dbPath string, llm LLM, mg MediaGen, narrator Narrator, outDir string) (*Studio, error) {
	dsn := dbPath
	if !strings.Contains(dbPath, "?") {
		dsn = dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-20000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("studio db: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL"} {
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
	CREATE TABLE IF NOT EXISTS studio_tiktok_radar_videos(
		video_id TEXT PRIMARY KEY, payload TEXT NOT NULL,
		collected_at TEXT NOT NULL,
		dance_verified INTEGER NOT NULL DEFAULT 0,
		dance_verified_at TEXT NOT NULL DEFAULT '');
	CREATE INDEX IF NOT EXISTS idx_tiktok_radar_collected
		ON studio_tiktok_radar_videos(collected_at DESC);
	CREATE TABLE IF NOT EXISTS studio_tiktok_radar_snapshots(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id TEXT NOT NULL, provider TEXT NOT NULL,
		captured_at TEXT NOT NULL,
		view_count INTEGER NOT NULL DEFAULT 0, views_known INTEGER NOT NULL DEFAULT 0,
		like_count INTEGER NOT NULL DEFAULT 0, likes_known INTEGER NOT NULL DEFAULT 0,
		comment_count INTEGER NOT NULL DEFAULT 0, comments_known INTEGER NOT NULL DEFAULT 0,
		share_count INTEGER NOT NULL DEFAULT 0, shares_known INTEGER NOT NULL DEFAULT 0,
		UNIQUE(video_id,captured_at));
	CREATE TABLE IF NOT EXISTS studio_tiktok_publications(
		job_id TEXT PRIMARY KEY,
		account_id INTEGER NOT NULL,
		platform_video_id TEXT NOT NULL,
		video_url TEXT NOT NULL,
		linked_at TEXT NOT NULL,
		UNIQUE(account_id, platform_video_id));
	CREATE INDEX IF NOT EXISTS idx_studio_tiktok_publications_account
		ON studio_tiktok_publications(account_id, linked_at DESC);
	CREATE INDEX IF NOT EXISTS idx_tiktok_radar_snapshots_video_time
		ON studio_tiktok_radar_snapshots(video_id,captured_at DESC);
	CREATE INDEX IF NOT EXISTS idx_studio_assets_job ON studio_assets(job_id);`)
	if err != nil {
		return err
	}
	// Additive migrations: tolerate "duplicate column" on re-run.
	for _, q := range []string{
		`ALTER TABLE studio_jobs ADD COLUMN caption TEXT NOT NULL DEFAULT ''`,
		// Per-scene render method ("veo" | "anh-tts" | ""), shown on the
		// storyboard to identify how each TikTok scene was produced.
		`ALTER TABLE studio_assets ADD COLUMN method TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE studio_tiktok_radar_videos ADD COLUMN dance_verified INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE studio_tiktok_radar_videos ADD COLUMN dance_verified_at TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(q); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	// Persist media-generation capability checks.
	if err := s.ensureCapabilitiesTable(); err != nil {
		return err
	}
	// Bảng trạng thái TikTok Draft dùng chung cho Studio và Autopilot.
	// Mỗi (job, platform) ghi trạng thái done, failed, publishing hoặc
	// needs_review để chống gửi trùng và đối chiếu kết quả chưa rõ.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS studio_publish_legs(
		job_id TEXT NOT NULL, platform TEXT NOT NULL,
		remote_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY(job_id, platform))`); err != nil {
		return err
	}

	// Bảng lưu chủ đề Radar và candidate analysis; các hàng đạt điều kiện đứng đầu.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS studio_tiktok_radar_topics(
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS studio_tiktok_radar_topic_videos(
			topic TEXT NOT NULL,
			video_id TEXT NOT NULL,
			rank INTEGER NOT NULL,
			analysis_payload TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			PRIMARY KEY(topic, video_id));
		CREATE INDEX IF NOT EXISTS idx_radar_topic_rank
			ON studio_tiktok_radar_topic_videos(topic, rank ASC);
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE studio_tiktok_radar_topic_videos ADD COLUMN analysis_payload TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return err
	}

	// Khởi tạo các chủ đề mặc định của Việt Nam nếu bảng chủ đề trống
	var topicCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM studio_tiktok_radar_topics`).Scan(&topicCount); err == nil && topicCount == 0 {
		defaultTopics := []string{
			"Xu hướng Việt Nam",
			"Nhảy trend",
			"Thời trang",
			"Làm đẹp",
			"Review ẩm thực",
		}
		now := time.Now().UTC().Format(time.RFC3339)
		for _, t := range defaultTopics {
			_, _ = s.db.Exec(`INSERT INTO studio_tiktok_radar_topics(name, created_at) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, t, now)
		}
	}

	// The dance-filtered default tab is reserved; restore it if an older build
	// allowed deleting it. Keep the existing Vietnamese/English alias as-is.
	rows, err := s.db.Query(`SELECT name FROM studio_tiktok_radar_topics`)
	if err != nil {
		return err
	}
	protectedTopicExists := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		if IsTikTokRadarProtectedDefaultTopic(name) {
			protectedTopicExists = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !protectedTopicExists {
		if _, err := s.db.Exec(`INSERT INTO studio_tiktok_radar_topics(name, created_at) VALUES(?, ?) ON CONFLICT(name) DO NOTHING`, "Xu hướng Việt Nam", time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}

	return s.migrateManualDanceVerification()
}

// migrateManualDanceVerification clears historical crawler auto-labels once.
// Those values do not prove an operator reviewed the video.
func (s *Studio) migrateManualDanceVerification() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= 2 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE studio_tiktok_radar_videos SET dance_verified=0,dance_verified_at=''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}

// Close releases the database handle.

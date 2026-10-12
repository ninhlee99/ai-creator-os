package reup

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

// Store lưu nguồn + video reup (SQLite WAL, một DB một nhiệm vụ: reup.db).
type Store struct {
	db *sql.DB
}

// schemaVersion hiện tại; migration qua PRAGMA user_version.
// v1 (Đợt D): reup_sources + reup_videos + reup_posts (posts để Đợt E dùng).
// v2 (Đợt E): reup_posts thêm video_ids, file_path, fail_reason, views,
// metrics_at, remote_id, remote_url (transform + đăng + kill rule 0-view).
// v3 (Đợt V): reup_posts thêm updated_at — sweeper reset bài kẹt "posting".
const schemaVersion = 3

// NewStore mở (hoặc tạo) reup.db trong dataDir.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("reup: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("reup: pragma %q: %w", p, err)
		}
	}
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		db.Close()
		return nil, fmt.Errorf("reup: pragma user_version: %w", err)
	}
	if cur < schemaVersion {
		if err := migrate(db, cur); err != nil {
			db.Close()
			return nil, err
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			db.Close()
			return nil, fmt.Errorf("reup: stamp user_version: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB, from int) error {
	if from < 1 {
		if err := migrateV1(db); err != nil {
			return err
		}
	}
	if from < 2 {
		if err := migrateV2(db); err != nil {
			return err
		}
	}
	if from < 3 {
		if err := migrateV3(db); err != nil {
			return err
		}
	}
	return nil
}

func migrateV1(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS reup_sources(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,            -- user | hashtag
		value TEXT NOT NULL,           -- unique_id Douyin hoặc tên hashtag
		display_name TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		added_at TEXT NOT NULL,
		UNIQUE(kind, value));
	CREATE TABLE IF NOT EXISTS reup_videos(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id INTEGER NOT NULL DEFAULT 0,
		douyin_id TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		file_path TEXT NOT NULL DEFAULT '',
		sha256 TEXT NOT NULL DEFAULT '',
		duration_sec REAL NOT NULL DEFAULT 0,
		resolution TEXT NOT NULL DEFAULT '',
		play_count INTEGER NOT NULL DEFAULT 0,
		author TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		watermark_free INTEGER NOT NULL DEFAULT 0,
		via TEXT NOT NULL DEFAULT '',  -- ytdlp | tikwm
		status TEXT NOT NULL DEFAULT 'queued', -- queued|downloading|downloaded|failed
		fail_reason TEXT NOT NULL DEFAULT '',
		qc_method TEXT NOT NULL DEFAULT '',
		downloaded_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_douyin ON reup_videos(douyin_id);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_sha ON reup_videos(sha256);
	CREATE INDEX IF NOT EXISTS idx_reup_videos_status ON reup_videos(status);
	-- Đợt E dùng: mỗi video → transform → bài đăng per kênh/nền tảng.
	CREATE TABLE IF NOT EXISTS reup_posts(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id INTEGER NOT NULL,
		account_ref TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '',
		transform_level INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending',
		posted_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		FOREIGN KEY(video_id) REFERENCES reup_videos(id));`)
	if err != nil {
		return fmt.Errorf("reup: migrate: %w", err)
	}
	return nil
}

// Close đóng DB.
func (s *Store) Close() error { return s.db.Close() }

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------------------------------------------------------------- sources

// Source là một nguồn Douyin (user hoặc hashtag).

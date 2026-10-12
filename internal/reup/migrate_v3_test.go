package reup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateV3Backfill(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reup.db")
	db, _ := sql.Open("sqlite", p)
	// Giả lập DB v2: tạo 3 bảng kiểu cũ (không có updated_at / last_error).
	db.Exec(`CREATE TABLE reup_sources(id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL, value TEXT NOT NULL, display_name TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1, added_at TEXT NOT NULL,
		UNIQUE(kind, value))`)
	db.Exec(`CREATE TABLE reup_videos(id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id INTEGER NOT NULL DEFAULT 0, douyin_id TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '', file_path TEXT NOT NULL DEFAULT '',
		sha256 TEXT NOT NULL DEFAULT '', duration_sec REAL NOT NULL DEFAULT 0,
		resolution TEXT NOT NULL DEFAULT '', play_count INTEGER NOT NULL DEFAULT 0,
		author TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
		watermark_free INTEGER NOT NULL DEFAULT 0, via TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'queued', fail_reason TEXT NOT NULL DEFAULT '',
		qc_method TEXT NOT NULL DEFAULT '', downloaded_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL)`)
	db.Exec(`CREATE TABLE reup_posts(id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id INTEGER NOT NULL, account_ref TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '', transform_level INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending', posted_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL)`)
	db.Exec(`PRAGMA user_version = 2`)
	// Dữ liệu mẫu: 1 bài posting cũ + 2 video trùng douyin_id (đúng bug
	// v4 phải dọn) + 1 video downloading kẹt + 1 nguồn user.
	db.Exec(`INSERT INTO reup_posts(video_id, status, created_at) VALUES(1,'posting','2026-10-01T10:00:00')`)
	db.Exec(`INSERT INTO reup_videos(douyin_id, status, created_at) VALUES('dup1','downloaded','2026-10-01T09:00:00')`)
	db.Exec(`INSERT INTO reup_videos(douyin_id, status, created_at) VALUES('dup1','downloaded','2026-10-01T09:05:00')`)
	db.Exec(`INSERT INTO reup_videos(douyin_id, status, created_at) VALUES('stuck1','downloading','2026-10-01T08:00:00')`)
	db.Exec(`INSERT INTO reup_sources(kind, value, added_at) VALUES('user','thantien','2026-10-01T08:00:00')`)
	db.Close()

	s, err := NewStore(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var updated string
	s.db.QueryRow(`SELECT updated_at FROM reup_posts WHERE id=1`).Scan(&updated)
	if updated != "2026-10-01T10:00:00" {
		t.Fatalf("backfill sai: %q", updated)
	}
	// Post kẹt posting từ 3h trước → sweep phải reset.
	s.db.Exec(`UPDATE reup_posts SET updated_at='2026-10-12T00:00:00', status='posting' WHERE id=1`)
	n, err := s.SweepStuckPosting(2 * 3600 * 1000000000)
	if err != nil || n != 1 {
		t.Fatalf("sweep: n=%d err=%v", n, err)
	}
	var st string
	s.db.QueryRow(`SELECT status FROM reup_posts WHERE id=1`).Scan(&st)
	if st != "transformed" {
		t.Fatalf("status sau sweep: %q", st)
	}
	// v4: video trùng douyin_id đã bị dọn (giữ 1), video còn lại có updated_at.
	var dupN int
	s.db.QueryRow(`SELECT COUNT(*) FROM reup_videos WHERE douyin_id='dup1'`).Scan(&dupN)
	if dupN != 1 {
		t.Fatalf("v4 dedupe: còn %d bản dup1", dupN)
	}
	var vUpd string
	s.db.QueryRow(`SELECT updated_at FROM reup_videos WHERE douyin_id='dup1'`).Scan(&vUpd)
	if vUpd != "2026-10-01T09:00:00" {
		t.Fatalf("v4 backfill updated_at: %q", vUpd)
	}
	// v4: unique index chặn insert trùng.
	if _, err := s.db.Exec(`INSERT INTO reup_videos(douyin_id, created_at) VALUES('dup1','x')`); err == nil {
		t.Fatal("v4 unique index không chặn trùng douyin_id")
	}
	// v4: nguồn có cột last_error, ghi/đọc được.
	if err := s.SetSourceLastError(1, "lỗi test"); err != nil {
		t.Fatal(err)
	}
	src, err := s.GetSource("user", "thantien")
	if err != nil || src.LastError != "lỗi test" {
		t.Fatalf("last_error: src=%+v err=%v", src, err)
	}
	// Video kẹt downloading từ hôm qua → sweep reset về queued.
	s.db.Exec(`UPDATE reup_videos SET updated_at='2026-10-11T00:00:00', status='downloading' WHERE douyin_id='stuck1'`)
	dn, err := s.SweepStuckDownloading(2 * 3600 * 1000000000)
	if err != nil || dn != 1 {
		t.Fatalf("sweep downloading: n=%d err=%v", dn, err)
	}
	s.db.QueryRow(`SELECT status FROM reup_videos WHERE douyin_id='stuck1'`).Scan(&st)
	if st != "queued" {
		t.Fatalf("status video sau sweep: %q", st)
	}
	os.Remove(p)
}

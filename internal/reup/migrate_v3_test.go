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
	// Giả lập DB v2: tạo bảng posts kiểu cũ (không có updated_at).
	db.Exec(`CREATE TABLE reup_posts(id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id INTEGER NOT NULL, account_ref TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '', transform_level INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending', posted_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL)`)
	db.Exec(`INSERT INTO reup_posts(video_id, status, created_at) VALUES(1,'posting','2026-10-01T10:00:00')`)
	db.Exec(`PRAGMA user_version = 2`)
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
	os.Remove(p)
}

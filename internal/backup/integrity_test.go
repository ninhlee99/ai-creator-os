package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestIntegrityCheckOK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec("CREATE TABLE x(a)")
	db.Close()
	if err := IntegrityCheck(p); err != nil {
		t.Fatalf("DB tốt phải pass: %v", err)
	}
}

func TestIntegrityCheckCorrupt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec("CREATE TABLE x(a)")
	db.Close()
	// Làm hỏng phần giữa file (giữ nguyên header để bẫy Validate cũ).
	f, _ := os.OpenFile(p, os.O_WRONLY, 0)
	f.WriteAt([]byte("XXXXCORRUPTXXXX"), 100)
	f.Close()
	if err := IntegrityCheck(p); err == nil {
		t.Fatalf("DB hỏng phải bị phát hiện")
	} else {
		t.Logf("phát hiện đúng: %v", err)
	}
}

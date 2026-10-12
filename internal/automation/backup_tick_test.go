package automation

// Đợt M2: tự sao lưu mỗi ngày.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func backupTestService(t *testing.T) *Service {
	t.Helper()
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	svc.DataDir = t.TempDir()
	// Tạo DB SQLite thật để backup có gì đó để zip (Đợt V: BackupTick kiểm
	// tra integrity trước khi zip nên file giả "fake-db" không qua được).
	db, err := sql.Open("sqlite", filepath.Join(svc.DataDir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE t(a)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return svc
}

func TestBackupTickCreatesZip(t *testing.T) {
	svc := backupTestService(t)
	notes := svc.BackupTick(context.Background())
	if len(notes) != 1 || !strings.Contains(notes[0], "sao lưu tự động:") {
		t.Fatalf("notes=%q, want 1 note sao lưu", notes)
	}
	entries, err := os.ReadDir(filepath.Join(svc.DataDir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".zip") {
		t.Fatalf("entries=%v, want 1 file zip", entries)
	}
	if got := BackupLastOK(svc.Settings); got == "" {
		t.Fatal("BackupLastOK phải được ghi")
	}
	// chạy lại ngay → im lặng (interval 1 ngày)
	if notes := svc.BackupTick(context.Background()); len(notes) != 0 {
		t.Fatalf("lần 2 phải im lặng, got %q", notes)
	}
}

func TestBackupTickRespectsKillAndDryRun(t *testing.T) {
	svc := backupTestService(t)
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.BackupTick(context.Background()); len(notes) != 0 {
		t.Fatalf("kill phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.BackupTick(context.Background()); len(notes) != 0 {
		t.Fatalf("dry-run phải chặn, got %q", notes)
	}
}

func TestPruneBackupsKeepsN(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"aicos-backup-20260101-000000.zip", "aicos-backup-20260102-000000.zip", "aicos-backup-20260103-000000.zip", "khac.zip"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if kept := pruneBackups(dir, 2); kept != 2 {
		t.Fatalf("kept=%d, want 2", kept)
	}
	if _, err := os.Stat(filepath.Join(dir, "aicos-backup-20260101-000000.zip")); !os.IsNotExist(err) {
		t.Fatal("bản cũ nhất phải bị xóa")
	}
	if _, err := os.Stat(filepath.Join(dir, "khac.zip")); err != nil {
		t.Fatal("file lạ không được đụng")
	}
}

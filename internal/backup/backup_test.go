package backup

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func seedData(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	// DB thật (SQLite) — Stage() integrity_check mọi *.db nên file giả
	// chỉ có magic header sẽ không qua được.
	for _, db := range []string{"ledger.db", "studio.db", "products.db", "accesstrade.db"} {
		p := filepath.Join(dir, db)
		sdb, err := sql.Open("sqlite", p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sdb.Exec("CREATE TABLE kv(k TEXT PRIMARY KEY, v TEXT)"); err != nil {
			t.Fatal(err)
		}
		if _, err := sdb.Exec("INSERT INTO kv(k,v) VALUES (?,?)", db, db+"-v1"); err != nil {
			t.Fatal(err)
		}
		sdb.Close()
	}
	files := map[string][]byte{
		"content_jobs.json": []byte(`{"jobs":[]}`),
		"tokens/a.json":     []byte(`{"refresh_token":"r"}`),
	}
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Đọc lại bytes DB thật để so sánh sau round-trip.
	out := map[string][]byte{}
	for _, db := range []string{"ledger.db", "studio.db", "products.db", "accesstrade.db"} {
		raw, err := os.ReadFile(filepath.Join(dir, db))
		if err != nil {
			t.Fatal(err)
		}
		out[db] = raw
	}
	for rel, body := range files {
		out[rel] = body
	}
	return out
}

func openZip(t *testing.T, raw []byte) *zip.Reader {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

// Vòng sao lưu → xoá → khôi phục: dữ liệu quay lại nguyên vẹn.
func TestBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	orig := seedData(t, dir)

	var buf bytes.Buffer
	if err := Create(dir, &buf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	zr := openZip(t, buf.Bytes())
	if err := Validate(zr); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Giả lập mất dữ liệu: xoá token, ghi đè ledger.
	os.Remove(filepath.Join(dir, "tokens", "a.json"))
	os.WriteFile(filepath.Join(dir, "ledger.db"), []byte("SQLite format 3\x00CORRUPT"), 0o600)

	if err := Stage(dir, openZip(t, buf.Bytes())); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if !PendingRestore(dir) {
		t.Fatal("thiếu cờ RESTORE_PENDING sau Stage")
	}
	// Dữ liệu thật vẫn chưa bị đụng tới ở bước Stage.
	if _, err := os.Stat(filepath.Join(dir, "tokens", "a.json")); !os.IsNotExist(err) {
		t.Fatal("Stage đã đụng vào dữ liệu thật")
	}
	if err := ApplyStaged(dir); err != nil {
		t.Fatalf("ApplyStaged: %v", err)
	}
	if PendingRestore(dir) {
		t.Fatal("cờ RESTORE_PENDING chưa được xoá")
	}
	for rel, want := range orig {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("thiếu %s sau khôi phục", rel)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s không khớp bản gốc sau khôi phục", rel)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	// Zip không có manifest.
	var b1 bytes.Buffer
	zw := zip.NewWriter(&b1)
	fw, _ := zw.Create("ledger.db")
	fw.Write(append([]byte("SQLite format 3\x00"), []byte("x")...))
	zw.Close()
	if err := Validate(openZip(t, b1.Bytes())); err == nil {
		t.Fatal("chấp nhận zip thiếu manifest")
	}

	// ledger.db không phải SQLite.
	var b2 bytes.Buffer
	zw2 := zip.NewWriter(&b2)
	fw2, _ := zw2.Create(ManifestName)
	fw2.Write([]byte(`{}`))
	fw3, _ := zw2.Create("ledger.db")
	fw3.Write([]byte("not a database at all"))
	zw2.Close()
	if err := Validate(openZip(t, b2.Bytes())); err == nil {
		t.Fatal("chấp nhận ledger.db không phải SQLite")
	}

	// Zip-slip.
	var b3 bytes.Buffer
	zw3 := zip.NewWriter(&b3)
	fw4, _ := zw3.Create(ManifestName)
	fw4.Write([]byte(`{}`))
	fw5, _ := zw3.Create("ledger.db")
	fw5.Write(append([]byte("SQLite format 3\x00"), []byte("x")...))
	fw6, _ := zw3.Create("../evil.sh")
	fw6.Write([]byte("x"))
	zw3.Close()
	if err := Validate(openZip(t, b3.Bytes())); err == nil {
		t.Fatal("chấp nhận entry zip-slip")
	}
}

func TestCreateEmptyDirFails(t *testing.T) {
	var buf bytes.Buffer
	if err := Create(t.TempDir(), &buf); err == nil {
		t.Fatal("sao lưu thư mục trống phải báo lỗi")
	}
}

func TestApplyStagedNoop(t *testing.T) {
	if err := ApplyStaged(t.TempDir()); err != nil {
		t.Fatalf("ApplyStaged không có gì chờ phải no-op: %v", err)
	}
}

// TestCreateIncludesAllFourDBs: ảnh chụp phải gồm đủ 4 DB đang mở
// (ledger.db, studio.db, products.db, accesstrade.db).
func TestCreateIncludesAllFourDBs(t *testing.T) {
	dir := t.TempDir()
	seedData(t, dir)
	var buf bytes.Buffer
	if err := Create(dir, &buf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	zr := openZip(t, buf.Bytes())
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, db := range []string{"ledger.db", "studio.db", "products.db", "accesstrade.db"} {
		if !names[db] {
			t.Errorf("thiếu %s trong ảnh chụp", db)
		}
	}
}

// Đợt R: checkpoint chạy best-effort trên DB thật đang mở (WAL) — không
// lỗi, và DB vẫn đọc được sau checkpoint.
func TestCheckpointLiveWALDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Skipf("không mở được sqlite test: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t (v) VALUES ('x')"); err != nil {
		t.Fatal(err)
	}
	checkpointDB(path) // best-effort: không được panic/lỗi lan ra ngoài
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM t").Scan(&n); err != nil || n != 1 {
		t.Fatalf("DB hỏng sau checkpoint: n=%d err=%v", n, err)
	}
}

// Đợt fix backup: Stage phải từ chối bản sao lưu chứa DB hỏng (dù header
// còn nguyên — Validate cũ bỏ lọt), không đặt cờ RESTORE_PENDING.
func TestStageRejectsCorruptDB(t *testing.T) {
	dir := t.TempDir()
	seedData(t, dir)
	// Làm hỏng studio.db trong data dir rồi zip lại thành "bản sao lưu hỏng".
	p := filepath.Join(dir, "studio.db")
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("XXXXCORRUPTXXXX"), 100); err != nil {
		t.Fatal(err)
	}
	f.Close()
	var buf bytes.Buffer
	if err := Create(dir, &buf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	zr := openZip(t, buf.Bytes())
	err = Stage(dir, zr)
	if err == nil {
		t.Fatal("Stage phải từ chối bản chứa DB hỏng")
	}
	if !strings.Contains(err.Error(), "studio.db") {
		t.Fatalf("lỗi phải nêu tên file hỏng, got: %v", err)
	}
	if PendingRestore(dir) {
		t.Fatal("không được đặt cờ RESTORE_PENDING khi stage thất bại")
	}
	if _, serr := os.Stat(filepath.Join(dir, stagingDir)); !os.IsNotExist(serr) {
		t.Fatal("thư mục staging phải được dọn khi stage thất bại")
	}
	// Dữ liệu hiện tại không bị đụng tới.
	if _, serr := os.Stat(filepath.Join(dir, "ledger.db")); serr != nil {
		t.Fatal("Stage thất bại không được đụng dữ liệu hiện tại")
	}
}

// CreateValidated: pipeline dùng chung cho nút bấm tay và BackupTick.
func TestCreateValidatedOK(t *testing.T) {
	dir := t.TempDir()
	seedData(t, dir)
	raw, err := CreateValidated(dir)
	if err != nil {
		t.Fatalf("CreateValidated: %v", err)
	}
	if err := ValidateZipBytes(raw); err != nil {
		t.Fatalf("ValidateZipBytes: %v", err)
	}
}

func TestCreateValidatedCorruptSource(t *testing.T) {
	dir := t.TempDir()
	seedData(t, dir)
	p := filepath.Join(dir, "ledger.db")
	f, _ := os.OpenFile(p, os.O_WRONLY, 0)
	_, _ = f.WriteAt([]byte("XXXXCORRUPTXXXX"), 100)
	f.Close()
	_, err := CreateValidated(dir)
	if err == nil {
		t.Fatal("DB nguồn hỏng phải chặn tạo bản sao lưu")
	}
	var cse *CorruptSourceError
	if !errors.As(err, &cse) {
		t.Fatalf("lỗi phải là *CorruptSourceError, got %T: %v", err, err)
	}
	if cse.DB != "ledger.db" {
		t.Fatalf("cse.DB=%q, want ledger.db", cse.DB)
	}
}

func TestCheckSourceDBsSkipsMissing(t *testing.T) {
	if err := CheckSourceDBs(t.TempDir()); err != nil {
		t.Fatalf("thư mục trống (DB chưa có) phải bỏ qua, got: %v", err)
	}
}

func TestValidateZipBytesRejects(t *testing.T) {
	if err := ValidateZipBytes([]byte("không phải zip")); err == nil {
		t.Fatal("bytes rác phải bị từ chối")
	}
}

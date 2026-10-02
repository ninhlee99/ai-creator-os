package backup

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func seedData(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{
		"ledger.db":         append([]byte("SQLite format 3\x00"), []byte("ledger-v1")...),
		"studio.db":         append([]byte("SQLite format 3\x00"), []byte("studio-v1")...),
		"products.db":       append([]byte("SQLite format 3\x00"), []byte("products-v1")...),
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
	return files
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

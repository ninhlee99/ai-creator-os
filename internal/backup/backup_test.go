package backup

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
	_ "modernc.org/sqlite"
)

func seedData(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	// DB thật (SQLite) — Stage() integrity_check mọi *.db nên file giả
	// chỉ có magic header sẽ không qua được.
	for _, db := range []string{"ledger.db", "studio.db", "products.db"} {
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
	webaiPath := filepath.Join(dir, "webai.db")
	webaiDB, err := sql.Open("sqlite", webaiPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := webaiDB.Exec(`CREATE TABLE credentials(cookie TEXT); INSERT INTO credentials VALUES('bearer-secret')`); err != nil {
		webaiDB.Close()
		t.Fatal(err)
	}
	_ = webaiDB.Close()
	files := map[string][]byte{
		"content_jobs.json": []byte(`{"jobs":[]}`),
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
	for _, db := range []string{"ledger.db", "studio.db", "products.db"} {
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

func TestArchiveLimitWriterStopsAtLimit(t *testing.T) {
	var dst bytes.Buffer
	w := &archiveLimitWriter{writer: &dst, limit: 3}

	n, err := w.Write([]byte("four"))
	if n != 3 || !errors.Is(err, errBackupArchiveTooLarge) {
		t.Fatalf("Write() = (%d, %v), want (3, archive too large)", n, err)
	}
	if got := dst.String(); got != "fou" {
		t.Fatalf("written bytes = %q, want %q", got, "fou")
	}
	if n, err := w.Write([]byte("x")); n != 0 || !errors.Is(err, errBackupArchiveTooLarge) {
		t.Fatalf("Write() after limit = (%d, %v), want (0, archive too large)", n, err)
	}
}

func writeValidManifest(t *testing.T, zw *zip.Writer) {
	t.Helper()
	f, err := zw.Create(ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(`{"app":"aicos","created":"2026-10-28T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
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
		if strings.HasSuffix(rel, ".db") {
			restored, err := sql.Open("sqlite", filepath.Join(dir, rel))
			if err != nil {
				t.Fatalf("mở %s sau khôi phục: %v", rel, err)
			}
			var got string
			err = restored.QueryRow(`SELECT v FROM kv WHERE k=?`, rel).Scan(&got)
			_ = restored.Close()
			if err != nil || got != rel+"-v1" {
				t.Fatalf("%s không khớp sau khôi phục: got=%q err=%v", rel, got, err)
			}
			continue
		}
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("thiếu %s sau khôi phục", rel)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s không khớp bản gốc sau khôi phục", rel)
		}
	}
}

func TestBackupRedactsLedgerCredentialsAndTokenFiles(t *testing.T) {
	dir := t.TempDir()
	seedData(t, dir)
	ledger, err := sql.Open("sqlite", filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"env:GEMINI_API_KEY":    "secret-env-key",
		"at.access_key":         "secret-at-key",
		"auth.password_hash":    "offline-crackable-hash",
		"llm.chain":             `{"order":[{"name":"gemini","api_key":"secret-json-key","headers":{"Authorization":"Bearer secret-authz","private_key":"secret-private","jwt":"secret-jwt"},"auth":{"type":"bearer","value":"secret-auth-object"},"enabled":true}]}`,
		"env:GEMINI_TEXT_MODEL": "gemini-2.5-pro",
		"automation.master":     "1",
	} {
		if _, err := ledger.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	_ = ledger.Close()
	tokenDir := filepath.Join(dir, "tokens")
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tokenDir, "refresh.json"), []byte(`{"refresh_token":"secret-refresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := Create(dir, &archive); err != nil {
		t.Fatal(err)
	}
	zr := openZip(t, archive.Bytes())
	var ledgerBytes []byte
	for _, file := range zr.File {
		if file.Name == "tokens/refresh.json" {
			t.Fatal("backup included token file")
		}
		if file.Name == "ledger.db" {
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			ledgerBytes, err = io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, secret := range []string{"secret-env-key", "secret-at-key", "secret-json-key", "secret-authz", "secret-private", "secret-jwt", "secret-auth-object", "offline-crackable-hash"} {
		if bytes.Contains(ledgerBytes, []byte(secret)) {
			t.Fatal("backup ledger contains credential material")
		}
	}
	safeDBPath := filepath.Join(t.TempDir(), "ledger.db")
	if err := os.WriteFile(safeDBPath, ledgerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	safeDB, err := sql.Open("sqlite", safeDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer safeDB.Close()
	var ops string
	if err := safeDB.QueryRow(`SELECT value FROM settings WHERE key='automation.master'`).Scan(&ops); err != nil || ops != "1" {
		t.Fatalf("safe setting lost: value=%q err=%v", ops, err)
	}
	var chain string
	if err := safeDB.QueryRow(`SELECT value FROM settings WHERE key='llm.chain'`).Scan(&chain); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(chain, "secret-json-key") || !strings.Contains(chain, "gemini") {
		t.Fatalf("chain backup should retain provider config but redact API key: %s", chain)
	}
	var model string
	if err := safeDB.QueryRow(`SELECT value FROM settings WHERE key='env:GEMINI_TEXT_MODEL'`).Scan(&model); err != nil || model != "gemini-2.5-pro" {
		t.Fatalf("non-secret environment setting should survive backup: value=%q err=%v", model, err)
	}
}

func TestRedactJSONCredentialsHandlesEmbeddedJSON(t *testing.T) {
	got := redactJSONCredentials(map[string]any{
		"payload": `{"auth":{"access_token":"nested-secret"},"private_key":"nested-private","model":"keep"}`,
		"plain":   "keep this text",
	})
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"nested-secret", "nested-private"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("nested JSON credential %q survived redaction: %s", secret, encoded)
		}
	}
	if !bytes.Contains(encoded, []byte("keep this text")) || !bytes.Contains(encoded, []byte("keep")) {
		t.Fatalf("non-secret fields should survive redaction: %s", encoded)
	}
}

func TestBackupRestorePreservesStudioInputsAndPortableModelPhotos(t *testing.T) {
	source := t.TempDir()
	seedData(t, source)
	webStore, err := webai.NewStore(filepath.Join(source, "webai.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := webStore.AddAccount(webai.Account{Platform: "museai", AccountName: "Test", SessionKey: "bearer-secret"}); err != nil {
		webStore.Close()
		t.Fatal(err)
	}
	_ = webStore.Close()
	modelPath := filepath.Join(source, "output", "studio", "models", "7", "character.jpg")
	uploadPath := filepath.Join(source, "output", "studio", "uploads", "product.jpg")
	for path, body := range map[string][]byte{
		modelPath:  []byte("character-reference"),
		uploadPath: []byte("product-reference"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(source, "output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "output", "rendered.mp4"), []byte("generated-output"), 0o600); err != nil {
		t.Fatal(err)
	}
	studioDB, err := sql.Open("sqlite", filepath.Join(source, "studio.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := studioDB.Exec(`CREATE TABLE model_photos(id INTEGER PRIMARY KEY, account_id INTEGER, path TEXT, created_at TEXT)`); err != nil {
		studioDB.Close()
		t.Fatal(err)
	}
	if _, err := studioDB.Exec(`INSERT INTO model_photos(id,account_id,path,created_at) VALUES(1,7,?,?)`, modelPath, "2026-10-28T00:00:00Z"); err != nil {
		studioDB.Close()
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE studio_jobs(id TEXT PRIMARY KEY, params TEXT NOT NULL)`,
		`CREATE TABLE studio_affiliate_plans(id TEXT PRIMARY KEY, payload TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE studio_workflow_graphs(id TEXT PRIMARY KEY, graph_json TEXT NOT NULL)`,
	} {
		if _, err := studioDB.Exec(query); err != nil {
			studioDB.Close()
			t.Fatal(err)
		}
	}
	if _, err := studioDB.Exec(`INSERT INTO studio_jobs(id,params) VALUES('viral-1',?)`, `{"model_photo":"`+modelPath+`","sample_video":"`+uploadPath+`"}`); err != nil {
		studioDB.Close()
		t.Fatal(err)
	}
	if _, err := studioDB.Exec(`INSERT INTO studio_affiliate_plans(id,payload,created_at) VALUES('plan-1',?,?)`, `{"params":{"model_photo":"`+modelPath+`","product_photo":"`+uploadPath+`"}}`, "2026-10-28T00:00:00Z"); err != nil {
		studioDB.Close()
		t.Fatal(err)
	}
	if _, err := studioDB.Exec(`INSERT INTO studio_workflow_graphs(id,graph_json) VALUES('graph-1',?)`, `{"nodes":[{"params":{"model_path":"`+modelPath+`","product_image":"`+uploadPath+`"}}]}`); err != nil {
		studioDB.Close()
		t.Fatal(err)
	}
	if err := studioDB.Close(); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := Create(source, &archive); err != nil {
		t.Fatalf("Create: %v", err)
	}
	zr := openZip(t, archive.Bytes())
	entries := make(map[string]bool, len(zr.File))
	for _, entry := range zr.File {
		entries[entry.Name] = true
	}
	if entries["webai.db"] {
		t.Fatal("plaintext backup must exclude WebAI database containing bearer credentials")
	}
	if !entries["output/studio/models/7/character.jpg"] || !entries["output/studio/uploads/product.jpg"] {
		t.Fatalf("backup omitted studio inputs: %v", entries)
	}
	if entries["output/rendered.mp4"] {
		t.Fatal("backup must still omit generated render output")
	}

	target := t.TempDir()
	if err := Stage(target, openZip(t, archive.Bytes())); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := ApplyStaged(target); err != nil {
		t.Fatalf("ApplyStaged: %v", err)
	}
	for _, check := range []struct{ rel, want string }{
		{"output/studio/models/7/character.jpg", "character-reference"},
		{"output/studio/uploads/product.jpg", "product-reference"},
	} {
		got, err := os.ReadFile(filepath.Join(target, check.rel))
		if err != nil || string(got) != check.want {
			t.Fatalf("restored %s = %q, err=%v", check.rel, got, err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(target, "studio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var storedPath string
	if err := db.QueryRow(`SELECT path FROM model_photos WHERE id=1`).Scan(&storedPath); err != nil {
		t.Fatal(err)
	}
	if storedPath != "models/7/character.jpg" {
		t.Fatalf("restored model path = %q, want portable relative path", storedPath)
	}
	wantModel := filepath.Join(target, "output", "studio", "models", "7", "character.jpg")
	wantUpload := filepath.Join(target, "output", "studio", "uploads", "product.jpg")
	for _, check := range []struct{ table, column, id string }{
		{"studio_jobs", "params", "viral-1"},
		{"studio_affiliate_plans", "payload", "plan-1"},
		{"studio_workflow_graphs", "graph_json", "graph-1"},
	} {
		var raw string
		query := `SELECT ` + check.column + ` FROM ` + check.table + ` WHERE id=?`
		if err := db.QueryRow(query, check.id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(raw, wantModel) || !strings.Contains(raw, wantUpload) {
			t.Errorf("%s.%s paths were not relocated: %s", check.table, check.column, raw)
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
	writeValidManifest(t, zw2)
	fw3, _ := zw2.Create("ledger.db")
	fw3.Write([]byte("not a database at all"))
	zw2.Close()
	if err := Validate(openZip(t, b2.Bytes())); err == nil {
		t.Fatal("chấp nhận ledger.db không phải SQLite")
	}

	// Zip-slip.
	var b3 bytes.Buffer
	zw3 := zip.NewWriter(&b3)
	writeValidManifest(t, zw3)
	fw5, _ := zw3.Create("ledger.db")
	fw5.Write(append([]byte("SQLite format 3\x00"), []byte("x")...))
	fw6, _ := zw3.Create("../evil.sh")
	fw6.Write([]byte("x"))
	zw3.Close()
	if err := Validate(openZip(t, b3.Bytes())); err == nil {
		t.Fatal("chấp nhận entry zip-slip")
	}
}

func TestValidateRejectsAbsoluteAndUnknownRestoreTargets(t *testing.T) {
	for _, name := range []string{
		"/tmp/escape",
		"C:/Windows/escape",
		`C:\Windows\escape`,
		`\\server\share\escape`,
		"tokens/../../escape",
		"tokens/NUL.json",
		"tokens/COM1",
		"tokens/invalid.",
		"settings.json",
	} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			writeValidManifest(t, zw)
			ledger, err := zw.Create("ledger.db")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.Write(append([]byte("SQLite format 3\x00"), []byte("x")...)); err != nil {
				t.Fatal(err)
			}
			entry, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte("untrusted")); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := Validate(openZip(t, buf.Bytes())); err == nil {
				t.Fatalf("Validate accepted restore target %q", name)
			}
		})
	}
}

func TestValidateRejectsDuplicateRestoreTargets(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeValidManifest(t, zw)
	for range 2 {
		entry, err := zw.Create("ledger.db")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(append([]byte("SQLite format 3\x00"), []byte("x")...)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Validate(openZip(t, buf.Bytes())); err == nil {
		t.Fatal("Validate accepted duplicate database paths")
	}
}

func TestValidateRejectsSQLiteSidecarWithoutDatabase(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeValidManifest(t, zw)
	ledger, err := zw.Create("ledger.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Write(append([]byte("SQLite format 3\x00"), []byte("x")...)); err != nil {
		t.Fatal(err)
	}
	wal, err := zw.Create("studio.db-wal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wal.Write([]byte("orphan wal")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Validate(openZip(t, buf.Bytes())); err == nil {
		t.Fatal("Validate accepted SQLite sidecar without its database")
	}
}

func TestValidateRequiresAicosManifest(t *testing.T) {
	for _, manifest := range []string{
		`{}`,
		`{"app":"another-app","created":"2026-10-28T00:00:00Z"}`,
		`{"app":"aicos","created":"not-a-time"}`,
	} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		mf, err := zw.Create(ManifestName)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = mf.Write([]byte(manifest))
		ledger, err := zw.Create("ledger.db")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = ledger.Write(append([]byte("SQLite format 3\x00"), []byte("x")...))
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := Validate(openZip(t, buf.Bytes())); err == nil {
			t.Fatalf("Validate accepted invalid manifest %s", manifest)
		}
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

func TestStagePreservesExistingPendingRestore(t *testing.T) {
	source := t.TempDir()
	seedData(t, source)
	var archive bytes.Buffer
	if err := Create(source, &archive); err != nil {
		t.Fatalf("Create: %v", err)
	}

	target := t.TempDir()
	stage := filepath.Join(target, stagingDir)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	want := []byte("original staged restore")
	if err := os.WriteFile(filepath.Join(stage, "ledger.db"), want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, RestorePending), []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Stage(target, openZip(t, archive.Bytes())); err == nil {
		t.Fatal("Stage replaced an unapplied restore")
	}
	got, err := os.ReadFile(filepath.Join(stage, "ledger.db"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("existing staged restore changed: got=%q err=%v", got, err)
	}
	if !PendingRestore(target) {
		t.Fatal("existing pending restore flag was cleared")
	}
}

func TestConcurrentStageAcceptsOnlyOneRestore(t *testing.T) {
	archives := make([][]byte, 2)
	markers := make([][]byte, 2)
	for i := range archives {
		source := t.TempDir()
		seedData(t, source)
		markers[i] = make([]byte, 2<<20)
		if _, err := rand.Read(markers[i]); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "content_jobs.json"), markers[i], 0o600); err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		if err := Create(source, &archive); err != nil {
			t.Fatalf("Create archive %d: %v", i, err)
		}
		archives[i] = archive.Bytes()
	}

	target := t.TempDir()
	start := make(chan struct{})
	readers := []*zip.Reader{openZip(t, archives[0]), openZip(t, archives[1])}
	type result struct {
		index int
		err   error
	}
	results := make(chan result, len(archives))
	var wg sync.WaitGroup
	for i := range archives {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results <- result{index: i, err: Stage(target, readers[i])}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	var winner int
	for result := range results {
		if result.err == nil {
			winners++
			winner = result.index
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent Stage accepted %d archives, want exactly one", winners)
	}
	if !PendingRestore(target) {
		t.Fatal("successful Stage did not write pending restore flag")
	}
	staged, err := os.ReadFile(filepath.Join(target, stagingDir, "content_jobs.json"))
	if err != nil || !bytes.Equal(staged, markers[winner]) {
		t.Fatalf("staged archive content does not match accepted restore: err=%v", err)
	}
}

func TestApplyStagedRecoversAfterStagingWasConsumed(t *testing.T) {
	for _, stageMissing := range []bool{false, true} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, RestorePending), []byte("pending"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !stageMissing {
			if err := os.Mkdir(filepath.Join(dir, stagingDir), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := ApplyStaged(dir); err != nil {
			t.Fatalf("ApplyStaged(stageMissing=%v): %v", stageMissing, err)
		}
		if PendingRestore(dir) {
			t.Fatalf("pending flag remains (stageMissing=%v)", stageMissing)
		}
	}
}

func TestApplyStagedPreservesPendingRestoreWhenOldWALCannotBeRemoved(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, stagingDir)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "ledger.db"), []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "ledger.db-wal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.db-wal", "busy"), []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, RestorePending), []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ApplyStaged(dir); err == nil {
		t.Fatal("ApplyStaged succeeded despite failing to remove old WAL")
	}
	if !PendingRestore(dir) {
		t.Fatal("failed restore cleared pending flag")
	}
	if _, err := os.Stat(filepath.Join(stage, "ledger.db")); err != nil {
		t.Fatal("failed restore consumed staged ledger.db")
	}
}

// TestCreateIncludesAllThreeDBs: ảnh chụp phải gồm đủ 3 DB đang mở
// (ledger.db, studio.db, products.db).
func TestCreateIncludesAllThreeDBs(t *testing.T) {
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
	for _, db := range []string{"ledger.db", "studio.db", "products.db"} {
		if !names[db] {
			t.Errorf("thiếu %s trong ảnh chụp", db)
		}
	}
}

// Online snapshot must include committed rows that still live in the WAL.
func TestCreateSnapshotsLiveWALDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.db")
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
	var walMode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&walMode); err != nil || strings.ToLower(walMode) != "wal" {
		t.Fatalf("bật WAL: mode=%q err=%v", walMode, err)
	}
	if _, err := db.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t (v) VALUES ('still-in-wal')"); err != nil {
		t.Fatal(err)
	}
	walInfo, err := os.Stat(path + "-wal")
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("test needs committed WAL pages: info=%v err=%v", walInfo, err)
	}

	var archive bytes.Buffer
	if err := Create(dir, &archive); err != nil {
		t.Fatalf("Create with active WAL: %v", err)
	}
	zr := openZip(t, archive.Bytes())
	var ledger *zip.File
	for _, f := range zr.File {
		if f.Name == "ledger.db" {
			ledger = f
			break
		}
	}
	if ledger == nil {
		t.Fatal("snapshot is missing ledger.db")
	}
	rc, err := ledger.Open()
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	out, err := os.Create(snapshotPath)
	if err != nil {
		rc.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	rc.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("extract snapshot: copy=%v close=%v", copyErr, closeErr)
	}
	snapshot, err := sql.Open("sqlite", snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var n int
	if err := snapshot.QueryRow("SELECT COUNT(*) FROM t").Scan(&n); err != nil || n != 2 {
		t.Fatalf("snapshot omitted committed WAL row: count=%d err=%v", n, err)
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

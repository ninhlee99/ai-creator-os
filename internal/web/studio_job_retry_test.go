package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	_ "modernc.org/sqlite"
)

func TestStudioJobRetryViralReEnqueuesViralJob(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "studio.db")
	st, err := studio.New(dbPath, nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st

	// Insert a mock viral job that failed
	params, _ := json.Marshal(studio.ViralParams{
		Topic:          "Test Viral Retry",
		SourceVideoURL: "https://www.tiktok.com/@test/video/1234567890",
		SourceVideoID:  "1234567890",
		TargetDuration: 10,
	})

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO studio_jobs(id,kind,title,status,params,created_at,log) VALUES(?,?,?,?,?,?,?)`,
		"viral_job_001", studio.KindViral, "Test Viral Retry", studio.StatusFailed, string(params), time.Now().Format("2006-01-02T15:04:05"), "20:00:00 LỖI: mock failure")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/studio/jobs/viral_job_001/retry", strings.NewReader("confirm_flow_credits=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "viral_job_001")
	w := httptest.NewRecorder()

	s.handleStudioJobRetry(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 See Other", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if strings.HasPrefix(loc, "/studio?") {
		t.Fatalf("Location = %q redirected to affiliate studio create instead of /studio/jobs", loc)
	}
	if !strings.HasPrefix(loc, "/studio/jobs") {
		t.Fatalf("Location = %q, want prefix /studio/jobs", loc)
	}
}

package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	_ "modernc.org/sqlite"
)

func TestHandleModelPhoto(t *testing.T) {
	s := newTestServer(t)

	// Setup studio and model directory
	workDir := t.TempDir()
	sDBPath := filepath.Join(workDir, "studio.db")
	sdb, err := sql.Open("sqlite", sDBPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sdb.Close()

	st, err := studio.New(sDBPath, nil, nil, nil, workDir)
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	defer st.Close()
	s.Studio = st

	// Create test model photo file
	modelDir := filepath.Join(st.WorkDir(), "models", "42")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir modelDir: %v", err)
	}
	samplePhoto := filepath.Join(modelDir, "face_front.jpg")
	if err := os.WriteFile(samplePhoto, []byte("fake-jpeg-content"), 0o644); err != nil {
		t.Fatalf("write samplePhoto: %v", err)
	}

	// 1. Success case: GET /models/42/face_front.jpg
	req := httptest.NewRequest(http.MethodGet, "/models/42/face_front.jpg", nil)
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 OK", rec.Code)
	}
	if rec.Body.String() != "fake-jpeg-content" {
		t.Fatalf("body = %q, want 'fake-jpeg-content'", rec.Body.String())
	}

	// 2. Disallowed extension: /models/42/secret.txt
	reqBad := httptest.NewRequest(http.MethodGet, "/models/42/secret.txt", nil)
	recBad := httptest.NewRecorder()
	serveLocalTestRoute(s, recBad, reqBad)
	if recBad.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 Not Found for disallowed extension", recBad.Code)
	}

	// 3. Non-numeric account ID
	reqNonNum := httptest.NewRequest(http.MethodGet, "/models/abc/face_front.jpg", nil)
	recNonNum := httptest.NewRecorder()
	serveLocalTestRoute(s, recNonNum, reqNonNum)
	if recNonNum.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 Not Found for non-numeric account", recNonNum.Code)
	}

	// 4. File does not exist
	reqNotFound := httptest.NewRequest(http.MethodGet, "/models/42/missing.jpg", nil)
	recNotFound := httptest.NewRecorder()
	serveLocalTestRoute(s, recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 Not Found for missing photo", recNotFound.Code)
	}
}

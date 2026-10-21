package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func setupLeonardoImport(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(s.DataDir, "ledger.db"), nil, nil, nil, s.DataDir)
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s.Studio = st
	s.OutDir = filepath.Join(s.DataDir, "output")
	return s
}

func postLeonardoUpload(t *testing.T, s *Server, filename string, contents []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("video_file", filename)
	if err != nil {
		t.Fatalf("create upload field: %v", err)
	}
	if _, err := file.Write(contents); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	_ = form.WriteField("topic", "Leonardo test clip")
	_ = form.WriteField("account", "leonardo-test-02")
	_ = form.WriteField("music_style", "keep_original")
	if err := form.Close(); err != nil {
		t.Fatalf("close multipart form: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/studio/leonardo-import", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestLeonardoImportStoresMP4AndAccountLabel(t *testing.T) {
	s := setupLeonardoImport(t)
	mp4 := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, []byte("test-video")...)
	rec := postLeonardoUpload(t, s, "scene-01.mp4", mp4)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST import = %d, want redirect; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Location"), "/studio/jobs?tab=done&notice=") {
		t.Fatalf("redirect = %q, want completed job page", rec.Header().Get("Location"))
	}

	entries, err := os.ReadDir(s.OutDir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	var outputName string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "leonardo_") && filepath.Ext(entry.Name()) == ".mp4" {
			outputName = entry.Name()
		}
	}
	if outputName == "" {
		t.Fatalf("output files = %+v, want a Leonardo MP4", entries)
	}
	got, err := os.ReadFile(filepath.Join(s.OutDir, outputName))
	if err != nil {
		t.Fatalf("read imported video: %v", err)
	}
	if !bytes.Equal(got, mp4) {
		t.Fatalf("imported bytes = %q, want original MP4 bytes", got)
	}

	var params string
	if err := s.db.QueryRow(`SELECT params FROM studio_jobs WHERE output = ?`, outputName).Scan(&params); err != nil {
		t.Fatalf("read stored job metadata: %v", err)
	}
	if !strings.Contains(params, `"source":"leonardo_ai"`) || !strings.Contains(params, `"account":"leonardo-test-02"`) {
		t.Fatalf("job params = %s, want Leonardo source and account label", params)
	}
}

func TestLeonardoImportRejectsNonMP4(t *testing.T) {
	s := setupLeonardoImport(t)
	rec := postLeonardoUpload(t, s, "not-a-video.png", []byte("not mp4"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST import = %d, want redirect", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "Ch%E1%BB%89+nh%E1%BA%ADn+t%E1%BB%87p+MP4") {
		t.Fatalf("redirect = %q, want invalid file message", rec.Header().Get("Location"))
	}
}

package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestVideoCreateRejectsUnreadyEditorBeforeParsingUpload(t *testing.T) {
	dir := t.TempDir()
	st, err := studio.New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := &Server{Studio: st}
	req := httptest.NewRequest(http.MethodPost, "/workflow/create", strings.NewReader("not a multipart body"))
	rec := httptest.NewRecorder()
	s.handleVideoCreateSubmit(rec, req)

	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("status=%d location=%q, want early unready editor redirect", rec.Code, rec.Header().Get("Location"))
	}
}

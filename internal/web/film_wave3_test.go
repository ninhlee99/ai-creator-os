package web

// Film Wave 3 — test web: endpoint "Kiểm tra quay video" và bảng khả năng AI
// trên trang Model local.

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type wave3StubLLM struct{}

func (wave3StubLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	return "{}", nil
}
func (wave3StubLLM) Name() string                     { return "stub" }
func (wave3StubLLM) Healthy(ctx context.Context) bool { return true }

type wave3StubMG struct{}

func (wave3StubMG) GenerateImage(ctx context.Context, prompt string, refs []studio.ImageRef, outPath string) error {
	return os.WriteFile(outPath, []byte("png"), 0644)
}
func (wave3StubMG) GenerateVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string) error {
	return os.WriteFile(outPath, []byte("mp4"), 0644)
}
func (wave3StubMG) Name() string                     { return "stub" }
func (wave3StubMG) Healthy(ctx context.Context) bool { return true }

func newWave3Studio(t *testing.T) *studio.Studio {
	t.Helper()
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"),
		wave3StubLLM{}, wave3StubMG{}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestProbeVideoEndpoint(t *testing.T) {
	s := newTestServer(t)
	// Chưa gắn Studio → 404.
	rec := postForm(t, s, "/settings/model-local/probe-video", url.Values{})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("thiếu Studio → 404, got %d", rec.Code)
	}
	// Gắn Studio → 303 về trang Model local (probe chạy nền).
	s.Studio = newWave3Studio(t)
	rec = postForm(t, s, "/settings/model-local/probe-video", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST probe → 303, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/settings/model-local") || !strings.Contains(loc, "ok=") {
		t.Fatalf("redirect về model-local kèm ?ok=, got %q", loc)
	}
}

func TestModelLocalCapabilityTable(t *testing.T) {
	s := newTestServer(t)
	s.Studio = newWave3Studio(t)
	rec := get(t, s, "/settings/model-local")
	if rec.Code != http.StatusOK {
		t.Fatalf("model-local → 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Khả năng AI", "probe-video", "Vẽ ảnh", "Quay video"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang model-local thiếu %q", want)
		}
	}
}

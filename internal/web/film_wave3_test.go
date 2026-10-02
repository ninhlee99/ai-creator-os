package web

// Film Wave 3 — test web: bảng khả năng AI trên trang Model local
// (endpoint "Kiểm tra quay video" đã park cùng pipeline phim).

import (
	"context"
	"net/http"
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

func TestModelLocalCapabilityTable(t *testing.T) {
	s := newTestServer(t)
	s.Studio = newWave3Studio(t)
	rec := get(t, s, "/settings/model-local")
	if rec.Code != http.StatusOK {
		t.Fatalf("model-local → 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Khả năng AI", "Vẽ ảnh"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang model-local thiếu %q", want)
		}
	}
	for _, gone := range []string{"probe-video", "Quay video"} {
		if strings.Contains(body, gone) {
			t.Errorf("trang model-local còn tàn dư video %q", gone)
		}
	}
}

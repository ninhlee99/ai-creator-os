package studio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
)

// Model ảnh mặc định bị Google tắt (404) → tự dò model còn sống qua
// ListModels rồi tạo ảnh thành công, không cần sửa code/cấu hình.
func TestGenerateImageRediscoversDeadModel(t *testing.T) {
	gmodels.Reset()
	t.Cleanup(gmodels.Reset)
	t.Setenv("GEMINI_IMAGE_MODEL", "")
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta/models":
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-9.0-flash-image","supportedGenerationMethods":["generateContent"]}]}`))
		case strings.Contains(r.URL.Path, "gemini-9.0-flash-image"):
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}]}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","message":"model not found"}}`))
		}
	}))
	defer srv.Close()
	g := NewGeminiMediaGen([]string{"k1"})
	g.SetBaseURL(srv.URL)
	out := filepath.Join(t.TempDir(), "a.png")
	if err := g.GenerateImage(context.Background(), "a cat", nil, out); err != nil {
		t.Fatalf("generate: %v (hits %v)", err, hits)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("chưa ghi ảnh")
	}
	if gmodels.Resolve(gmodels.Image) != "gemini-9.0-flash-image" {
		t.Fatalf("phải nhớ model mới, got %s", gmodels.Resolve(gmodels.Image))
	}
}

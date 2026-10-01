package local

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegistryDownloadOK(t *testing.T) {
	content := "model-bytes-123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "15")
		w.Write([]byte(content))
	}))
	defer srv.Close()

	reg := NewRegistry(t.TempDir())
	var progress []int64
	err := reg.Download(context.Background(), "m.gguf", srv.URL+"/m.gguf", 5, func(d, total int64) {
		progress = append(progress, d)
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(reg.ModelPath("m.gguf"))
	if err != nil || string(got) != content {
		t.Fatalf("file = %q err=%v", got, err)
	}
	if len(progress) == 0 {
		t.Fatal("onProgress never called")
	}
	if !reg.Has("m.gguf") {
		t.Fatal("Has should be true")
	}
	// second call: already downloaded -> no-op, no network needed
	if err := reg.Download(context.Background(), "m.gguf", "http://127.0.0.1:1/nope", 5, nil); err != nil {
		t.Fatalf("second Download: %v", err)
	}
}

func TestRegistryDownloadCancel(t *testing.T) {
	// Slow server: 1 byte per 50ms.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		fl := w.(http.Flusher)
		for i := 0; i < 100000; i++ {
			if _, err := w.Write([]byte("x")); err != nil {
				return
			}
			fl.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()

	reg := NewRegistry(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- reg.Download(ctx, "big.gguf", srv.URL+"/big.gguf", 0, nil)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancel error, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Download did not abort on cancel")
	}
	// partial file is kept for resume
	if _, err := os.Stat(reg.ModelPath("big.gguf.part")); err != nil {
		t.Fatalf("partial file missing: %v", err)
	}
}

func TestRegistryDownloadMinSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tiny"))
	}))
	defer srv.Close()
	reg := NewRegistry(t.TempDir())
	err := reg.Download(context.Background(), "m.gguf", srv.URL+"/m.gguf", 1000, nil)
	if err == nil || !strings.Contains(err.Error(), "need at least") {
		t.Fatalf("err = %v, want min-size error", err)
	}
}

func TestEnsureVieNeuRepoPresent(t *testing.T) {
	dataDir := t.TempDir()
	p := filepath.Join(dataDir, "third_party", "vieneu-tts", "apps")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "openai_speech.py"), []byte("#"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(dataDir)
	if !reg.VieNeuPresent() {
		t.Fatal("VieNeuPresent should be true")
	}
	// No network needed when already present.
	if err := reg.EnsureVieNeuRepo(context.Background(), nil); err != nil {
		t.Fatalf("EnsureVieNeuRepo: %v", err)
	}
}

func TestDefaultModels(t *testing.T) {
	if len(DefaultModels) == 0 {
		t.Fatal("empty catalog")
	}
	if !strings.HasSuffix(DefaultModels[0].Name, ".gguf") {
		t.Fatalf("model = %+v", DefaultModels[0])
	}
	if !strings.HasPrefix(DefaultModels[0].URL, "https://huggingface.co/") {
		t.Fatalf("url = %q", DefaultModels[0].URL)
	}
}

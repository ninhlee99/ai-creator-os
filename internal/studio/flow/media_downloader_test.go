package flow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadMediaResourceStopsHTMLCycles(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/a" {
			_, _ = w.Write([]byte(`<meta property="og:image" content="/b">`))
			return
		}
		_, _ = w.Write([]byte(`<meta property="og:image" content="/a">`))
	}))
	defer server.Close()

	_, _, err := DownloadMediaResource(context.Background(), server.URL+"/a", t.TempDir(), "image")
	if err == nil || !strings.Contains(err.Error(), "vòng lặp") {
		t.Fatalf("cycle error = %v, want media URL cycle error", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests = %d, want 2 before cycle detection", got)
	}
}

func TestDownloadMediaResourceRemovesPartialFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, _, err := DownloadMediaResource(context.Background(), server.URL+"/image.jpg", destDir, "image")
	if err == nil {
		t.Fatal("truncated media response unexpectedly succeeded")
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial download left files in %s: %v", filepath.Base(destDir), entries)
	}
}

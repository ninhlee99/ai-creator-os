package flow

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCopyMediaWithLimitRejectsUnknownLengthOverflow(t *testing.T) {
	var dst bytes.Buffer
	written, err := copyMediaWithLimit(&dst, strings.NewReader("four"), 3)
	if !errors.Is(err, errMediaDownloadTooLarge) || written != 4 {
		t.Fatalf("copyMediaWithLimit() = (%d, %v), want (4, media too large)", written, err)
	}
	if dst.String() != "four" {
		t.Fatalf("copied bytes = %q, want one byte over the limit", dst.String())
	}
}

func TestDownloadMediaResourceRejectsDeclaredOversize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.FormatInt(maxDownloadedMediaBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, _, err := DownloadMediaResource(context.Background(), server.URL+"/large.mp4", destDir, "video")
	if !errors.Is(err, errMediaDownloadTooLarge) {
		t.Fatalf("DownloadMediaResource() error = %v, want media size limit", err)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized download left files in destination: %v", entries)
	}
}

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

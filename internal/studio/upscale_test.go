package studio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lavfiPNG(t *testing.T, dir, name, src string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", src, "-frames:v", "1", out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return out
}

func probeHeight(t *testing.T, path string) int {
	t.Helper()
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe height: %v", err)
	}
	var h int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &h); err != nil {
		t.Fatalf("parse height: %v", err)
	}
	return h
}

func TestUpscalePhoto4K(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	src := lavfiPNG(t, dir, "small.png", "testsrc=s=270x480:d=1")
	dst := filepath.Join(dir, "big.png")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := UpscalePhoto(ctx, src, dst, PhotoWidth4K); err != nil {
		t.Fatalf("upscale: %v", err)
	}
	if w := ProbeWidth(ctx, dst); w != 2160 {
		t.Fatalf("width = %d, want 2160", w)
	}
	if h := probeHeight(t, dst); h != 3840 {
		t.Fatalf("height = %d, want 3840", h)
	}
}

func TestUpscalePhotoSkipsWhenAlready4K(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	src := lavfiPNG(t, dir, "already4k.png", "testsrc=s=2160x3840:d=1")
	dst := filepath.Join(dir, "out.png")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := UpscalePhoto(ctx, src, dst, PhotoWidth4K); err != nil {
		t.Fatalf("upscale: %v", err)
	}
	a, _ := os.ReadFile(src)
	b, _ := os.ReadFile(dst)
	if string(a) != string(b) {
		t.Fatal("already-4K source should be copied untouched, not re-encoded")
	}
}

func TestUpscalePhotoBadWidth(t *testing.T) {
	ctx := context.Background()
	if err := UpscalePhoto(ctx, "a.png", "b.png", 1234); err == nil {
		t.Fatal("want error for unsupported width")
	}
}

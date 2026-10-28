//go:build unix

package studio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadTikTokSourceVideoHardLimit(t *testing.T) {
	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "yt-dlp")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	destination := filepath.Join(t.TempDir(), "source.mp4")
	installFakeYTDLP := func(contents string) {
		t.Helper()
		if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nprintf '"+contents+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	installFakeYTDLP("1234")
	if err := downloadTikTokSourceVideoWithLimit(context.Background(), "https://www.tiktok.com/@test/video/123", destination, 4); err != nil {
		t.Fatalf("download at exact limit: %v", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "1234" {
		t.Fatalf("exact-limit output = %q, %v; want 1234", got, err)
	}

	installFakeYTDLP("12345")
	err := downloadTikTokSourceVideoWithLimit(context.Background(), "https://www.tiktok.com/@test/video/123", destination, 4)
	if !errors.Is(err, errTikTokSourceTooLarge) {
		t.Fatalf("oversized download error = %v, want size-limit error", err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "1234" {
		t.Fatalf("oversized download replaced destination: %q, %v", got, err)
	}
	partials, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".tiktok-source-*.part"))
	if err != nil || len(partials) != 0 {
		t.Fatalf("temporary download files remain: %v, %v", partials, err)
	}
}

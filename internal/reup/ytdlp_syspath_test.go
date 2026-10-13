package reup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// yt-dlp cài sẵn trên máy (Homebrew) được dùng khi app chưa tự tải bản
// riêng; có bản riêng thì bản riêng thắng (Update chỉ ghi vào binDir).
func TestBinaryPathPrefersOwnThenSystem(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	sys := filepath.Join(t.TempDir(), "yt-dlp")
	lookPath = func(string) (string, error) { return sys, nil }
	m := NewManager(t.TempDir())
	if got, _ := m.BinaryPath(); got != sys {
		t.Fatalf("chưa có bản riêng → dùng bản hệ thống, got %s", got)
	}
	own, _ := assetName()
	ownPath := filepath.Join(m.binDir, own)
	if err := os.WriteFile(ownPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.BinaryPath(); got != ownPath {
		t.Fatalf("có bản riêng → dùng bản riêng, got %s", got)
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	os.Remove(ownPath)
	if got, _ := m.BinaryPath(); got != ownPath {
		t.Fatalf("không có gì → trả đường dẫn bản riêng để Update ghi vào, got %s", got)
	}
}

package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ghi vượt maxSize → file xoay: aicos.log mới + aicos.log.1 giữ nội dung cũ.
func TestRotateOnSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "aicos.log")
	r, err := New(path, 100, 3) // 100 bytes: xoay nhanh để test
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.Write([]byte("dong-dau-tien\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	big := strings.Repeat("x", 120) + "\n"
	if _, err := r.Write([]byte(big)); err != nil {
		t.Fatalf("Write big: %v", err)
	}
	r.Close()

	old, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("thiếu file xoay .1: %v", err)
	}
	if !strings.Contains(string(old), "dong-dau-tien") {
		t.Fatalf(".1 không giữ nội dung cũ: %q", old)
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("thiếu file hiện tại: %v", err)
	}
	if !strings.Contains(string(cur), "xxx") {
		t.Fatalf("file hiện tại không có nội dung mới: %q", cur)
	}
}

// Ghi nhiều lần vượt nhiều ngưỡng → không quá maxBackup bản, không mất writer.
func TestRotateKeepsMaxBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aicos.log")
	r, err := New(path, 50, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := r.Write([]byte(strings.Repeat("y", 60) + "\n")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	r.Close()
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("giữ quá 2 bản backup")
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Fatalf("thiếu .2: %v", err)
	}
}

// File đã phình từ trước (bản cũ) → mở là xoay ngay.
func TestRotateOversizedOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aicos.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("z", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := New(path, 100, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() >= 100 {
		t.Fatalf("file hiện tại vẫn phình: %d bytes", fi.Size())
	}
}

package lockfile

import (
	"path/filepath"
	"testing"
)

// Hai Acquire trên cùng file: cái thứ hai phải trả về (nil, nil) —
// tức là "đã có instance khác", không phải lỗi.
func TestAcquireExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aicos.lock")
	l1, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire lần 1: %v", err)
	}
	if l1 == nil {
		t.Fatal("Acquire lần 1 trả về nil")
	}
	l2, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire lần 2: %v", err)
	}
	if l2 != nil {
		l2.Close()
		t.Fatal("Acquire lần 2 phải trả về nil khi khóa đang bị giữ")
	}
	// Giải phóng → acquire lại được.
	if err := l1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	l3, err := Acquire(path)
	if err != nil || l3 == nil {
		t.Fatalf("Acquire sau Close: l=%v err=%v", l3, err)
	}
	l3.Close()
}

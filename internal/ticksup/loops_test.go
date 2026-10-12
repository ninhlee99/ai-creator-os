package ticksup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// ArmLoops với Deps tắt hết → không arm gì, không panic.
func TestArmLoopsAllDisabled(t *testing.T) {
	s := New(nil, nil)
	ArmLoops(context.Background(), s, Deps{})
	if len(s.Snapshot()) != 0 {
		t.Fatalf("không tick nào được arm mà vẫn có beat: %v", s.Snapshot())
	}
}

// ArmLoops với sup nil → no-op, không panic.
func TestArmLoopsNilSupervisor(t *testing.T) {
	ArmLoops(context.Background(), nil, Deps{Growth: true, Story: true})
}

// loop: fn chạy mỗi interval cho tới khi ctx hủy.
func TestLoopFiresUntilCancel(t *testing.T) {
	var n int64
	ctx, cancel := context.WithCancel(context.Background())
	loop(ctx, 30*time.Millisecond, func() { atomic.AddInt64(&n, 1) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt64(&n) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	got := atomic.LoadInt64(&n)
	if got < 2 {
		t.Fatalf("loop chỉ chạy %d lần trong 2s (interval 30ms)", got)
	}
	// Sau cancel: không chạy thêm nữa.
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt64(&n) != got {
		t.Fatal("loop vẫn chạy sau khi ctx hủy")
	}
}

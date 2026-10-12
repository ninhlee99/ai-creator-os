package ticksup

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []string
}

func (f *fakeAlerter) RaiseAlert(kind, severity, msg, action string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alerts = append(f.alerts, kind+"/"+severity+": "+msg)
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout chờ: %s", msg)
}

// Panic trong tick: người gọi (process) sống, alert critical được ghi,
// beat ghi lỗi, tick khác vẫn chạy được.
func TestDispatchRecoversPanic(t *testing.T) {
	fa := &fakeAlerter{}
	s := New(func() Alerter { return fa }, nil)
	ctx := context.Background()

	s.Dispatch(ctx, "boom", time.Minute, func(context.Context) []string {
		panic("kaboom")
	})
	// Tick khác vẫn dispatch + chạy bình thường sau panic.
	done := make(chan struct{})
	s.Dispatch(ctx, "ok", time.Minute, func(context.Context) []string {
		close(done)
		return []string{"fine"}
	})

	waitFor(t, 3*time.Second, func() bool {
		snap := s.Snapshot()
		b, ok := snap["boom"]
		return ok && b.Runs == 1 && b.LastErr != ""
	}, "beat lỗi của tick panic")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("tick bình thường không chạy được sau panic")
	}
	fa.mu.Lock()
	defer fa.mu.Unlock()
	found := false
	for _, a := range fa.alerts {
		if len(a) >= 10 && a[:10] == "tick_panic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("thiếu alert tick_panic, chỉ có: %v", fa.alerts)
	}
}

// Lượt trước chưa xong → lượt mới bị bỏ qua (không dồn goroutine).
func TestDispatchSkipsWhenInflight(t *testing.T) {
	s := New(nil, nil)
	ctx := context.Background()
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	s.Dispatch(ctx, "slow", time.Minute, func(c context.Context) []string {
		started <- struct{}{}
		<-release // treo tới khi test cho phép
		return nil
	})
	<-started
	s.Dispatch(ctx, "slow", time.Minute, func(context.Context) []string {
		started <- struct{}{}
		return nil
	})
	select {
	case <-started:
		t.Fatal("lượt 2 chạy trong khi lượt 1 còn treo — phải bị bỏ qua")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	waitFor(t, 3*time.Second, func() bool {
		return s.Snapshot()["slow"].Runs == 1
	}, "beat runs của slow")
}

// Timeout: tick treo quá hạn thì context bị hủy.
func TestDispatchTimeoutCancels(t *testing.T) {
	s := New(nil, nil)
	ctx := context.Background()
	cancelled := make(chan struct{})
	s.Dispatch(ctx, "hang", 100*time.Millisecond, func(c context.Context) []string {
		<-c.Done()
		close(cancelled)
		return nil
	})
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("context không bị hủy khi quá timeout")
	}
}

// Snapshot phản ánh đúng số lượt + lần OK cuối.
func TestSnapshotBeats(t *testing.T) {
	s := New(nil, nil)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		done := make(chan struct{})
		s.Dispatch(ctx, "t", time.Minute, func(context.Context) []string {
			close(done)
			return nil
		})
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("tick không chạy")
		}
	}
	waitFor(t, 3*time.Second, func() bool {
		b := s.Snapshot()["t"]
		return b.Runs == 2 && !b.LastOK.IsZero() && b.LastErr == ""
	}, "beat sau 2 lượt OK")
}

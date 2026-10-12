package ticksup

import (
	"testing"
	"time"
)

func TestWatchStale(t *testing.T) {
	sup := New(nil, nil)
	// Tick chưa từng chạy nhưng mới đăng ký → chưa stale.
	sup.Watch("fresh", time.Minute)
	if got := sup.Stale(time.Now()); len(got) != 0 {
		t.Fatalf("mới đăng ký không được stale: %v", got)
	}
	// Giả lập đăng ký từ lâu, chưa từng chạy → stale.
	sup.mu.Lock()
	sup.watches["old"] = watchSpec{every: time.Minute, registeredAt: time.Now().Add(-time.Hour)}
	sup.mu.Unlock()
	if got := sup.Stale(time.Now()); len(got) != 1 || got[0] != "old" {
		t.Fatalf("tick cũ chưa chạy phải stale: %v", got)
	}
	// Tick vừa dispatch → không stale.
	sup.Watch("recent", time.Minute)
	sup.beatStart("recent")
	if got := sup.Stale(time.Now()); len(got) != 1 || got[0] != "old" {
		t.Fatalf("tick vừa chạy không được stale: %v", got)
	}
	// Tick dispatch từ lâu → stale.
	sup.mu.Lock()
	sup.beats["hung"] = &Beat{LastStart: time.Now().Add(-time.Hour), Runs: 1}
	sup.watches["hung"] = watchSpec{every: time.Minute, registeredAt: time.Now().Add(-2 * time.Hour)}
	sup.mu.Unlock()
	got := sup.Stale(time.Now())
	if len(got) != 2 {
		t.Fatalf("muốn [hung old], được %v", got)
	}
}

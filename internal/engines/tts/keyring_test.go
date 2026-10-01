package tts

import (
	"testing"
	"time"
)

// The observability counters (Requests, LastRateLimitUnix) must not change
// rotation behavior: they only record what happened.
func TestKeyRingObservability(t *testing.T) {
	r := NewKeyRing([]string{"k1", "k2"})
	tried := map[int]bool{}
	k, idx, ok := r.Next(tried)
	if !ok || k != "k1" || idx != 0 {
		t.Fatalf("Next = %q,%d,%v", k, idx, ok)
	}
	tried[idx] = true
	if _, _, ok := r.Next(tried); !ok {
		t.Fatal("second Next should succeed")
	}
	r.ReportRateLimit(1)

	st := r.Status()
	if len(st) != 2 {
		t.Fatalf("Status len = %d", len(st))
	}
	if st[0].Requests != 1 || st[1].Requests != 1 {
		t.Fatalf("Requests = %d,%d, want 1,1", st[0].Requests, st[1].Requests)
	}
	if st[1].RateLimitHits != 1 {
		t.Fatalf("RateLimitHits = %d, want 1", st[1].RateLimitHits)
	}
	if st[1].LastRateLimitUnix <= 0 {
		t.Fatal("LastRateLimitUnix should be set after ReportRateLimit")
	}
	if st[0].LastRateLimitUnix != 0 {
		t.Fatal("LastRateLimitUnix should be 0 when never rate-limited")
	}
	if st[1].State != "cooldown" {
		t.Fatalf("State = %q, want cooldown", st[1].State)
	}
	// JSON shape used by the dashboard must carry the new fields.
	if time.Unix(st[1].LastRateLimitUnix, 0).After(time.Now()) {
		t.Fatal("LastRateLimitUnix is in the future")
	}
}

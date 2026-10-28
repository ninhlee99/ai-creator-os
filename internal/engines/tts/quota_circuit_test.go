package tts

import (
	"testing"
	"time"
)

// Đủ 3 lần cạn kiệt liên tiếp → mạch mở tới nửa đêm; alert đúng 1 lần.
func TestCircuitTripsAfterThreshold(t *testing.T) {
	c := &Circuit{minInterval: time.Nanosecond}
	if c.Open() {
		t.Fatal("mạch mới phải đóng")
	}
	if observe(c) {
		t.Fatal("lần 1 chưa được trip")
	}
	if observe(c) {
		t.Fatal("lần 2 chưa được trip")
	}
	if !observe(c) {
		t.Fatal("lần 3 phải trip")
	}
	if !c.Open() {
		t.Fatal("mạch phải mở sau khi trip")
	}
	until := c.OpenUntil()
	if until.IsZero() {
		t.Fatal("thiếu thời điểm tự đóng")
	}
	// Quota Gemini reset at midnight Pacific, independent of host timezone.
	want := nextPacificMidnight(time.Now())
	if !until.Equal(want) {
		t.Fatalf("mở tới %v, muốn %v", until, want)
	}
	if !c.TakeAlert() {
		t.Fatal("TakeAlert lần 1 phải true")
	}
	if c.TakeAlert() {
		t.Fatal("TakeAlert lần 2 phải false (không spam)")
	}
}

// Một lần thành công → đóng mạch, reset bộ đếm.
func TestCircuitClosesOnSuccess(t *testing.T) {
	c := &Circuit{minInterval: time.Nanosecond}
	observe(c)
	observe(c)
	observe(c)
	if !c.Open() {
		t.Fatal("setup: mạch phải mở")
	}
	c.ObserveSuccess()
	if c.Open() {
		t.Fatal("thành công phải đóng mạch")
	}
	// Bộ đếm reset: cần lại đủ 3 lần mới trip.
	observe(c)
	observe(c)
	if c.Open() {
		t.Fatal("mới 2 lần sau reset, mạch phải còn đóng")
	}
}

// Xen kẽ thành công thì không bao giờ trip (không phải cạn quota ngày).
func TestCircuitNoTripOnInterleavedSuccess(t *testing.T) {
	c := &Circuit{minInterval: time.Nanosecond}
	for i := 0; i < 10; i++ {
		observe(c)
		c.ObserveSuccess()
	}
	if c.Open() {
		t.Fatal("xen kẽ thành công thì mạch phải đóng")
	}
}

// observe gọi ObserveExhausted sau khi xoá mốc lần trước — tránh flaky
// trên máy có đồng hồ thô (M1: 2 lời gọi liền nhau có thể cách 0ns nên
// bị dedup minInterval nuốt mất).
func observe(c *Circuit) bool {
	c.mu.Lock()
	c.lastObserved = time.Time{}
	c.mu.Unlock()
	return c.ObserveExhausted()
}

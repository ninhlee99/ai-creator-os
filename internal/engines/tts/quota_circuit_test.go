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
	if c.ObserveExhausted() {
		t.Fatal("lần 1 chưa được trip")
	}
	if c.ObserveExhausted() {
		t.Fatal("lần 2 chưa được trip")
	}
	if !c.ObserveExhausted() {
		t.Fatal("lần 3 phải trip")
	}
	if !c.Open() {
		t.Fatal("mạch phải mở sau khi trip")
	}
	until := c.OpenUntil()
	if until.IsZero() {
		t.Fatal("thiếu thời điểm tự đóng")
	}
	// Mở tới 0h đêm nay (giờ địa phương), không phải "vài phút nữa".
	now := time.Now()
	y, m, d := now.Date()
	want := time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
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
	c.ObserveExhausted()
	c.ObserveExhausted()
	c.ObserveExhausted()
	if !c.Open() {
		t.Fatal("setup: mạch phải mở")
	}
	c.ObserveSuccess()
	if c.Open() {
		t.Fatal("thành công phải đóng mạch")
	}
	// Bộ đếm reset: cần lại đủ 3 lần mới trip.
	c.ObserveExhausted()
	c.ObserveExhausted()
	if c.Open() {
		t.Fatal("mới 2 lần sau reset, mạch phải còn đóng")
	}
}

// Xen kẽ thành công thì không bao giờ trip (không phải cạn quota ngày).
func TestCircuitNoTripOnInterleavedSuccess(t *testing.T) {
	c := &Circuit{minInterval: time.Nanosecond}
	for i := 0; i < 10; i++ {
		c.ObserveExhausted()
		c.ObserveSuccess()
	}
	if c.Open() {
		t.Fatal("xen kẽ thành công thì mạch phải đóng")
	}
}

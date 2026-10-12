// Ngắt mạch quota ngày cho các Gemini tier (Đợt S).
//
// Vấn đề: KeyRing backoff 429 tối đa 15 phút và chỉ nằm trong bộ nhớ.
// Khi mọi key đã hết quota ngày, mỗi ~15 phút một key "hồi" lại, tick tiếp
// theo đốt 1 API call thật để rồi nhận 429 — vô vọng cả ngày, và các job
// mới (vd: StoryRetryTick) vẫn được châm vào quota đã cạn.
//
// Circuit: đếm số lần liên tiếp "mọi key đang cooldown" (không có API call
// nào thành công ở giữa). Đủ ngưỡng → mở mạch tới 0h đêm hôm sau (giờ
// địa phương — gần đúng với chu kỳ reset quota ngày): các Gemini tier
// fail-fast NGAY, không chạm keyring, chain chuyển sang tier local như
// bình thường. Một key thành công (kể cả nút "test" ở Cài đặt) → đóng mạch.
//
// Đặt ở package tts để cả engines (LLM) lẫn studio (mediagen) dùng chung
// mà không tạo import vòng.
//
// Fail-closed theo hướng an toàn: mở mạch chỉ làm Gemini tier nghỉ, tier
// local vẫn chạy — không mất tính năng, chỉ bớt đốt quota vô ích.
package tts

import (
	"sync"
	"time"
)

// circuitTripThreshold: số lần "mọi key cooldown" liên tiếp thì mở mạch.
const circuitTripThreshold = 3

// Circuit là ngắt mạch quota dùng chung cho mọi Gemini provider
// (LLM + TTS + image). Một instance duy nhất cho cả app.
type Circuit struct {
	mu          sync.Mutex
	consecutive int
	openUntil   time.Time
	alerted     bool // đã báo alert cho lần mở mạch hiện tại chưa
}

var geminiCircuit = &Circuit{}

// GeminiQuotaCircuit trả về ngắt mạch quota dùng chung.
func GeminiQuotaCircuit() *Circuit { return geminiCircuit }

// ObserveExhausted ghi nhận một lần "mọi key đang cooldown". Trả về true
// đúng một lần khi mạch vừa mở (để gọi alert).
func (c *Circuit) ObserveExhausted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isOpenLocked() {
		return false
	}
	c.consecutive++
	if c.consecutive >= circuitTripThreshold {
		c.openUntil = nextMidnight(time.Now())
		c.alerted = false
		c.consecutive = 0
		return true
	}
	return false
}

// ObserveSuccess ghi nhận một API call thành công: đóng mạch đang mở
// (quota đã hồi) và reset bộ đếm.
func (c *Circuit) ObserveSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consecutive = 0
	c.openUntil = time.Time{}
	c.alerted = false
}

// Open báo mạch có đang mở không (hết hạn → tự đóng).
func (c *Circuit) Open() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isOpenLocked()
}

// OpenUntil trả về thời điểm mạch tự đóng (zero khi đang đóng).
func (c *Circuit) OpenUntil() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() {
		return time.Time{}
	}
	return c.openUntil
}

// TakeAlert trả về true đúng một lần cho mỗi lần mạch mở — dùng để gửi
// đúng 1 alert cho Ninh, không spam.
func (c *Circuit) TakeAlert() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isOpenLocked() || c.alerted {
		return false
	}
	c.alerted = true
	return true
}

func (c *Circuit) isOpenLocked() bool {
	return !c.openUntil.IsZero() && time.Now().Before(c.openUntil)
}

// nextMidnight: 0h đêm nay theo giờ địa phương.
func nextMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

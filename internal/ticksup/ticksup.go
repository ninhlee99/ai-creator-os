// Package ticksup giám sát các automation tick chạy nền (Đợt R).
//
// Trước đây: 6 tick chạy NỐI TIẾP trong một goroutine, không recover, không
// timeout. Một panic ở tick bất kỳ giết chết toàn bộ process (cả dashboard
// HTTP); một tick treo (vd: upload YouTube kẹt) làm self-heal, backup và
// Telegram alert đứng im theo mà không ai biết — đúng lúc cần cảnh báo nhất.
//
// Bây giờ: mỗi tick chạy trong goroutine riêng qua Dispatch — panic được
// recover + ghi alert critical, treo quá timeout thì context bị hủy (tick
// fail, chạy lại vòng sau), lượt trước chưa xong thì bỏ qua lượt mới
// (không dồn). Heartbeat từng tick phục vụ /healthz và Cài đặt · Hệ thống.
package ticksup

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// Alerter nhận alert khi supervisor phát hiện sự cố (panic tick, ngắt mạch
// quota mở). *automation.Service thỏa mãn interface này.
type Alerter interface {
	RaiseAlert(kind, severity, msg, action string)
}

// Beat là heartbeat của một tick.
type Beat struct {
	LastStart time.Time
	LastOK    time.Time
	LastErr   string
	Runs      int64
}

// QuotaAlert là hàm báo "ngắt mạch quota vừa mở" — trả true đúng một lần
// mỗi lần mở (vd: tts.GeminiQuotaCircuit().TakeAlert). Nil = không dùng.
type QuotaAlert func() bool

// Supervisor giữ heartbeat và trạng thái đang-chạy của các tick.
type Supervisor struct {
	mu         sync.Mutex
	beats      map[string]*Beat
	inflight   map[string]bool
	alerterFn  func() Alerter
	quotaAlert QuotaAlert
	// watches: nhịp kỳ vọng của từng tick (Đợt V — watchdog).
	watches map[string]watchSpec
}

// watchSpec ghi nhịp kỳ vọng + thời điểm đăng ký của một tick.
type watchSpec struct {
	every        time.Duration
	registeredAt time.Time
}

// New tạo supervisor. alerterFn build Alerter lúc cần báo (lazy — để
// Service luôn thấy đủ các store đã wiring xong); nil = chỉ log.
func New(alerterFn func() Alerter, quotaAlert QuotaAlert) *Supervisor {
	return &Supervisor{
		beats:      make(map[string]*Beat),
		inflight:   make(map[string]bool),
		alerterFn:  alerterFn,
		quotaAlert: quotaAlert,
		watches:    make(map[string]watchSpec),
	}
}

// raiseAlert gửi alert qua alerterFn (nếu có).
func (s *Supervisor) raiseAlert(kind, severity, msg, action string) {
	if s.alerterFn == nil {
		return
	}
	if a := s.alerterFn(); a != nil {
		a.RaiseAlert(kind, severity, msg, action)
	}
}

// Dispatch chạy tick trong goroutine riêng có giám sát. Không chặn người gọi.
// Nếu lượt trước của cùng tick vẫn đang chạy → bỏ qua lượt này, ghi log.
func (s *Supervisor) Dispatch(ctx context.Context, name string, timeout time.Duration, tick func(context.Context) []string) {
	s.mu.Lock()
	if s.inflight[name] {
		s.mu.Unlock()
		log.Printf("tick %s: lượt trước vẫn đang chạy — bỏ qua lượt này", name)
		return
	}
	s.inflight[name] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.inflight, name)
			s.mu.Unlock()
		}()
		s.beatStart(name)
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("Tick %s gặp panic (%v) — đã tự phục hồi, tick chạy lại ở vòng sau", name, r)
				log.Printf("tick %s: PANIC recovered: %v", name, r)
				s.beatErr(name, fmt.Sprintf("panic: %v", r))
				s.raiseAlert("tick_panic", "critical", msg,
					"Kiểm tra log aicos.log để điều tra nguyên nhân.")
			}
		}()
		tctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		for _, note := range tick(tctx) {
			log.Printf("%s: %s", name, note)
		}
		s.beatOK(name)
		// Đợt S: ngắt mạch quota vừa mở trong tick này → báo đúng 1 lần.
		if s.quotaAlert != nil && s.quotaAlert() {
			log.Printf("tick %s: ngắt mạch quota Gemini mở (hết quota ngày)", name)
			s.raiseAlert("quota_circuit", "warn",
				"Hết quota Gemini ngày — các tier Gemini nghỉ tới 0h đêm, tier local vẫn chạy.",
				"Không cần làm gì; mai quota tự hồi. Muốn chạy ngay: thêm key ở Cài đặt · Nhà cung cấp.")
		}
	}()
}

// Snapshot trả bản chụp heartbeat của mọi tick (cho /healthz và UI).
func (s *Supervisor) Snapshot() map[string]Beat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Beat, len(s.beats))
	for k, v := range s.beats {
		out[k] = *v
	}
	return out
}

func (s *Supervisor) beatStart(name string) {
	s.mu.Lock()
	b, ok := s.beats[name]
	if !ok {
		b = &Beat{}
		s.beats[name] = b
	}
	b.LastStart = time.Now()
	b.Runs++
	s.mu.Unlock()
}

func (s *Supervisor) beatOK(name string) {
	s.mu.Lock()
	if b, ok := s.beats[name]; ok {
		b.LastOK = time.Now()
		b.LastErr = ""
	}
	s.mu.Unlock()
}

func (s *Supervisor) beatErr(name, msg string) {
	s.mu.Lock()
	if b, ok := s.beats[name]; ok {
		b.LastErr = msg
	}
	s.mu.Unlock()
}

// Watch đăng ký nhịp kỳ vọng cho một tick (Đợt V — watchdog trong app).
// every là khoảng cách tối đa giữa hai lần Dispatch (bằng nhịp của loop).
func (s *Supervisor) Watch(name string, every time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.watches[name]; !ok {
		s.watches[name] = watchSpec{every: every, registeredAt: time.Now()}
	}
}

// Stale trả về tên các tick đã quá 3×nhịp kỳ vọng mà chưa Dispatch —
// dấu hiệu tick deadlock hoặc loop chết mà /healthz không phát hiện được
// (beats cũ vẫn hiển thị bình thường).
func (s *Supervisor) Stale(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for name, w := range s.watches {
		limit := 3 * w.every
		if b, ok := s.beats[name]; ok && !b.LastStart.IsZero() {
			if now.Sub(b.LastStart) > limit {
				out = append(out, name)
			}
			continue
		}
		// Chưa từng chạy: chỉ tính stale khi đã qua 3×nhịp từ lúc đăng ký
		// (loại trừ giai đoạn khởi động).
		if now.Sub(w.registeredAt) > limit {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// RunWatchdog chạy vòng kiểm tra mỗi interval; tick stale → alert critical
// (Đợt V). Gọi một lần trong ArmLoops.
func (s *Supervisor) RunWatchdog(ctx context.Context, interval time.Duration) {
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		lastAlert := map[string]time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				now := time.Now()
				for _, name := range s.Stale(now) {
					// Cooldown 1 giờ mỗi tick — tránh spam alert mỗi 5 phút
					// cho cùng một tick kẹt.
					if la, ok := lastAlert[name]; ok && now.Sub(la) < time.Hour {
						continue
					}
					lastAlert[name] = now
					log.Printf("watchdog: tick %q stale — quá lâu chưa chạy", name)
					s.raiseAlert("tick_stale", "critical",
						fmt.Sprintf("Tác vụ nền %q quá lâu chưa chạy — có thể đã kẹt.", name),
						"Khởi động lại app nếu sau 30 phút vẫn báo.")
				}
			}
		}
	}()
}

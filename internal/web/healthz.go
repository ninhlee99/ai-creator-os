package web

// Health endpoint cho máy chạy 24/7 không người trông (Đợt S).
//
// Vấn đề: KeepAlive của launchd chỉ restart khi process *exit*. Process bị
// deadlock/treo (tick kẹt) thì launchd tưởng vẫn khỏe → không restart,
// không alert. /healthz cho phép watchdog ngoài (vd: cron kiểm tra định kỳ
// hoặc công cụ giám sát) phát hiện máy treo mà process chưa chết.
//
// Luôn trả 200 với JSON (kể cả khi db_ok=false) để phân biệt được "app treo"
// (không kết nối được) với "app sống nhưng DB có vấn đề" (kết nối được,
// db_ok=false). Không yêu cầu đăng nhập/onboard — cố ý để watchdog gọi được
// bất cứ lúc nào.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// TickBeat là heartbeat của một automation tick: lần chạy gần nhất, lần
// thành công gần nhất, lỗi gần nhất. Do supervisor tick ở cmd/aicos cập nhật.
type TickBeat struct {
	LastStart time.Time `json:"last_start"`
	LastOK    time.Time `json:"last_ok"`
	LastErr   string    `json:"last_err"`
	Runs      int64     `json:"runs"`
}

// SidecarBeat là sức khỏe một tiến trình phụ local (Đợt AK).
type SidecarBeat struct {
	State       string    `json:"state"` // stopped | running | error
	Restarts    int       `json:"restarts"`
	HealthErr   string    `json:"health_err,omitempty"`
	HealthFails int       `json:"health_fails"`
	CheckedAt   time.Time `json:"checked_at"`
}

// handleHealthz: GET /healthz → JSON trạng thái sống của app.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	dbOK := false
	if s.db != nil {
		if err := s.db.Ping(); err == nil {
			dbOK = true
		}
	}
	beats := map[string]TickBeat{}
	if s.TickBeats != nil {
		beats = s.TickBeats()
	}
	qOpen, qUntil := false, ""
	if s.QuotaCircuit != nil {
		qOpen, qUntil = s.QuotaCircuit()
	}
	stale := []string{}
	if s.StaleTicks != nil {
		if st := s.StaleTicks(); st != nil {
			stale = st
		}
	}
	sidecars := map[string]SidecarBeat{}
	if s.Sidecars != nil {
		sidecars = s.Sidecars()
	}
	var uptime int64
	if !s.StartedAt.IsZero() {
		uptime = int64(time.Since(s.StartedAt).Seconds())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                  dbOK,
		"version":             s.Cfg.Version,
		"uptime_sec":          uptime,
		"db_ok":               dbOK,
		"ticks":               beats,
		"quota_circuit_open":  qOpen,
		"quota_circuit_until": qUntil,
		"stale_ticks":         stale,
		"sidecars":            sidecars,
		// needs_restart: app còn trả lời nhưng có tác vụ nền kẹt — chỉ
		// khởi động lại mới gỡ được. Watchdog ngoài dựa vào cờ này.
		"needs_restart": len(stale) > 0,
	})
}

// ---------------------------------------------------------------------------
// Hiển thị ở Cài đặt · Hệ thống (Đợt R+S): Ninh thấy tick nào đang chạy,
// tick nào lỗi, và ngắt mạch quota có đang mở không — không cần đọc log.
// ---------------------------------------------------------------------------

// TickBeatView là một dòng heartbeat tick cho template.
type TickBeatView struct {
	Name    string
	Label   string // tên tiếng Việt cho người dùng (Đợt AK)
	Stale   bool   // quá hạn chưa chạy — có thể đã kẹt
	Runs    int64
	LastOK  string // "chưa xong lần nào" khi trống
	LastErr string
	Running bool // có lượt đang chạy (start sau lần OK cuối)
}

// QuotaCircuitView là trạng thái ngắt mạch quota cho template.
type QuotaCircuitView struct {
	Open  bool
	Until string
}

// tickBeatViews trả về các tick đã sắp xếp theo tên cho template.
func (s *Server) tickBeatViews() []TickBeatView {
	if s.TickBeats == nil {
		return nil
	}
	beats := s.TickBeats()
	stale := map[string]bool{}
	if s.StaleTicks != nil {
		for _, n := range s.StaleTicks() {
			stale[n] = true
		}
	}
	out := make([]TickBeatView, 0, len(beats))
	for name, b := range beats {
		v := TickBeatView{Name: name, Label: tickLabel(name), Stale: stale[name], Runs: b.Runs, LastErr: b.LastErr}
		if b.LastOK.IsZero() {
			v.LastOK = "chưa xong lần nào"
		} else {
			v.LastOK = b.LastOK.Format("02/01 15:04")
		}
		v.Running = !b.LastStart.IsZero() && b.LastStart.After(b.LastOK)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// tickLabels dịch tên kỹ thuật của tick sang tiếng người (Đợt AK): Ninh đọc
// "Đăng video reup" thay vì `reup_post`.
var tickLabels = map[string]string{
	"autopilot":      "Tự động đăng (autopilot)",
	"growth":         "Phát triển kênh",
	"accesstrade":    "Affiliate Accesstrade",
	"reup":           "Reup: tìm & tải video",
	"reup_transform": "Reup: biến đổi video",
	"reup_post":      "Reup: đăng video",
	"reup_kill":      "Reup: tắt nguồn 0 view",
	"story":          "Kể chuyện: dựng video",
	"story_public":   "Kể chuyện: công khai video",
	"story_retry":    "Kể chuyện: thử lại video lỗi",
	"selfheal":       "Tự hồi phục",
	"backup":         "Sao lưu tự động",
	"notify":         "Gửi thông báo",
	"draft_digest":   "Nhắc bản nháp chờ đăng",
}

func tickLabel(name string) string {
	if l, ok := tickLabels[name]; ok {
		return l
	}
	return name
}

// SidecarView là một dòng tiến trình phụ cho trang Cài đặt · Hệ thống.
type SidecarView struct {
	Label    string
	State    string // ok | warn | err | off
	Note     string
	Restarts int
}

var sidecarLabels = map[string]string{
	"llama-server": "LLM local (llama-server)",
	"vieneu":       "Giọng đọc VieNeu",
}

// sidecarViews dịch SidecarBeat sang dòng dễ đọc: đang chạy + kiểm tra gần
// nhất ổn → "ổn"; kiểm tra lỗi → "chập chờn" kèm lỗi; dừng/lỗi → nêu rõ.
func (s *Server) sidecarViews() []SidecarView {
	if s.Sidecars == nil {
		return nil
	}
	var out []SidecarView
	for name, b := range s.Sidecars() {
		v := SidecarView{Label: sidecarLabels[name], Restarts: b.Restarts}
		if v.Label == "" {
			v.Label = name
		}
		switch {
		case b.State == "error":
			v.State, v.Note = "err", "đã dừng sau nhiều lần lỗi — khởi động lại app"
		case b.State != "running":
			v.State, v.Note = "off", "chưa chạy"
		case b.HealthErr != "":
			v.State, v.Note = "warn", fmt.Sprintf("kiểm tra lỗi %d lần liên tiếp: %s", b.HealthFails, b.HealthErr)
		default:
			v.State, v.Note = "ok", "đang chạy"
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// quotaCircuitView trả về trạng thái ngắt mạch quota cho template.
func (s *Server) quotaCircuitView() QuotaCircuitView {
	if s.QuotaCircuit == nil {
		return QuotaCircuitView{}
	}
	open, until := s.QuotaCircuit()
	return QuotaCircuitView{Open: open, Until: until}
}

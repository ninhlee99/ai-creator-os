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
	})
}

// ---------------------------------------------------------------------------
// Hiển thị ở Cài đặt · Hệ thống (Đợt R+S): Ninh thấy tick nào đang chạy,
// tick nào lỗi, và ngắt mạch quota có đang mở không — không cần đọc log.
// ---------------------------------------------------------------------------

// TickBeatView là một dòng heartbeat tick cho template.
type TickBeatView struct {
	Name    string
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
	out := make([]TickBeatView, 0, len(beats))
	for name, b := range beats {
		v := TickBeatView{Name: name, Runs: b.Runs, LastErr: b.LastErr}
		if b.LastOK.IsZero() {
			v.LastOK = "chưa xong lần nào"
		} else {
			v.LastOK = b.LastOK.Format("02/01 15:04")
		}
		v.Running = !b.LastStart.IsZero() && b.LastStart.After(b.LastOK)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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

package web

// Bảng nhanh các công tắc lịch chính trên Trang chủ. Mỗi công tắc vẫn
// đọc/ghi đúng khóa setting mà trang chi tiết dùng.

import (
	"net/http"
	"net/url"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
)

type autoSwitch struct {
	Key   string
	Label string
	Hint  string
	Page  string // trang chi tiết để chỉnh sâu hơn
	On    bool
	Known bool // false when settings could not be read
}

// autoSwitchDefs: khóa → mặc định khi chưa lưu (khớp trang gốc).
var autoSwitchDefs = []struct {
	key, label, hint, page string
	def                    bool
}{
	{"growth.production_enabled", "Lịch sản xuất theo Growth plan", "Cho phép xử lý plan đến hạn; có thể tạo job và upload YouTube nếu provider, credential, quota và safety gate đều qua.", "/growth", true},
	{"autopilot_enabled", "Lịch autopilot affiliate", "Producer hiện dừng trước khi tạo video vì thiếu xác nhận dùng Google Flow credits; bật lịch không làm video chạy.", "/products", true},
	{automation.KeyATHunterEnabled, "Quét sản phẩm Accesstrade", "Feed hiện thiếu sales/rating evidence nên chưa đủ điều kiện tự tạo video.", "/products", true},
	{"viral.auto_produce_enabled", "Radar video xu hướng", "Tìm và lưu nguồn tham khảo; không tự dựng video viral.", "/viral", true},
	{automation.KeyStoryEnabled, "Tự viết & dựng truyện", "Cần bật lịch và có chủ đề; tự refill chủ đề là công tắc riêng.", "/stories", false},
	{automation.KeyStoryAutoPublish, "Tải truyện lên YouTube ở chế độ riêng tư", "Không tự chuyển video sang công khai.", "/stories", false},
}

func (s *Server) autoSwitches() []autoSwitch {
	out := make([]autoSwitch, 0, len(autoSwitchDefs))
	for _, d := range autoSwitchDefs {
		row := autoSwitch{Key: d.key, Label: d.label, Hint: d.hint, Page: d.page, On: d.def}
		if s.Ledger != nil {
			v, ok, err := s.Ledger.GetSetting(d.key)
			if err == nil {
				row.Known = true
				if ok {
					row.On = v == "1" || v == "true"
				}
			}
		}
		out = append(out, row)
	}
	return out
}

// handleAutoSwitch bật/tắt một công tắc trong bảng (POST /automation/switch).
func (s *Server) handleAutoSwitch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form không hợp lệ", http.StatusBadRequest)
		return
	}
	key := r.PostFormValue("key")
	onValue := r.PostFormValue("on")
	if onValue != "0" && onValue != "1" {
		http.Error(w, "trạng thái công tắc không hợp lệ", http.StatusBadRequest)
		return
	}
	label := ""
	for _, d := range autoSwitchDefs {
		if d.key == key {
			label = d.label
		}
	}
	if label == "" {
		http.Error(w, "công tắc không hợp lệ", http.StatusBadRequest)
		return
	}
	st := s.settings()
	if s.Ledger == nil {
		seeOther(w, r, "/?err="+url.QueryEscape("Kho cài đặt chưa sẵn sàng."))
		return
	}
	on := onValue == "1"
	v, msg := "0", "Đã tắt: "+label
	if on {
		v, msg = "1", "Đã bật: "+label
	}
	if err := st.Set(key, v); err != nil {
		seeOther(w, r, "/?err="+url.QueryEscape("Không lưu được: "+err.Error()))
		return
	}
	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "auto_switch", nil, msg, map[string]any{"key": key, "on": on})
	}
	seeOther(w, r, "/?ok="+url.QueryEscape(msg))
}

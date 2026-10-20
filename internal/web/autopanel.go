package web

// Bảng tự động hóa trên Trang chủ: gom mọi công tắc tự động (trước nằm rải
// ở 5 trang) về một chỗ. Mỗi công tắc vẫn đọc/ghi đúng khóa setting mà
// trang gốc dùng, nên bật/tắt ở đây hay ở trang gốc đều như nhau.

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
}

// autoSwitchDefs: khóa → mặc định khi chưa lưu (khớp trang gốc).
var autoSwitchDefs = []struct {
	key, label, hint, page string
	def                    bool
}{
	{"growth.production_enabled", "Sản xuất theo kế hoạch kênh", "Tự sản xuất + đăng theo plan 30 ngày", "/growth", true},
	{"autopilot_enabled", "Video affiliate tự động", "Tự dựng video từ sản phẩm trong kho theo lịch", "/products", true},
	{automation.KeyATHunterEnabled, "Săn sản phẩm Accesstrade", "Mỗi ngày tìm sản phẩm hoa hồng tốt (cần mã Accesstrade)", "/products", true},
	{"viral.auto_produce_enabled", "Video Viral Xu Hướng 100% Gốc", "Tự động bắt trend và sản xuất video viral ngắn 9:16 mỗi ngày", "/viral", true},
	{automation.KeyStoryEnabled, "Tự viết & dựng truyện", "Chỉ chạy khi đã bật và có chủ đề cấu hình", "/stories", false},
	{automation.KeyStoryAutoPublish, "Tự đăng truyện lên YouTube (riêng tư)", "Đăng private để bạn duyệt trước khi công khai", "/stories", false},
}

func (s *Server) autoSwitches() []autoSwitch {
	st := s.settings()
	out := make([]autoSwitch, 0, len(autoSwitchDefs))
	for _, d := range autoSwitchDefs {
		on := d.def
		if s.Ledger != nil {
			if v, ok := st.Get(d.key); ok {
				on = v == "1" || v == "true"
			}
		}
		out = append(out, autoSwitch{Key: d.key, Label: d.label, Hint: d.hint, Page: d.page, On: on})
	}
	return out
}

// handleAutoSwitch bật/tắt một công tắc trong bảng (POST /automation/switch).
func (s *Server) handleAutoSwitch(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	key := r.PostFormValue("key")
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
	on := r.PostFormValue("on") == "1"
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

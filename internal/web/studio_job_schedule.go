package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// handleStudioScheduleSave saves the autopilot schedule settings directly from Studio.
func (s *Server) handleStudioScheduleSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Kho cài đặt chưa khởi tạo"))
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Không đọc được dữ liệu form: "+err.Error()))
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	autoPub := "0"
	if r.PostFormValue("auto_publish") == "on" {
		autoPub = "1"
	}
	youTube := "0"
	if r.PostFormValue("youtube_enabled") == "on" {
		youTube = "1"
	}
	meta := "0"
	if r.PostFormValue("meta_enabled") == "on" {
		meta = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}

	st := s.settings()
	for _, setting := range []struct{ key, value string }{
		{SettingAutopilotEnabled, enabled},
		{SettingAutopilotInterval, strconv.Itoa(hours)},
		{SettingAutopilotAutoPublish, autoPub},
		{SettingAutopilotYouTube, youTube},
		{SettingAutopilotMeta, meta},
	} {
		if err := st.Set(setting.key, setting.value); err != nil {
			msg := fmt.Sprintf("Không lưu đủ cấu hình lịch (%s); một số giá trị có thể đã cập nhật. Hãy tải lại và kiểm tra.", setting.key)
			seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape(msg))
			return
		}
	}

	msg := "Đã cập nhật lịch tự động: TẮT"
	if enabled == "1" {
		msg = fmt.Sprintf("Đã lưu lịch mỗi %d giờ; producer đang tạm dừng nên chưa tạo video.", hours)
	}
	seeOther(w, r, "/studio/jobs?tab=schedule&msg="+url.QueryEscape(msg))
}

// handleStudioScheduleRunNow reports the current production gate instead of
// claiming that a paused Autopilot cycle was started.
func (s *Server) handleStudioScheduleRunNow(w http.ResponseWriter, r *http.Request) {
	seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Tạo video Autopilot đang tạm dừng cho tới khi có xác nhận dùng Google Flow credits theo kênh."))
}

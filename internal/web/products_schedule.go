package web

// Affiliate product catalog: discovery, shelf, and autopilot scheduling.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// scheduleView reads the current schedule state for the products page.
func (s *Server) scheduleView() (enabled bool, hours int, lastRun string, autoPublish bool) {
	// Automation switches read from the single settings facade (R2-W4):
	// ledger settings. Switches default ON when unset (Đợt 3); a stored
	// "0" is an explicit operator choice and always wins.
	st := s.settings()
	enabled = automation.AutopilotEnabled(st)
	hours = automation.AutopilotIntervalHours(st)
	lastRun = automation.AutopilotLastRun(st)
	autoPublish = automation.AutopilotAutoPublish(st)
	return
}

// handleProductsSchedule saves the autopilot background schedule from the
// web UI: enable toggle + interval in hours + auto-publish toggle.
func (s *Server) handleProductsSchedule(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		s.fail(w, fmt.Errorf("kho cài đặt chưa khởi tạo"), "save schedule")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse schedule form")
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
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}
	st := s.settings()
	if err := st.Set(SettingAutopilotEnabled, enabled); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotInterval, strconv.Itoa(hours)); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotAutoPublish, autoPub); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	msg := "Đã tắt lịch autopilot."
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch autopilot — chạy mỗi %d giờ cho các account đã sẵn sàng.", hours)
	}
	if autoPub == "1" {
		msg += " Video xong sẽ tự đăng TikTok (dạng nháp)."
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape(msg))
}

// The autopilot background schedule is managed in the web UI (not via env
// or CLI). R2-W4: these keys live in the single settings facade (ledger
// settings table) — see internal/automation. Exported so cmd/aicos (the
// scheduler wiring) and internal/automation read the same keys the UI
// writes.
const (
	SettingAutopilotEnabled     = "autopilot_enabled"
	SettingAutopilotInterval    = "autopilot_interval_hours"
	SettingAutopilotLastRun     = "autopilot_last_run"
	SettingAutopilotAutoPublish = "autopilot_auto_publish"
)

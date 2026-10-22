package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// handleSettingsMaster flips the persisted network-daemon master switch
// (R2-W4, R2-08). Default OFF; the network daemon reads the gate live,
// so no restart is needed. Every change is written to the decision log.
func (s *Server) handleSettingsMaster(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse master form")
		return
	}
	on := r.PostFormValue("value") == "1" || r.PostFormValue("on") == "1"
	if err := automation.SetMasterOn(s.settings(), on); err != nil {
		s.fail(w, err, "save master switch")
		return
	}
	msg := "Đã TẮT chế độ tự động: hệ thống đứng yên."
	if on {
		msg = "Đã BẬT chế độ tự động: hệ thống chạy theo lịch."
	}
	_ = s.Ledger.Decide("system", "master_switch", nil, msg, map[string]any{"on": on})
	// Đợt AG: quay lại trang đã bấm nút (dashboard hoặc system).
	back := r.PostFormValue("back")
	if back == "" || !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/settings/system"
	}
	seeOther(w, r, back+"?ok="+url.QueryEscape(msg))
}

// handleSettingsAPIBudget saves the UI-tuned daily API spend ceiling
// (R2-W4, R2-05). The persisted value wins over DAILY_API_BUDGET_USD.
func (s *Server) handleSettingsAPIBudget(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse budget form")
		return
	}
	raw := strings.TrimSpace(r.PostFormValue("value"))
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 || usd > 10000 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Ngân sách API không hợp lệ (0–10000 USD)."))
		return
	}
	if err := automation.SetAPIBudgetUSD(s.settings(), usd); err != nil {
		s.fail(w, err, "save api budget")
		return
	}
	_ = s.Ledger.Decide("system", "api_budget", nil,
		fmt.Sprintf("ngân sách API/ngày đặt thành $%.2f qua UI", usd),
		map[string]any{"usd": usd})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(fmt.Sprintf("Đã lưu ngân sách API: $%.2f/ngày.", usd)))
}

func (s *Server) handleSettingsDryRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse dry-run form")
		return
	}
	// NOTE: the required bugfix — dry_run is ON exactly when value == "on".
	on := r.PostFormValue("value") == "on"
	value := "0"
	if on {
		value = "1"
	}
	if err := s.Ledger.SetSetting("ops.dry_run", value); err != nil {
		s.fail(w, err, "save dry-run setting")
		return
	}
	s.Cfg.SetDryRun(on)
	if err := s.Ledger.Decide("human", "dry_run", nil,
		fmt.Sprintf("set dry_run=%v via dashboard", on), map[string]any{}); err != nil {
		log.Printf("web: decide dry_run: %v", err)
	}
	seeOther(w, r, "/settings/safety")
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(true)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"engaged via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide kill: %v", err)
	}
	seeOther(w, r, "/")
}

func (s *Server) handleUnkill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(false)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"released via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide unkill: %v", err)
	}
	seeOther(w, r, "/")
}

// ------------------------------------------------------- chain settings API

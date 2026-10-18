package web

// Dashboard overview page (GET /): stat cards, decision feed, growth
// alerts and the local runtime status block.

import (
	"net/http"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	commRevenue, _ := s.Ledger.TotalRevenue()
	today := s.today()
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions WHERE action != 'onboard' ORDER BY id DESC LIMIT 8")
	if err != nil {
		s.fail(w, err, "recent decisions")
		return
	}
	var recentJobs []studioJobView
	if s.Studio != nil {
		recentJobs = s.studioJobViews(s.Studio.ListJobs(6))
	}
	s.render(w, "dashboard", s.ctx(
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
		"AutoSwitches", s.autoSwitches(),
		"Money", s.moneyReadiness(),
		"Accounts", accounts,
		"ByStatus", sortedStatusCounts(accounts),
		"CommissionRevenue", commRevenue,
		"Decisions", decisions,
		"GrowthAlerts", s.recentGrowthAlerts(accounts),
		"Today", today,
		"NJobs", s.activeStudioJobs(),
		"RecentJobs", recentJobs,
		// Bắt đầu nhanh checklist state (trang chủ khi chưa có tài khoản).
		"HasKeys", s.hasAnyAPIKey(),
		"KeyWrong", s.geminiKeyWrong(),
		"HasStudioJobs", s.Studio != nil && len(s.Studio.ListJobs(1)) > 0,
		// R2-W4 (R2-08): chip công tắc chính — nhãn trung thực: đang TẮT
		// hay đang cho phép daemon chạy theo lịch.
		"MasterOn", automation.MasterOn(s.settings()),
		"Missions", s.listAIMissions(3),
		"LiveOperations", s.listLiveOperations(),
		"EnterpriseDepts", s.listDepartments(s.listAgents()),
	))
}

// hasAnyAPIKey reports whether any Gemini key is configured (keyring in the
// database or the GEMINI_API_KEYS environment variable).
func (s *Server) hasAnyAPIKey() bool {
	for _, k := range append(s.storedKeys("llm", "gemini"), s.storedKeys("tts", "gemini")...) {
		if looksLikeGeminiKey(strings.TrimSpace(k)) {
			return true
		}
	}
	return getenv("GEMINI_API_KEYS", "") != "" || getenv("TTS_API_KEY", "") != ""
}

// geminiKeyWrong: đã nhập key nhưng không key nào đúng dạng Gemini (dán
// nhầm) — Trang chủ báo rõ thay vì tick "đã có key".
func (s *Server) geminiKeyWrong() bool {
	ks := append(s.storedKeys("llm", "gemini"), s.storedKeys("tts", "gemini")...)
	return len(ks) > 0 && !s.hasAnyAPIKey()
}

// ---------------------------------------------------------------- accounts

// activeStudioJobs đếm video Studio/Kể chuyện đang chờ hoặc đang dựng
// (Đợt AK: trước đây thẻ "Video đang dựng" chỉ đếm job video chữ động nên
// luôn báo "chưa có" dù Studio đang dựng).
func (s *Server) activeStudioJobs() int {
	if s.Studio == nil {
		return 0
	}
	n := 0
	for _, j := range s.Studio.ListJobs(100) {
		if j.Status == "queued" || j.Status == "running" {
			n++
		}
	}
	return n
}

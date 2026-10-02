package web

// Dashboard overview page (GET /): stat cards, decision feed, growth
// alerts and the local runtime status block.

import (
	"net/http"
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
	eligible := 0
	for _, a := range accounts {
		if a.Status == "live_ready" || a.Status == "live" {
			eligible++
		}
	}
	revenue := s.scalarFloat("SELECT COALESCE(SUM(commission),0) FROM orders")
	commRevenue, _ := s.Ledger.TotalRevenue()
	s.ensureTodaySchedule()
	today := s.today()
	slots, err := s.Ledger.GetSlots(today)
	if err != nil {
		s.fail(w, err, "get slots")
		return
	}
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions ORDER BY id DESC LIMIT 8")
	if err != nil {
		s.fail(w, err, "recent decisions")
		return
	}
	s.render(w, "dashboard", s.ctx(
		"Accounts", accounts,
		"EligibleCount", eligible,
		"ByStatus", sortedStatusCounts(accounts),
		"Revenue", revenue,
		"CommissionRevenue", commRevenue,
		"LiveSessions", s.scalarInt("SELECT COUNT(*) FROM live_sessions"),
		"Slots", s.slotViews(slots),
		"Decisions", decisions,
		"GrowthAlerts", s.recentGrowthAlerts(accounts),
		"LocalRuntimes", s.localRuntimeViews(),
		"Today", today,
		"NJobs", s.Jobs.Count(),
		// Bắt đầu nhanh checklist state (trang chủ khi chưa có tài khoản).
		"HasKeys", s.hasAnyAPIKey(),
		"HasStudioJobs", s.Studio != nil && len(s.Studio.ListJobs(1)) > 0,
	))
}

// hasAnyAPIKey reports whether any Gemini key is configured (keyring in the
// database or the GEMINI_API_KEYS environment variable).
func (s *Server) hasAnyAPIKey() bool {
	if len(s.keyRingStatuses("tts", "gemini")) > 0 ||
		len(s.keyRingStatuses("llm", "gemini")) > 0 {
		return true
	}
	return getenv("GEMINI_API_KEYS", "") != "" || getenv("TTS_API_KEY", "") != ""
}

// ---------------------------------------------------------------- accounts

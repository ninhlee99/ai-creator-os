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
	commRevenue, _ := s.Ledger.TotalRevenue()
	var recentJobs []studioJobView
	if s.Studio != nil {
		recentJobs = s.studioJobViews(s.Studio.ListJobs(6))
	}
	recentPublishOutcomes, err := s.dashboardPublishOutcomes(6)
	if err != nil {
		s.fail(w, err, "load recent publish outcomes")
		return
	}
	s.render(w, "dashboard", s.ctx(
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
		"Accounts", accounts,
		"CommissionRevenue", commRevenue,
		"NJobs", s.activeStudioJobs(),
		"RecentJobs", recentJobs,
		"RecentPublishOutcomes", recentPublishOutcomes,
	))
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

package web

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) defaultViralRadarQuery() string {
	if s.Mgr != nil {
		if accounts, err := s.Mgr.List(); err == nil {
			for _, account := range accounts {
				if account != nil && strings.TrimSpace(account.Niche) != "" && account.Status != "paused" && account.Status != "retired" {
					return strings.TrimSpace(account.Niche)
				}
			}
		}
	}
	return "video viral Việt Nam"
}

func (s *Server) handleViralRadarRefresh(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Could not read radar form"), http.StatusSeeOther)
		return
	}
	query := strings.TrimSpace(r.PostFormValue("query"))
	if err := s.automation().RefreshViralRadarQuery(query); err != nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	notes := s.AutomationService().ViralRadarTick(r.Context(), true)
	message := "YouTube radar finished."
	if len(notes) > 0 {
		message = strings.Join(notes, " ")
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "missing") || strings.Contains(lower, "failed") || strings.Contains(lower, "could not") || strings.Contains(lower, "kill switch") {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/viral?notice="+url.QueryEscape(message), http.StatusSeeOther)
}

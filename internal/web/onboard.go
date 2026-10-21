package web

import (
	"net/http"
	"net/url"
	"strings"
)

// First-run wizard (R2-W7). main.go sets Server.Onboarding when the data
// directory has no ledger.db yet — i.e. the app has never run with real
// state. While onboarding, every route except the wizard itself and
// static assets redirects to /onboard; completing the wizard (one POST)
// drops the gate for the rest of the process lifetime.

// onboardGate keeps first-run users inside the wizard until it completes.
func (s *Server) onboardGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Onboarding {
			p := r.URL.Path
			// Đợt S: /healthz cho watchdog ngoài gọi bất cứ lúc nào.
			if p != "/onboard" && p != "/healthz" && !strings.HasPrefix(p, "/static/") && p != "/favicon.ico" {
				seeOther(w, r, "/onboard")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleOnboard renders the wizard (GET) and completes it (POST).
func (s *Server) handleOnboard(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			s.fail(w, err, "parse onboard form")
			return
		}
		dry := r.PostFormValue("dryrun") == "1"
		value := "0"
		if dry {
			value = "1"
		}
		if err := s.Ledger.SetSetting("ops.dry_run", value); err != nil {
			s.fail(w, err, "save onboarding dry-run setting")
			return
		}
		s.Cfg.SetDryRun(dry)
		_ = s.Ledger.Decide("human", "onboard", nil,
			"Hoàn tất thiết lập lần đầu.",
			map[string]any{"dry_run": dry, "data_dir": s.DataDir})
		s.Onboarding = false
		seeOther(w, r, "/?ok="+url.QueryEscape("Chào mừng! App đã sẵn sàng."))
		return
	}
	s.render(w, "onboard", s.ctx("SettingsPage", "onboard",
		"DataDir", s.DataDir,
		"Version", s.Cfg.Version,
	))
}

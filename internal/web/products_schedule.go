package web

// Affiliate product catalog: discovery, shelf, and autopilot scheduling.

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
)

// scheduleView reads the current schedule state for the products page.
func (s *Server) scheduleView() (enabled bool, hours int, lastRun string) {
	// Automation settings read from the single settings facade (R2-W4).
	st := s.settings()
	enabled = automation.AutopilotEnabled(st)
	hours = automation.AutopilotIntervalHours(st)
	lastRun = automation.AutopilotLastRun(st)
	return
}

// The autopilot background schedule is managed in the web UI (not via env
// or CLI). R2-W4: these keys live in the single settings facade (ledger
// settings table) — see internal/automation. Exported so cmd/aicos (the
// scheduler wiring) and internal/automation read the same keys the UI
// writes.
const (
	SettingAutopilotEnabled  = "autopilot_enabled"
	SettingAutopilotInterval = "autopilot_interval_hours"
	SettingAutopilotLastRun  = "autopilot_last_run"
)

package automation

import (
	"context"
	"time"
)

// Growth setting keys in the ledger settings table (kept identical to
// the web era so existing databases keep working).
const (
	growthProductionKey = "growth.production_enabled"
	growthLastSyncKey   = "growth.last_sync"
)

// productionEnabled is the service-side toggle check.
func (s *Service) productionEnabled() bool {
	if s.Settings == nil {
		return true
	}
	v, ok := s.Settings.Get(growthProductionKey)
	return !ok || v != "0"
}

// ProductionEnabled is the exported toggle check for the dashboard UI.
func (s *Service) ProductionEnabled() bool { return s.productionEnabled() }

// SetProductionEnabled persists the operator's choice ("1"/"0").
func (s *Service) SetProductionEnabled(on bool) error {
	if s.Settings == nil {
		return errNoLedger
	}
	if on {
		return s.Settings.Set(growthProductionKey, "1")
	}
	return s.Settings.Set(growthProductionKey, "0")
}

// ------------------------------------------------------------ the tick

// GrowthTick runs one scheduled TikTok production pass and an hourly
// metrics and affiliate-order sync. It runs by
// default (a stored "0" opts out); the cmd daemon calls it on a timer.
// Returned notes are for logs/UI — the pass also writes alerts and
// decision rows itself. force=true is the manual "chạy ngay" button (an
// explicit human command — still bounded by kill switch and dry-run).
func (s *Service) GrowthTick(ctx context.Context, force bool) []string {
	if s.Growth == nil {
		return nil
	}
	if s.killed() {
		return []string{"Kill switch đang bật — vòng growth đứng yên."}
	}
	if !force && !s.productionEnabled() {
		return nil
	}
	var notes []string

	if s.growthSyncDue() {
		// Affiliate money reconcile rides the hourly sync — real API
		// payload only, fail-closed (no source -> honest decision,
		// numbers stay 0).
		notes = append(notes, s.ReconcileCommissions(ctx)...)
		if accounts, err := s.Accounts.List(); err == nil {
			for _, a := range accounts {
				notes = append(notes, s.SyncOneAccount(ctx, a)...)
			}
			if s.Settings != nil {
				_ = s.Settings.Set(growthLastSyncKey, time.Now().Format(time.RFC3339))
			}
		}
	}

	accounts, err := s.Accounts.List()
	if err != nil {
		return append(notes, "Không đọc được danh sách tài khoản: "+err.Error())
	}
	for _, a := range accounts {
		switch a.Status {
		case "paused", "penalized", "retired":
			continue // never produce for stopped accounts (doc §4.2)
		}
		notes = append(notes, s.produceAccount(ctx, a)...)
		notes = append(notes, s.advanceAccount(ctx, a)...)
	}
	return notes
}

func (s *Service) growthSyncDue() bool {
	if s.Settings == nil {
		return true
	}
	raw, ok := s.Settings.Get(growthLastSyncKey)
	if !ok || raw == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, raw)
	return err != nil || time.Since(t) >= time.Hour
}

func variantLabel(v string) string {
	return "TikTok"
}

// -------------------------------------------------------- produce pass

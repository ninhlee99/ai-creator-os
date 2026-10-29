package automation

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ------------------------------------------------------------------ ports

// AccountManager is the account boundary the automation layer needs.
// *network.AccountManager satisfies it.
type AccountManager interface {
	List(statuses ...string) ([]*network.Account, error)
	Get(id int64) (*network.Account, error)
	Transition(id int64, status string, extra map[string]any) (*network.Account, error)
}

// EnvProvider resolves a config variable's current value: UI-saved values
// win over the process environment. The web server implements it
// (effectiveEnv). The automation layer never reads web.Config fields
// directly — that was the R2-05 staleness bug (UI save updated os env and
// DB but not the already-read Config struct).
type EnvProvider interface {
	EffectiveEnv(name string) (string, bool)
}

// ProduceRequest is one due plan item handed to the renderer.
type ProduceRequest struct {
	Account *network.Account
	Item    growth.PlanItem
}

// Producer renders plan items and reports render state. JobState returns
// ("", "") when the job is unknown.
type Producer interface {
	Enqueue(ctx context.Context, req ProduceRequest) (string, error)
	JobState(jobID string) (status, output string)
}

// StudioMaintenance exposes generic maintenance operations for rendered jobs.
type StudioMaintenance interface {
	SweepHungJobs(maxAge time.Duration) int
	PruneFailedWork() (int, int64)
	PruneOldJobs(maxAge time.Duration) int
}

// ExecutionGate reads current publishing safety switches.
type ExecutionGate interface {
	KillSwitch() bool
	DryRun() bool
}

// AutopilotRunner attempts affiliate cycles; Studio may stop before product
// discovery when account-level Google Flow credit authorization is absent.
// *studio.Autopilot satisfies it.
type AutopilotRunner interface {
	Run(ctx context.Context, accountID int64) (studio.Result, error)
	RunAll(ctx context.Context) []studio.Result
}

// OrdersSource is the affiliate-orders feed for money reconciliation.
// Tests inject a fake; production removed its TikTok Shop default in
// Đợt H1 (Accesstrade-only) — commission sync now rides ATOrderSyncTick.
// Payload shape mirrors a generic affiliate orders API.
type OrdersSource interface {
	AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error)
}

// ---------------------------------------------------------------- service

// Service is the hands-off execution layer: growth tick (plan ->
// production), the affiliate autopilot scheduler and other background
// maintenance. All dependencies arrive through the interfaces
// above; the service imports nothing web-related.
type Service struct {
	Growth   *growth.Store
	Ledger   *ledger.Ledger
	Accounts AccountManager
	Gate     ExecutionGate // nil = safest (dry-run on)
	Env      EnvProvider
	Settings Settings // the single config facade (ledger-backed)

	Producer  Producer
	Autopilot AutopilotRunner

	Products     *products.Store
	StudioRunner StudioRunner
	Maintenance  StudioMaintenance
	// DataDir thư mục dữ liệu app — disk guard (Đợt L) đo % đĩa đã dùng
	// tại đây để tự dọn khi đầy.
	DataDir string

	// CommissionSource overrides the affiliate-orders feed (tests inject a
	// fake). Nil = build the real one from the current env.
	CommissionSource OrdersSource

	// Timezone is the IANA name for "today" boundaries; falls back to
	// fixed UTC+7 like the rest of the app.
	Timezone string

	// Cadence knobs (were package-level consts in web; now service
	// fields with the same defaults — tunable without editing code).
	MaxProductionsPerTick int
	StaleItemDays         int
	DedupWindowDays       int
}

// NewService builds a Service with the historical cadence defaults.
func NewService() *Service {
	return &Service{
		MaxProductionsPerTick: 3,
		StaleItemDays:         14,
		DedupWindowDays:       45,
	}
}

// dryRun treats a nil Gate as "safest": nothing real runs without an
// explicit gate saying otherwise.
func (s *Service) dryRun() bool {
	if s.Gate == nil {
		return true
	}
	return s.Gate.DryRun()
}

func (s *Service) killed() bool {
	if s.Gate == nil {
		return false
	}
	return s.Gate.KillSwitch()
}

// producer resolves the render backend; nil when nothing is wired (the
// tick then marks items failed honestly instead of crashing).
func (s *Service) producer() Producer { return s.Producer }

// location resolves the service timezone like web.Server.location.
func (s *Service) location() *time.Location {
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	return time.FixedZone("ICT", 7*3600)
}

func (s *Service) today() string { return time.Now().In(s.location()).Format("2006-01-02") }

func (s *Service) nowISO() string { return time.Now().In(s.location()).Format("2006-01-02T15:04:05") }

// decide writes to the decision log; a broken ledger must never crash a
// tick — it is recorded in the returned notes instead.
func (s *Service) decide(agent, action string, target *string, reason string, inputs map[string]any) error {
	if s.Ledger == nil {
		return errNoLedger
	}
	return s.Ledger.Decide(agent, action, target, reason, inputs)
}

// OutputPath resolves a job's output filename to its full filesystem path.
func (s *Service) OutputPath(name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		return name
	}
	if s.DataDir != "" {
		cand := filepath.Join(s.DataDir, "output", name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	cand := filepath.Join("data", "output", name)
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	if _, err := os.Stat(name); err == nil {
		return name
	}
	if s.DataDir != "" {
		return filepath.Join(s.DataDir, "output", name)
	}
	return name
}

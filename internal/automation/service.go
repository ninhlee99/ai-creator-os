package automation

import (
	"context"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
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

// YTState is the honest YouTube-upload readiness of one account.
type YTState struct {
	HasClient  bool // OAuth app (client id + secret) configured in Settings
	HasToken   bool // per-account refresh token file present
	HasChannel bool // channel mapping attached (metrics + display)
	Privacy    string
}

// Ready reports whether an upload could actually be attempted.
func (st YTState) Ready() bool { return st.HasClient && st.HasToken }

// Label is the compact badge text for account rows.
func (st YTState) Label() string {
	switch {
	case st.Ready():
		return "sẵn sàng đăng"
	case !st.HasClient:
		return "thiếu OAuth app"
	default:
		return "thiếu token tài khoản"
	}
}

// Missing lists what blocks uploads, in Vietnamese, for item notes.
func (st YTState) Missing() []string {
	var m []string
	if !st.HasClient {
		m = append(m, "chưa nhập YOUTUBE_CLIENT_ID/YOUTUBE_CLIENT_SECRET trong Cài đặt")
	}
	if !st.HasToken {
		m = append(m, "tài khoản chưa hoàn tất OAuth YouTube (chưa có token)")
	}
	return m
}

// Uploader publishes rendered videos to YouTube.
type Uploader interface {
	State(a *network.Account) YTState
	Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult
}

// AutopilotRunner runs hands-off affiliate cycles (products -> video).
// *studio.Autopilot satisfies it.
type AutopilotRunner interface {
	Run(ctx context.Context, accountID int64) (studio.Result, error)
	RunAll(ctx context.Context) []studio.Result
}

// OrdersSource is the affiliate-orders feed for money reconciliation. The
// real *tiktok.ShopClient satisfies it once its endpoint paths are
// verified; tests inject a fake. Payload shape mirrors the TikTok Shop
// affiliate orders API.
type OrdersSource interface {
	AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error)
}

// ---------------------------------------------------------------- service

// Service is the hands-off execution layer: growth tick (plan ->
// production -> publish), the affiliate autopilot scheduler and the
// auto-publish hook. All dependencies arrive through the interfaces
// above; the service imports nothing web-related.
type Service struct {
	Growth   *growth.Store
	Ledger   *ledger.Ledger
	Accounts AccountManager
	Gate     network.Gate // kill switch / dry-run; nil = treat as safest (dry-run on)
	Env      EnvProvider
	Settings Settings // the single config facade (ledger-backed)

	Producer  Producer
	Uploader  Uploader
	Autopilot AutopilotRunner

	// JobStore is the studio job backend for the auto-publish hook.
	// Nil = the hook never fires (videos stay in Studio output).
	JobStore JobStore

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

// uploader resolves the YouTube backend; nil when nothing is wired.
func (s *Service) uploader() Uploader { return s.Uploader }

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

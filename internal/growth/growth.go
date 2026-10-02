// Package growth implements the Channel Growth MVP (docs/CHANNEL_GROWTH.md):
// per-account growth profiles and KPI targets, a deterministic 30-day
// content-plan engine, format kill/double-down/penalty decision rules, and
// provider metric snapshots.
//
// Honesty rules (same as the rest of the app): metrics only ever come from
// provider APIs — a source that is not connected returns ErrNotConnected and
// NO snapshot is written. The engine only schedules/describes production
// work; rendering stays in Studio and publishing stays in publishers.
package growth

import "time"

// Growth stages (docs/CHANNEL_GROWTH.md §4.1), ordered by lifecycle.
const (
	StageColdStart        = "cold_start"
	StageFormatTesting    = "format_testing"
	StageScaling          = "scaling"
	StageMonetizationPush = "monetization_push"
	StageMonetized        = "monetized"
	StageStalled          = "stalled"
)

// Plan variants (platform adaptations of one idea).
const (
	VariantTikTok  = "tiktok"
	VariantShorts  = "youtube_shorts"
	VariantYouTube = "youtube_long"
)

// Plan item statuses.
const (
	ItemPlanned        = "planned"
	ItemProducing      = "producing"
	ItemQC             = "qc"
	ItemQueued         = "queued"
	ItemProduced       = "produced"        // render finished, not (yet) published
	ItemWaitingConnect = "waiting_connect" // rendered, blocked on missing platform credentials
	ItemWaitingQuota   = "waiting_quota"   // rendered, blocked on the daily API quota
	ItemPublished      = "published"
	ItemDropped        = "dropped"
	ItemFailed         = "failed" // linked Studio job failed / vanished
)

// Format verdicts.
const (
	VerdictTesting = "testing"
	VerdictWinner  = "winner"
	VerdictKilled  = "killed"
)

// YPPDeadline is the date after which NEW YouTube Partner Program
// applicants face doubled thresholds (docs/CHANNEL_GROWTH.md §2.2). Plans
// for channels not yet monetized count down to it.
const YPPDeadline = "2027-02-01"

// CanaryDays is the low-cadence probation window for new accounts (§5.3).
const CanaryDays = 7

// Config holds the governance thresholds for growth decisions. All rules
// read from here so they stay tunable in one place (docs §0: thresholds
// are Governance parameters, never per-decision human calls).
type Config struct {
	KillMinVideos       int     `json:"kill_min_videos"`       // videos a format needs before it can be killed
	KillMinDays         int     `json:"kill_min_days"`         // days of data a format needs before it can be killed
	KillCompletionFloor float64 `json:"kill_completion_floor"` // median completion below this kills (0..1)
	KillProxyFloor      float64 `json:"kill_proxy_floor"`      // fallback when completion is unavailable: median (shares+saves)/views
	DoubleDownFactor    float64 `json:"double_down_factor"`    // video views >= factor * account median => series
	BreakoutFactor      float64 `json:"breakout_factor"`       // video views >= factor * account median => breakout
	PenaltyViewsDrop    float64 `json:"penalty_views_drop"`    // views_30d drop fraction that signals a penalty
	StalledDays         int     `json:"stalled_days"`          // window for the stalled check
	StalledGrowthMax    float64 `json:"stalled_growth_max"`    // follower growth below this over the window = stalled
}

// DefaultConfig returns the doc §4.2 defaults.
func DefaultConfig() Config {
	return Config{
		KillMinVideos:       8,
		KillMinDays:         14,
		KillCompletionFloor: 0.35,
		KillProxyFloor:      0.01,
		DoubleDownFactor:    5,
		BreakoutFactor:      10,
		PenaltyViewsDrop:    0.70,
		StalledDays:         14,
		StalledGrowthMax:    0.02,
	}
}

// Profile is one account's growth state (growth_profiles row).
type Profile struct {
	AccountID   int64
	Stage       string
	CadenceDay  float64 // current items/day the system aims for (informational)
	HorizonDays int
	StartedAt   string
	UpdatedAt   string
}

// Target is one KPI milestone (growth_targets row).
type Target struct {
	ID        int64
	AccountID int64
	Metric    string // followers | subs | watch_hours_12m | shorts_views_90d
	Threshold float64
	Label     string
	Deadline  string // "" = no deadline
	ReachedAt string // "" = not reached yet
}

// Plan is one rolling content-plan revision (content_plans row); a new plan
// supersedes the previous one, history is never rewritten.
type Plan struct {
	ID        int64
	AccountID int64
	Phase     string // d30 | d60 | d90
	Status    string // active | superseded
	Rationale string
	CreatedAt string
}

// PlanItem is one scheduled piece of content (content_plan_items row).
// The phase-2 fields record the production/publishing trail: which Studio
// job rendered it, the concept fingerprint used by the anti-duplicate
// guard, the variant lineage (Group + RelatedItemID), and the honest
// publish state (PubNote) — never a fabricated "published".
type PlanItem struct {
	ID            int64
	PlanID        int64
	AccountID     int64
	PlannedFor    string // YYYY-MM-DD (ICT)
	FormatID      string
	Variant       string // tiktok | youtube_shorts | youtube_long
	Topic         string
	Hook          string
	SeriesEp      int
	Status        string
	ContentItemID int64
	PublishedRef  string
	// Phase 2 — production + anti-duplicate trail.
	StudioJobID   string // studio_jobs.id (TEXT) that rendered this item
	ConceptText   string // normalized concept the dedup guard compared
	ConceptHash   string // sha256 over the concept token set
	VariantGroup  string // "YYYY-MM-DD|<format_id>" — variants of one idea
	DedupAction   string // "" | "angle_regenerated"
	Attempts      int    // failed enqueue attempts so far
	PubTitle      string // per-variant publish title decided at production
	PubCaption    string // per-variant caption/description base
	PubNote       string // honest publish/production state note (Vietnamese)
	RelatedItemID int64  // plan item of the sibling variant (Short <-> long)
	ProducedAt    string // when the render finished
}

// FormatStat is the aggregated score of one format family on one account.
type FormatStat struct {
	AccountID        int64
	FormatID         string
	Videos           int
	MedianCompletion *float64 // nil when the platform does not expose it
	MedianProxyRate  *float64 // median (shares+saves)/views fallback signal
	MedianViews      float64
	AvgViews         float64
	FollowsPer1k     *float64
	Verdict          string
	VerdictReason    string
	UpdatedAt        string
}

// Snapshot is one provider-metric reading for an account. Pointer fields
// stay nil when the provider did not supply that metric — never invent 0.
type Snapshot struct {
	AccountID int64
	TakenAt   time.Time
	Source    string // youtube_api | tiktok_api | import
	Followers *int64
	Views30d  *float64
	Videos    *int64
	Extra     map[string]any // watch_hours_12m, shorts_views_90d, views_total...
}

// Alert is one growth alert (growth_alerts row). Alerts are notify-only and
// always carry the action the system already took.
type Alert struct {
	ID          int64
	AccountID   int64  // 0 = network-wide
	Severity    string // info | warn | critical
	Kind        string // breakout | penalty_signal | stalled | milestone | quota | token_expired | plan | format
	Message     string
	ActionTaken string
	CreatedAt   string
}

// VideoStat is one published video's latest metrics, joined to its format.
type VideoStat struct {
	VideoID     string
	FormatID    string
	Views       int64
	Completion  *float64
	PublishedAt time.Time
}

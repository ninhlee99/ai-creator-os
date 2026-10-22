package growth

import (
	"fmt"
	"sort"
	"time"
)

type FormatInput struct {
	FormatID         string
	Videos           int
	FirstVideoAt     time.Time // zero = unknown
	MedianCompletion *float64
	MedianProxyRate  *float64 // median (shares+saves)/views when completion is unavailable
}

// DecideKill implements doc §4.2: a format dies only with enough evidence
// (>= 8 videos AND >= 14 days) and median completion under the floor. When
// the platform hides completion (TikTok Display API), the share/save proxy
// rate stands in. Returns (kill, human-readable reason).
func DecideKill(cfg Config, in FormatInput, now time.Time) (bool, string) {
	if in.Videos < cfg.KillMinVideos {
		return false, ""
	}
	if in.FirstVideoAt.IsZero() || now.Sub(in.FirstVideoAt) < time.Duration(cfg.KillMinDays)*24*time.Hour {
		return false, ""
	}
	if in.MedianCompletion != nil {
		if *in.MedianCompletion < cfg.KillCompletionFloor {
			return true, fmt.Sprintf("median completion %.0f%% < ngưỡng %.0f%% sau %d video",
				*in.MedianCompletion*100, cfg.KillCompletionFloor*100, in.Videos)
		}
		return false, ""
	}
	if in.MedianProxyRate != nil && *in.MedianProxyRate < cfg.KillProxyFloor {
		return true, fmt.Sprintf("tỉ lệ share/save %.2f%% < ngưỡng proxy %.2f%% sau %d video (không có completion)",
			*in.MedianProxyRate*100, cfg.KillProxyFloor*100, in.Videos)
	}
	return false, ""
}

// DecideDoubleDown: one video at >= 5x the account's 30-day median views
// earns a 3-episode series (doc §4.2). Median <= 0 means no baseline yet.
// Factor <= 0 disables the rule (UI có thể đặt 0 để tắt).
func DecideDoubleDown(cfg Config, accountMedianViews float64, v VideoStat) bool {
	if cfg.DoubleDownFactor <= 0 {
		return false
	}
	return accountMedianViews > 0 && float64(v.Views) >= cfg.DoubleDownFactor*accountMedianViews
}

// DecideBreakout: >= 10x median — triggers the fast 24h replan.
// Factor <= 0 disables the rule.
func DecideBreakout(cfg Config, accountMedianViews float64, v VideoStat) bool {
	if cfg.BreakoutFactor <= 0 {
		return false
	}
	return accountMedianViews > 0 && float64(v.Views) >= cfg.BreakoutFactor*accountMedianViews
}

// PenaltyInput is the evidence for the auto-pause rule.
type PenaltyInput struct {
	PolicyErrors  int           // platform policy/API violation errors in the window
	RemovedVideos int           // videos taken down in the window
	Views30dNow   float64       // current 30-day views (0 with HasViews=false)
	Views30dPrev  float64       // previous 30-day views
	HasViews      bool          // both view figures are real readings
	ViewsSpan     time.Duration // time between the two view readings (đợt AA)
}

// DecidePenalty implements doc §4.2: any policy error, any takedown, or a
// >70% views collapse not explained by cadence pauses the account.
// Đợt AA: cú rơi view chỉ tính khi CẢ HAI reading > 0 và cách nhau ≥ 24h —
// metric 30 ngày không thể rơi thật 70% trong 1h, nên trường hợp đó là số
// liệu lỗi (token hết hạn trả 0, API glitch) và không được pause oan.
func DecidePenalty(cfg Config, in PenaltyInput) (bool, string) {
	if in.PolicyErrors > 0 {
		return true, fmt.Sprintf("%d lỗi chính sách từ API nền tảng", in.PolicyErrors)
	}
	if in.RemovedVideos > 0 {
		return true, fmt.Sprintf("%d video bị gỡ trong kỳ", in.RemovedVideos)
	}
	if in.HasViews && in.Views30dPrev > 0 && in.Views30dNow > 0 && in.ViewsSpan >= 24*time.Hour {
		drop := (in.Views30dPrev - in.Views30dNow) / in.Views30dPrev
		if drop > cfg.PenaltyViewsDrop {
			return true, fmt.Sprintf("view 30 ngày rơi %.0f%% (ngưỡng %.0f%%)", drop*100, cfg.PenaltyViewsDrop*100)
		}
	}
	return false, ""
}

// StalledInput is the evidence for the deep-replan trigger.
type StalledInput struct {
	Days             int      // days under observation
	FollowerGrowth   float64  // fractional follower growth over the window
	MedianCompletion *float64 // nil = unknown (rule then ignores completion)
}

// DecideStalled: 14 days of <2% follower growth with weak completion
// means the format families are wrong, not the cadence (doc §4.2).
func DecideStalled(cfg Config, in StalledInput) bool {
	if in.Days < cfg.StalledDays || in.FollowerGrowth >= cfg.StalledGrowthMax {
		return false
	}
	if in.MedianCompletion != nil && *in.MedianCompletion >= cfg.KillCompletionFloor {
		return false
	}
	return true
}

// ------------------------------------------------------------ stage moves

// StageInput is the evidence for stage advancement.
type StageInput struct {
	DaysSinceStart   int
	CanaryClean      bool // canary passed: QC clean, 0 policy errors, completion above hard floor
	HasWinner        bool // at least one format reached winner verdict
	NearMonetization bool // next KPI milestone is within reach (caller-computed)
	Monetized        bool // platform monetization gate passed
}

// AdvanceStage moves a profile one step along the lifecycle when the
// evidence allows (doc §4.1/§5.3). It never skips a stage and never moves
// backwards except stalled → format_testing after a clean replan.
func AdvanceStage(cur string, in StageInput) string {
	switch cur {
	case StageColdStart:
		if in.DaysSinceStart >= CanaryDays && in.CanaryClean {
			return StageFormatTesting
		}
	case StageFormatTesting:
		if in.HasWinner {
			return StageScaling
		}
	case StageScaling:
		if in.NearMonetization {
			return StageMonetizationPush
		}
	case StageMonetizationPush:
		if in.Monetized {
			return StageMonetized
		}
	case StageStalled:
		if in.CanaryClean {
			return StageFormatTesting
		}
	}
	return cur
}

// Median is a small exported helper (used for views/completion medians).
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

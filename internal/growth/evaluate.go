package growth

import (
	"fmt"
	"time"
)

// EvalResult summarizes one Evaluate pass: what the system decided and did.
type EvalResult struct {
	StageBefore  string
	StageAfter   string
	PauseAccount bool
	PauseReason  string
	Actions      []string // human-readable log lines (Vietnamese)
}

// Evaluate runs the growth decision loop for one account (doc §4.2):
// recompute format stats → kill/double-down verdicts → penalty auto-pause
// → stalled detection → stage advancement. Every decision is written to the
// shared decisions audit trail and, when it matters, surfaced as a
// notify-only alert that names the action already taken.
//
// PauseAccount is only a recommendation here: the caller owns the account
// lifecycle and applies the legal status transition (AccountManager).
// With no metric data yet, Evaluate honestly decides nothing.
func (s *Store) Evaluate(accountID int64, username string, cfg Config, now time.Time) (*EvalResult, error) {
	prof, err := s.GetProfile(accountID)
	if err != nil {
		return nil, err
	}
	if prof == nil {
		return nil, fmt.Errorf("growth: account %d chưa có hồ sơ growth", accountID)
	}
	res := &EvalResult{StageBefore: prof.Stage, StageAfter: prof.Stage}

	stats, err := s.RecomputeFormatStats(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := s.latestVideoRows(accountID)
	if err != nil {
		return nil, err
	}
	medViews := accountMedianViews(rows)

	// Per-format first-seen time (first metric reading of its oldest video).
	firstSeen := map[string]time.Time{}
	for _, r := range rows {
		if r.FormatID == "" {
			continue
		}
		if t, ok := firstSeen[r.FormatID]; !ok || r.TakenAt.Before(t) {
			firstSeen[r.FormatID] = r.TakenAt
		}
	}

	hasWinner := false
	for _, st := range stats {
		// Kill rule (only verdicts still in testing can die).
		if st.Verdict == VerdictTesting {
			kill, reason := DecideKill(cfg, FormatInput{
				FormatID:         st.FormatID,
				Videos:           st.Videos,
				FirstVideoAt:     firstSeen[st.FormatID],
				MedianCompletion: st.MedianCompletion,
				MedianProxyRate:  st.MedianProxyRate,
			}, now)
			if kill {
				st.Verdict = VerdictKilled
				st.VerdictReason = reason
				if err := s.upsertFormatStat(st); err != nil {
					return nil, err
				}
				_ = s.Decide("growth_analyst", "kill_format", username,
					reason, map[string]any{"format": st.FormatID, "videos": st.Videos})
				_ = s.InsertAlert(Alert{AccountID: accountID, Severity: "warn", Kind: "format",
					Message:     fmt.Sprintf("Đã dừng format %q: %s", st.FormatID, reason),
					ActionTaken: "Director ngừng lên plan cho format này"})
				res.Actions = append(res.Actions, "kill format "+st.FormatID+": "+reason)
				continue
			}
		}
		// Double-down: any video at >= 5x account median marks a winner.
		if st.Verdict == VerdictTesting {
			for _, r := range rows {
				if r.FormatID != st.FormatID {
					continue
				}
				vs := VideoStat{VideoID: r.VideoID, FormatID: r.FormatID, Views: r.Views}
				if DecideDoubleDown(cfg, medViews, vs) {
					st.Verdict = VerdictWinner
					st.VerdictReason = fmt.Sprintf("video %s đạt %.0f view (>= %.0fx median %.0f)",
						r.VideoID, float64(r.Views), cfg.DoubleDownFactor, medViews)
					if err := s.upsertFormatStat(st); err != nil {
						return nil, err
					}
					_ = s.Decide("growth_analyst", "double_down_format", username,
						st.VerdictReason, map[string]any{"format": st.FormatID, "video": r.VideoID})
					_ = s.InsertAlert(Alert{AccountID: accountID, Severity: "info", Kind: "format",
						Message:     fmt.Sprintf("Format %q thắng: %s", st.FormatID, st.VerdictReason),
						ActionTaken: "Hệ thống sẽ sinh series 3 tập tiếp theo trong plan kế"})
					res.Actions = append(res.Actions, "double-down format "+st.FormatID)
					break
				}
			}
		}
		if st.Verdict == VerdictWinner {
			hasWinner = true
		}
		// Breakout: >= 10x median gets its own fast-replan alert.
		for _, r := range rows {
			if r.FormatID != st.FormatID {
				continue
			}
			if DecideBreakout(cfg, medViews, VideoStat{Views: r.Views}) {
				_ = s.Decide("growth_analyst", "breakout_video", username,
					fmt.Sprintf("video %s đạt %.0f view (>= %.0fx median)", r.VideoID, float64(r.Views), cfg.BreakoutFactor),
					map[string]any{"format": st.FormatID, "video": r.VideoID})
				res.Actions = append(res.Actions, "breakout video "+r.VideoID)
				break
			}
		}
	}

	// Penalty: views collapse across the two newest 30-day readings.
	// Policy-error/takedown counts have no provider source in the MVP, so
	// they stay 0 — the views leg is the only live signal for now.
	penalty := PenaltyInput{}
	if series, err := s.SnapshotSeries(accountID, 10); err == nil {
		var readings []float64
		for _, sn := range series {
			if sn.Views30d != nil {
				readings = append(readings, *sn.Views30d)
			}
		}
		if len(readings) >= 2 {
			penalty.Views30dPrev = readings[len(readings)-2]
			penalty.Views30dNow = readings[len(readings)-1]
			penalty.HasViews = true
		}
	}
	if pause, reason := DecidePenalty(cfg, penalty); pause {
		res.PauseAccount = true
		res.PauseReason = reason
		_ = s.Decide("governance", "auto_pause_account", username, reason,
			map[string]any{"views_prev": penalty.Views30dPrev, "views_now": penalty.Views30dNow})
		_ = s.InsertAlert(Alert{AccountID: accountID, Severity: "critical", Kind: "penalty_signal",
			Message:     "Tín hiệu phạt: " + reason,
			ActionTaken: "Hệ thống tự tạm dừng tài khoản; format liên quan bị gắn cờ tránh trên toàn mạng"})
		res.Actions = append(res.Actions, "auto-pause: "+reason)
		return res, nil // paused: no stage work this pass
	}

	// Stalled: follower growth flat over the snapshot window.
	stage := prof.Stage
	if stage == StageFormatTesting || stage == StageScaling || stage == StageMonetizationPush {
		if series, err := s.SnapshotSeries(accountID, 30); err == nil && len(series) >= 2 {
			first, last := series[0], series[len(series)-1]
			if first.Followers != nil && last.Followers != nil && *first.Followers > 0 {
				spanDays := int(last.TakenAt.Sub(first.TakenAt).Hours() / 24)
				growth := float64(*last.Followers-*first.Followers) / float64(*first.Followers)
				var medComp *float64
				for _, st := range stats {
					if st.MedianCompletion != nil {
						medComp = st.MedianCompletion
						break
					}
				}
				if DecideStalled(cfg, StalledInput{Days: spanDays, FollowerGrowth: growth, MedianCompletion: medComp}) {
					if err := s.SetStage(accountID, StageStalled); err != nil {
						return nil, err
					}
					res.StageAfter = StageStalled
					_ = s.Decide("growth_analyst", "mark_stalled", username,
						fmt.Sprintf("follower tăng %.1f%% trong %d ngày", growth*100, spanDays), nil)
					_ = s.InsertAlert(Alert{AccountID: accountID, Severity: "warn", Kind: "stalled",
						Message:     "Kênh đang chững số liệu",
						ActionTaken: "Hệ thống kích hoạt replan sâu: săn lại format trong cùng trụ persona"})
					res.Actions = append(res.Actions, "mark stalled")
					return res, nil
				}
			}
		}
	}

	// Stage advancement (never while paused/stalled this pass).
	started, _ := time.Parse("2006-01-02 15:04:05", prof.StartedAt)
	days := 0
	if !started.IsZero() {
		days = int(now.Sub(started).Hours() / 24)
	}
	alerts, _ := s.ListAlerts(accountID, 50)
	penaltySeen := false
	for _, a := range alerts {
		if a.Kind == "penalty_signal" {
			penaltySeen = true
			break
		}
	}
	latest, _ := s.LatestSnapshot(accountID)
	var followers float64
	if latest != nil && latest.Followers != nil {
		followers = float64(*latest.Followers)
	}
	targets, _ := s.ListTargets(accountID)
	next := AdvanceStage(stage, StageInput{
		DaysSinceStart:   days,
		CanaryClean:      !penaltySeen,
		HasWinner:        hasWinner,
		NearMonetization: nearMonetization(targets, followers),
		Monetized:        monetizationGatePassed(targets),
	})
	if next != stage {
		if err := s.SetStage(accountID, next); err != nil {
			return nil, err
		}
		res.StageAfter = next
		_ = s.Decide("growth_analyst", "advance_stage", username,
			fmt.Sprintf("%s -> %s", stage, next), map[string]any{"days": days})
		res.Actions = append(res.Actions, "advance stage "+stage+" -> "+next)
	}
	return res, nil
}

// nearMonetization: the account is within 20% of its next unreached
// follower/sub milestone.
func nearMonetization(targets []Target, followers float64) bool {
	if followers <= 0 {
		return false
	}
	for _, t := range targets {
		if t.ReachedAt == "" && (t.Metric == "followers" || t.Metric == "subs") {
			return followers >= 0.8*t.Threshold
		}
	}
	return false
}

// monetizationGatePassed: TikTok = the 1.000-follower LIVE milestone;
// YouTube = 1.000 subs AND (4.000h OR 10M Shorts views) reached.
func monetizationGatePassed(targets []Target) bool {
	reached := map[string]bool{}
	for _, t := range targets {
		if t.ReachedAt == "" {
			continue
		}
		switch {
		case t.Metric == "followers" && t.Threshold >= 1000:
			return true // TikTok gate
		case t.Metric == "subs" && t.Threshold >= 1000:
			reached["subs1000"] = true
		case t.Metric == "watch_hours_12m" && t.Threshold >= 4000:
			reached["hours"] = true
		case t.Metric == "shorts_views_90d" && t.Threshold >= 10000000:
			reached["shorts"] = true
		}
	}
	return reached["subs1000"] && (reached["hours"] || reached["shorts"])
}

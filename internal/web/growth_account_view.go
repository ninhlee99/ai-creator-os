package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"time"
)

type growthTargetView struct {
	growth.Target
	Value    float64
	Pct      float64
	HasValue bool
}

// accountGrowthView assembles the growth block for the account detail page.
func (s *Server) accountGrowthView(a *network.Account) map[string]any {
	out := map[string]any{"GrowthFollowersLabel": growthFollowersLabel("")}
	if s.Growth == nil {
		return out
	}
	prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a))
	if err != nil || prof == nil {
		return out
	}
	out["GrowthProfile"] = prof
	latest, _ := s.Growth.LatestSnapshot(a.ID)
	out["GrowthHasSnapshot"] = latest != nil
	if v, ok := metricValue(latest, "followers"); ok {
		out["GrowthFollowers"] = int64(v)
		out["GrowthHasFollowers"] = true
		out["GrowthFollowersLabel"] = growthFollowersLabel(latest.Source)
	}
	if targets, err := s.Growth.ListTargets(a.ID); err == nil {
		views := make([]growthTargetView, 0, len(targets))
		for _, t := range targets {
			v := growthTargetView{Target: t}
			if val, ok := metricValue(latest, t.Metric); ok {
				v.Value, v.HasValue = val, true
			}
			if t.Threshold > 0 {
				v.Pct = v.Value / t.Threshold * 100
				if v.Pct > 100 {
					v.Pct = 100
				}
			}
			views = append(views, v)
		}
		out["GrowthTargets"] = views
	}
	if plan, items, err := s.Growth.ActivePlan(a.ID); err == nil && plan != nil {
		out["GrowthPlan"] = plan
		today := time.Now().Format("2006-01-02")
		var upcoming []growth.PlanItem
		var recent []growth.PlanItem
		for _, it := range items {
			if it.Status == growth.ItemPlanned && it.PlannedFor >= today {
				upcoming = append(upcoming, it)
			} else if it.Status != growth.ItemPlanned {
				recent = append(recent, it)
			}
		}
		if len(upcoming) > 7 {
			upcoming = upcoming[:7]
		}
		// Newest production activity first.
		for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
			recent[i], recent[j] = recent[j], recent[i]
		}
		if len(recent) > 7 {
			recent = recent[:7]
		}
		out["GrowthPlanItems"] = upcoming
		out["GrowthRecentItems"] = recent
	}
	if stats, err := s.Growth.ListFormatStats(a.ID); err == nil {
		out["GrowthStats"] = stats
	}
	if alerts, err := s.Growth.ListAlerts(a.ID, 5); err == nil {
		out["GrowthAlerts"] = alerts
	}
	return out
}

func growthFollowersLabel(source string) string {
	switch source {
	case "youtube_api":
		return "YouTube subscribers"
	case "tiktok_api":
		return "TikTok followers"
	default:
		return "Followers · nguồn chưa rõ"
	}
}

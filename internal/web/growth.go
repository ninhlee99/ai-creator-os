package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"net/http"
	"sort"
	"time"
)

// ------------------------------------------------------------- /growth

// growthAccountRow is one account's growth state on the overview page.
type growthAccountRow struct {
	Account        network.Account
	Stage          string
	NextLabel      string
	NextMetric     string
	NextValue      float64
	NextTarget     float64
	NextPct        float64
	HasNext        bool
	HasSnapshot    bool
	Followers      int64
	FollowersLabel string
	ViewsTotal     int64
	YTConnected    bool
	YTUpload       string // YouTube upload readiness label (phase 2)
	YTReady        bool
	YPP            []growth.YPPTierProgress // chỉ kênh YouTube
}

type growthAlertRow struct {
	growth.Alert
	Username string
}

type growthItemRow struct {
	growth.PlanItem
	Username string
}

// primaryPlatform: accounts are TikTok-first; an account that carries
// YouTube content types is treated as a YouTube channel for growth.
func primaryPlatform(a *network.Account) string {
	if len(a.YoutubeContentTypes) > 0 {
		return "youtube"
	}
	return "tiktok"
}

func hasYouTube(a *network.Account) bool {
	return a.YoutubeChannel != "" || len(a.YoutubeContentTypes) > 0
}

func metricValue(snap *growth.Snapshot, metric string) (float64, bool) {
	if snap == nil {
		return 0, false
	}
	switch metric {
	case "followers", "subs", "subscribers":
		if snap.Followers != nil {
			return float64(*snap.Followers), true
		}
	case "watch_hours_12m", "views_total", "shorts_views_90d":
		if v, ok := snap.Extra[metric]; ok {
			switch n := v.(type) {
			case int64:
				return float64(n), true
			case float64:
				return n, true
			}
		}
	}
	return 0, false
}

// growthView builds all data for the /growth page (also reused after a sync).
func (s *Server) growthView() map[string]any {
	ctx := map[string]any{}
	accounts, err := s.Mgr.List()
	if err != nil {
		ctx["Error"] = "Không đọc được danh sách tài khoản: " + err.Error()
		return ctx
	}
	ctx["Accounts"] = accounts
	// Phase-2 production defaults — set before any early return so the
	// production card renders honestly even with zero accounts.
	ctx["ProdEnabled"] = s.GrowthProductionEnabled()
	ctx["ProdDryRun"] = s.Cfg != nil && s.Cfg.DryRun()
	ctx["Thresholds"] = automation.LoadThresholds(s.settings())
	ctx["QuotaUsed"] = 0
	ctx["QuotaLeft"] = growth.QuotaUploadsLeft(0)
	ctx["QuotaLimit"] = growth.DailyQuotaUnits
	ctx["QuotaPerUpload"] = growth.UploadCostUnits
	ctx["QuotaReady"] = false
	ctx["YTPrivacy"] = "private"
	if s.Growth == nil {
		ctx["Error"] = "Module Phát triển kênh chưa khởi tạo được (xem log máy chủ)."
		return ctx
	}
	ytKeyVal, _ := s.effectiveEnv("YOUTUBE_API_KEY")
	ytKey := ytKeyVal != ""
	today := time.Now().Format("2006-01-02")
	var rows []growthAccountRow
	var alerts []growthAlertRow
	var upcoming []growthItemRow
	var totalYTSubscribers, totalTikTokFollowers, totalViews int64
	for i := range accounts {
		a := accounts[i]
		prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a))
		if err != nil || prof == nil {
			continue
		}
		row := growthAccountRow{Account: *a, Stage: prof.Stage, YTConnected: ytKey && a.YoutubeChannel != ""}
		ytState := s.yt().State(a)
		row.YTUpload = ytState.Label()
		row.YTReady = ytState.Ready()
		latest, _ := s.Growth.LatestSnapshot(a.ID)
		if hasYouTube(a) {
			row.YPP = growth.YPPProgress(latest)
		}
		if latest != nil {
			row.HasSnapshot = true
			row.FollowersLabel = growthFollowersLabel(latest.Source)
			if latest.Followers != nil {
				row.Followers = *latest.Followers
				switch latest.Source {
				case "youtube_api":
					totalYTSubscribers += *latest.Followers
				case "tiktok_api":
					totalTikTokFollowers += *latest.Followers
				}
			}
			if v, ok := metricValue(latest, "views_total"); ok {
				row.ViewsTotal = int64(v)
				totalViews += int64(v)
			}
			if targets, err := s.Growth.ListTargets(a.ID); err == nil {
				for _, t := range targets {
					if t.ReachedAt != "" {
						continue
					}
					row.NextLabel = t.Label
					row.NextMetric = t.Metric
					row.NextTarget = t.Threshold
					if cur, ok := metricValue(latest, t.Metric); ok {
						row.NextValue = cur
					}
					if t.Threshold > 0 {
						row.NextPct = row.NextValue / t.Threshold * 100
						if row.NextPct > 100 {
							row.NextPct = 100
						}
					}
					row.HasNext = true
					break
				}
			}
		}
		rows = append(rows, row)

		if al, err := s.Growth.ListAlerts(a.ID, 5); err == nil {
			for _, x := range al {
				alerts = append(alerts, growthAlertRow{Alert: x, Username: a.Username})
			}
		}
		if items, err := s.Growth.ActivePlanItems(a.ID); err == nil {
			for _, it := range items {
				switch it.Status {
				case growth.ItemPlanned, growth.ItemProducing, growth.ItemProduced, growth.ItemPublishing,
					growth.ItemWaitingConnect, growth.ItemWaitingQuota, growth.ItemNeedsReview:
					if it.Status != growth.ItemPlanned || it.PlannedFor >= today {
						upcoming = append(upcoming, growthItemRow{PlanItem: it, Username: a.Username})
					}
				}
			}
		}
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].ID > alerts[j].ID })
	if len(alerts) > 15 {
		alerts = alerts[:15]
	}
	sort.Slice(upcoming, func(i, j int) bool {
		if upcoming[i].PlannedFor != upcoming[j].PlannedFor {
			return upcoming[i].PlannedFor < upcoming[j].PlannedFor
		}
		return upcoming[i].Username < upcoming[j].Username
	})
	if len(upcoming) > 20 {
		upcoming = upcoming[:20]
	}
	ctx["Rows"] = rows
	ctx["Alerts"] = alerts
	ctx["Upcoming"] = upcoming
	ctx["TotalYTSubscribers"] = totalYTSubscribers
	ctx["TotalTikTokFollowers"] = totalTikTokFollowers
	ctx["TotalViews"] = totalViews
	ctx["HasYTKey"] = ytKey
	// Phase-2 production state: toggle, quota today, upload privacy.
	ctx["ProdEnabled"] = s.GrowthProductionEnabled()
	ctx["ProdDryRun"] = s.Cfg != nil && s.Cfg.DryRun()
	// Đợt AA: số job đang chờ/đang chạy trong Studio — user biết hàng đợi dài bao nhiêu.
	ctx["StudioPending"] = 0
	if s.Studio != nil {
		ctx["StudioPending"] = s.Studio.PendingJobCount()
	}
	quotaDay := growth.YouTubeQuotaDay(time.Now())
	if used, err := s.Growth.QuotaUsed(quotaDay); err == nil {
		ctx["QuotaUsed"] = used
		ctx["QuotaLeft"] = growth.QuotaUploadsLeft(used)
		ctx["QuotaReady"] = true
	}
	ctx["QuotaLimit"] = growth.DailyQuotaUnits
	ctx["QuotaPerUpload"] = growth.UploadCostUnits
	if priv, _ := s.effectiveEnv("YOUTUBE_DEFAULT_PRIVACY"); priv != "" {
		ctx["YTPrivacy"] = priv
	} else {
		ctx["YTPrivacy"] = "private"
	}
	// Chỉ hiện bảng tiến độ YPP khi có ít nhất 1 kênh YouTube.
	hasYT := false
	for i := range accounts {
		if hasYouTube(accounts[i]) {
			hasYT = true
			break
		}
	}
	ctx["HasYTAccount"] = hasYT
	ctx["Today"] = today
	return ctx
}

// recentGrowthAlerts merges the newest growth alerts across accounts
// for the homepage (Đợt 3 / A10): what the system already decided is
// visible without hunting through /growth.
func (s *Server) recentGrowthAlerts(accounts []*network.Account) []growthAlertRow {
	if s.Growth == nil {
		return nil
	}
	var out []growthAlertRow
	for _, a := range accounts {
		als, err := s.Growth.ListAlerts(a.ID, 3)
		if err != nil {
			continue
		}
		for _, al := range als {
			out = append(out, growthAlertRow{Alert: al, Username: a.Username})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func (s *Server) handleGrowth(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	for k, v := range s.growthView() {
		data[k] = v
	}
	s.render(w, "growth", data)
}

// ------------------------------------------------------- sync + decide

func (s *Server) handleGrowthSync(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Mgr.List()
	if err != nil {
		s.render(w, "growth", s.ctx("Error", err.Error()))
		return
	}
	var notes []string
	for _, a := range accounts {
		notes = append(notes, s.syncOneAccount(r.Context(), a)...)
	}
	data := s.ctx()
	for k, v := range s.growthView() {
		data[k] = v
	}
	data["SyncNotes"] = notes
	s.render(w, "growth", data)
}

// ------------------------------------------------------------- plans

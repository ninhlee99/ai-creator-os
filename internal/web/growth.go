package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"path/filepath"
	"sort"
	"time"
)

// ------------------------------------------------------------- /growth

// growthAccountRow is one account's growth state on the overview page.
type growthAccountRow struct {
	Account               network.Account
	Stage                 string
	NextLabel             string
	NextMetric            string
	NextValue             float64
	NextTarget            float64
	NextPct               float64
	HasNext               bool
	HasSnapshot           bool
	Followers             int64
	FollowersLabel        string
	ViewsTotal            int64
	YTAnalyticsConfigured bool
	YTUpload              string // YouTube upload readiness label (phase 2)
	YTReady               bool
	YPP                   []growth.YPPTierProgress // chỉ kênh YouTube
}

type growthAlertRow struct {
	growth.Alert
	Username string
}

type growthItemRow struct {
	growth.PlanItem
	Username        string
	OutputFile      string
	DashboardStatus string
}

func (s *Server) growthMediaOutput(jobID string) string {
	if s.Studio == nil || jobID == "" {
		return ""
	}
	job, ok := s.Studio.GetJob(jobID)
	if !ok || job.Status != studio.StatusDone || studioOutputKind(job.Output) != "video" {
		return ""
	}
	name := mediaName(job.Output)
	if !mediaNameRe.MatchString(name) || !fileExists(filepath.Join(s.OutDir, name)) {
		return ""
	}
	return name
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
	today := s.today()
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
		row := growthAccountRow{Account: *a, Stage: prof.Stage, YTAnalyticsConfigured: ytKey && a.YoutubeChannel != ""}
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
				case growth.ItemPlanned, growth.ItemEnqueueing, growth.ItemProducing, growth.ItemProduced, growth.ItemPublishing,
					growth.ItemWaitingConnect, growth.ItemWaitingQuota, growth.ItemNeedsReview:
					if it.Status != growth.ItemPlanned || it.PlannedFor >= today {
						upcoming = append(upcoming, growthItemRow{
							PlanItem: it, Username: a.Username,
							OutputFile: s.growthMediaOutput(it.StudioJobID),
						})
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

// recentGrowthAlerts returns the newest persisted growth alerts for the
// homepage, retaining network-wide alerts and their account labels.
func (s *Server) recentGrowthAlerts(accounts []*network.Account) ([]growthAlertRow, error) {
	if s.Growth == nil {
		return nil, nil
	}
	alerts, err := s.Growth.ListAlerts(0, 6)
	if err != nil {
		return nil, err
	}
	usernames := make(map[int64]string, len(accounts))
	for _, a := range accounts {
		usernames[a.ID] = a.Username
	}
	out := make([]growthAlertRow, 0, len(alerts))
	for _, alert := range alerts {
		username := usernames[alert.AccountID]
		if alert.AccountID == 0 {
			username = "Toàn mạng"
		} else if username == "" {
			username = "Kênh không còn trong danh sách"
		}
		out = append(out, growthAlertRow{Alert: alert, Username: username})
	}
	return out, nil
}

// dashboardGrowthState returns the compact queue and every current Growth
// item that needs a publish/configuration/recovery action. Future plans stay
// on /growth's schedule and do not appear as blockers here.
func (s *Server) dashboardGrowthState(accounts []*network.Account) ([]growthItemRow, []growthItemRow, error) {
	if s.Growth == nil {
		return nil, nil, nil
	}
	today := s.today()
	var queue, attention []growthItemRow
	for _, account := range accounts {
		items, err := s.Growth.ActivePlanItems(account.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			displayStatus := statusLabel(item.Status)
			isOverdue := false
			switch item.Status {
			case growth.ItemEnqueueing, growth.ItemProducing, growth.ItemQC, growth.ItemQueued, growth.ItemProduced, growth.ItemPublishing,
				growth.ItemWaitingConnect, growth.ItemWaitingQuota, growth.ItemNeedsReview, growth.ItemFailed:
			case growth.ItemPlanned:
				if item.PlannedFor == "" || item.PlannedFor >= today {
					continue
				}
				displayStatus = "Quá lịch"
				isOverdue = true
			default:
				continue
			}
			row := growthItemRow{PlanItem: item, Username: account.Username, DashboardStatus: displayStatus}
			queue = append(queue, row)
			if isOverdue || item.Status == growth.ItemProduced || item.Status == growth.ItemWaitingConnect ||
				item.Status == growth.ItemWaitingQuota || item.Status == growth.ItemNeedsReview || item.Status == growth.ItemFailed {
				attention = append(attention, row)
			}
		}
	}
	sortRows := func(rows []growthItemRow) {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].PlannedFor != rows[j].PlannedFor {
				return rows[i].PlannedFor < rows[j].PlannedFor
			}
			if rows[i].Username != rows[j].Username {
				return rows[i].Username < rows[j].Username
			}
			return rows[i].ID < rows[j].ID
		})
	}
	sortRows(queue)
	sortRows(attention)
	if len(queue) > 6 {
		queue = queue[:6]
	}
	return queue, attention, nil
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

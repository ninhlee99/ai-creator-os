package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ------------------------------------------------------------- /growth

// growthAccountRow is one account's TikTok growth state.
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
	FollowersKnown bool
	FollowerTrend  string
	FollowerAdvice string
	VideoCount     int64
	VideosKnown    bool
	FollowersLabel string
	Views30d       float64
}

type growthVideoMetricRow struct {
	Username    string
	VideoID     string
	URL         string
	Title       string
	PublishedAt string
	TakenAt     string
	Views       string
	Likes       string
	Comments    string
	Shares      string
}

type growthStudioVideoRow struct {
	accountID       int64
	viewsValue      int64
	engagement      float64
	engagementKnown bool
	Username        string
	JobID           string
	Title           string
	KindLabel       string
	VideoID         string
	URL             string
	Views           string
	Likes           string
	Comments        string
	Shares          string
	TakenAt         string
	HasMetrics      bool
	AttributionID   string
	Orders          accesstrade.OrderAttributionSummary
	OrdersKnown     bool
	OrdersSyncedAt  string
	EngagementRate  string
	EngagementBase  string
	OrderPerView    string
	Recommendation  string
	SalesAdvice     string
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

func metricValue(snap *growth.Snapshot, metric string) (float64, bool) {
	if snap == nil {
		return 0, false
	}
	switch metric {
	case "followers":
		if snap.Followers != nil {
			return float64(*snap.Followers), true
		}
	case "views_30d":
		if snap.Views30d != nil {
			return *snap.Views30d, true
		}
	}
	return 0, false
}

// growthView builds all data for the /growth page.
func (s *Server) growthView() map[string]any {
	ctx := map[string]any{}
	accounts, err := s.Mgr.List()
	if err != nil {
		ctx["Error"] = "Không đọc được danh sách tài khoản: " + err.Error()
		return ctx
	}
	ctx["Accounts"] = accounts
	ctx["ProdEnabled"] = s.GrowthProductionEnabled()
	ctx["ProdDryRun"] = s.Cfg != nil && s.Cfg.DryRun()
	ctx["Thresholds"] = automation.LoadThresholds(s.settings())
	if s.Growth == nil {
		ctx["Error"] = "Module Phát triển kênh chưa khởi tạo được (xem log máy chủ)."
		return ctx
	}

	today := s.today()
	var rows []growthAccountRow
	var alerts []growthAlertRow
	var upcoming []growthItemRow
	var videoMetrics []growthVideoMetricRow
	var studioVideoMetrics []growthStudioVideoRow
	var totalFollowers int64
	var totalViews30d float64
	allViews30dKnown := len(accounts) > 0
	hasTikTokMetrics := false
	atOrdersSyncedAt := ""
	atOrdersSynced := false
	if s.AT != nil {
		atOrdersSyncedAt, atOrdersSynced = s.AT.GetSyncState("orders.until")
	}
	for _, account := range accounts {
		profile, err := s.Growth.EnsureProfile(account.ID)
		if err != nil || profile == nil {
			continue
		}
		row := growthAccountRow{Account: *account, Stage: profile.Stage}
		latest, _ := s.Growth.LatestSnapshot(account.ID)
		if latest != nil && latest.Source == "tiktok_api" {
			row.FollowersLabel = "TikTok followers"
			if series, err := s.Growth.SnapshotSeries(account.ID, 500); err == nil {
				row.FollowerTrend, row.FollowerAdvice = followerTrendAdvice(series, latest, time.Now())
			}
			if latest.Followers != nil {
				row.FollowersKnown = true
				row.Followers = *latest.Followers
				totalFollowers += *latest.Followers
			}
			if latest.Videos != nil {
				row.VideosKnown = true
				row.VideoCount = *latest.Videos
			}
			if latest.Views30d != nil {
				row.Views30d = *latest.Views30d
				totalViews30d += *latest.Views30d
			} else {
				allViews30dKnown = false
			}
			row.HasSnapshot = row.FollowersKnown || row.VideosKnown || latest.Views30d != nil
			hasTikTokMetrics = hasTikTokMetrics || row.HasSnapshot
			if targets, err := s.Growth.ListTargets(account.ID); err == nil {
				for _, target := range targets {
					if target.ReachedAt != "" {
						continue
					}
					row.NextLabel, row.NextMetric, row.NextTarget = target.Label, target.Metric, target.Threshold
					if current, ok := metricValue(latest, target.Metric); ok {
						row.NextValue = current
					}
					if target.Threshold > 0 {
						row.NextPct = row.NextValue / target.Threshold * 100
						if row.NextPct > 100 {
							row.NextPct = 100
						}
					}
					row.HasNext = true
					break
				}
			}
		} else {
			allViews30dKnown = false
		}
		rows = append(rows, row)

		if metrics, err := s.Growth.LatestVideoMetricRows(account.ID, 30); err == nil {
			for _, metric := range metrics {
				videoURL := ""
				if studio.IsTikTokVideoURL(metric.ShareURL) {
					videoURL = metric.ShareURL
				} else if numericTikTokVideoID(metric.PlatformVideoID) {
					videoURL = "https://www.tiktok.com/video/" + metric.PlatformVideoID
				}
				title := strings.TrimSpace(metric.Title)
				if title == "" {
					title = "Video TikTok " + metric.PlatformVideoID
				}
				videoMetrics = append(videoMetrics, growthVideoMetricRow{
					Username: account.Username, VideoID: metric.PlatformVideoID, URL: videoURL, Title: title,
					PublishedAt: shortMetricTime(metric.PublishedAt), TakenAt: shortMetricTime(metric.TakenAt),
					Views: metricLabel(metric.Views), Likes: metricLabel(metric.Likes),
					Comments: metricLabel(metric.Comments), Shares: metricLabel(metric.Shares),
				})
			}
		}
		if s.Studio != nil {
			if publications, err := s.Studio.TikTokPublications(account.ID, 100); err == nil {
				for _, publication := range publications {
					row := growthStudioVideoRow{
						accountID: account.ID,
						Username:  account.Username, JobID: publication.JobID, Title: publication.Title,
						KindLabel: "TikTok Viral", VideoID: publication.PlatformVideoID, URL: publication.VideoURL,
						Views: "—", Likes: "—", Comments: "—", Shares: "—", TakenAt: "—",
					}
					if publication.Kind == studio.KindAffiliate {
						row.KindLabel = "Affiliate TikTok"
						if job, ok := s.Studio.GetJob(publication.JobID); ok {
							var params studio.AffiliateParams
							if json.Unmarshal([]byte(job.Params), &params) == nil {
								row.AttributionID = strings.TrimSpace(params.AffiliateAttributionID)
							}
						}
						if row.AttributionID != "" && s.AT != nil && atOrdersSynced {
							if orders, err := s.AT.OrderAttributionBySub2(row.AttributionID); err == nil {
								row.Orders = orders
								row.OrdersKnown = true
								row.OrdersSyncedAt = shortMetricTimeString(atOrdersSyncedAt)
							}
						}
					}
					if metric, found, err := s.Growth.LatestVideoMetric(account.ID, publication.PlatformVideoID); err == nil && found {
						row.Views, row.Likes = metricLabel(metric.Views), metricLabel(metric.Likes)
						row.Comments, row.Shares = metricLabel(metric.Comments), metricLabel(metric.Shares)
						row.TakenAt, row.HasMetrics = shortMetricTime(metric.TakenAt), true
						if metric.Views != nil {
							row.viewsValue = *metric.Views
						}
						if metric.Views != nil && *metric.Views > 0 && metric.Likes != nil && metric.Comments != nil && metric.Shares != nil {
							row.engagement = (float64(*metric.Likes) + float64(*metric.Comments) + float64(*metric.Shares)) / float64(*metric.Views)
							row.engagementKnown = true
							row.EngagementRate = fmt.Sprintf("%.1f%%", row.engagement*100)
						}
					}
					studioVideoMetrics = append(studioVideoMetrics, row)
				}
			}
		}

		if accountAlerts, err := s.Growth.ListAlerts(account.ID, 5); err == nil {
			for _, alert := range accountAlerts {
				alerts = append(alerts, growthAlertRow{Alert: alert, Username: account.Username})
			}
		}
		if items, err := s.Growth.ActivePlanItems(account.ID); err == nil {
			for _, item := range items {
				switch item.Status {
				case growth.ItemPlanned, growth.ItemEnqueueing, growth.ItemProducing, growth.ItemProduced, growth.ItemFailed:
					if item.Status != growth.ItemPlanned || item.PlannedFor >= today {
						upcoming = append(upcoming, growthItemRow{PlanItem: item, Username: account.Username,
							OutputFile: s.growthMediaOutput(item.StudioJobID)})
					}
				}
			}
		}
	}
	annotateStudioVideoPerformance(studioVideoMetrics)
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
	ctx["Rows"], ctx["Alerts"], ctx["Upcoming"] = rows, alerts, upcoming
	ctx["TotalTikTokFollowers"] = totalFollowers
	ctx["TotalViews30d"] = totalViews30d
	ctx["TikTokMetricsAvailable"] = hasTikTokMetrics
	ctx["Views30dKnown"] = allViews30dKnown
	ctx["VideoMetrics"] = videoMetrics
	ctx["StudioVideoMetrics"] = studioVideoMetrics
	ctx["StudioPending"] = 0
	if s.Studio != nil {
		ctx["StudioPending"] = s.Studio.PendingJobCount()
	}
	ctx["Today"] = today
	return ctx
}

// followerTrendAdvice compares fresh TikTok follower snapshots at least a
// day apart. It stays unknown when history is missing or stale.
func followerTrendAdvice(series []growth.Snapshot, latest *growth.Snapshot, now time.Time) (string, string) {
	if latest == nil || latest.Source != "tiktok_api" || latest.Followers == nil {
		return "Chưa có snapshot follower đủ dữ liệu", ""
	}
	currentAge := now.Sub(latest.TakenAt)
	if currentAge < -5*time.Minute || currentAge > 14*24*time.Hour {
		return "Snapshot follower đã cũ; đồng bộ lại", ""
	}
	for i := len(series) - 1; i >= 0; i-- {
		previous := series[i]
		if previous.Source != "tiktok_api" || previous.Followers == nil || !previous.TakenAt.Before(latest.TakenAt) {
			continue
		}
		span := latest.TakenAt.Sub(previous.TakenAt)
		if span < 24*time.Hour {
			continue
		}
		if span > 14*24*time.Hour {
			break
		}
		delta := *latest.Followers - *previous.Followers
		period := followerTrendPeriod(span)
		label := fmt.Sprintf("%+d trong %s · từ %s", delta, period, shortMetricTime(previous.TakenAt))
		switch {
		case delta < 0:
			return label, "Followers giảm trong snapshot; thử CTA theo dõi rõ và nội dung dạng series, rồi đo lại."
		case delta == 0:
			return label, "Chưa tăng followers trong khoảng đo; thử CTA theo dõi rõ và nội dung dạng series. Chưa có retention để kết luận nguyên nhân."
		default:
			return label, "Followers đang tăng; giữ hướng hiện tại và chỉ đổi một yếu tố mỗi video để so sánh."
		}
	}
	return "Chưa đủ 2 snapshot cách nhau ít nhất 24 giờ", ""
}

func followerTrendPeriod(span time.Duration) string {
	totalHours := int(span / time.Hour)
	days, hours := totalHours/24, totalHours%24
	if hours == 0 {
		return fmt.Sprintf("%d ngày", days)
	}
	if days == 0 {
		return fmt.Sprintf("%d giờ", totalHours)
	}
	return fmt.Sprintf("%d ngày %d giờ", days, hours)
}

// annotateStudioVideoPerformance compares each linked post with other linked
// Studio posts of the same flow on the same channel. It reports only observed
// counters and synced AccessTrade orders; it does not infer watch time or age.
func annotateStudioVideoPerformance(rows []growthStudioVideoRow) {
	type baseline struct {
		sum   float64
		count int
	}
	keyFor := func(row growthStudioVideoRow) string {
		return fmt.Sprintf("%d:%s", row.accountID, row.KindLabel)
	}
	baselines := map[string]baseline{}
	for _, row := range rows {
		if !row.engagementKnown {
			continue
		}
		key := keyFor(row)
		value := baselines[key]
		value.sum += row.engagement
		value.count++
		baselines[key] = value
	}
	for i := range rows {
		row := &rows[i]
		if row.engagementKnown {
			group := baselines[keyFor(*row)]
			peerCount := group.count - 1
			if peerCount > 0 {
				average := (group.sum - row.engagement) / float64(peerCount)
				row.EngagementBase = fmt.Sprintf("%.1f%% · n=%d", average*100, peerCount)
				if peerCount < 3 {
					row.Recommendation = fmt.Sprintf("Engagement %.1f%%; baseline mới có %d video cùng flow trên kênh, mẫu nhỏ nên chưa kết luận cao/thấp. Thử từng thay đổi và đo thêm.", row.engagement*100, peerCount)
				} else if row.engagement < average {
					row.Recommendation = fmt.Sprintf("Engagement %.1f%% thấp hơn baseline %.1f%% của flow này trên kênh; thử hook vào hành động sớm hơn, bỏ intro và giữ phần được người xem phản hồi.", row.engagement*100, average*100)
				} else {
					row.Recommendation = fmt.Sprintf("Engagement %.1f%% bằng hoặc cao hơn baseline %.1f%%; giữ format đang hiệu quả, thử đổi từng yếu tố ở video kế tiếp.", row.engagement*100, average*100)
				}
			} else {
				row.Recommendation = "Chưa có video cùng flow trên kênh để lập baseline; giữ snapshot này làm mốc so sánh."
			}
		} else if row.HasMetrics {
			row.Recommendation = "TikTok chưa trả đủ views, likes, comments, shares để tính engagement; chưa kết luận hiệu quả video."
		} else {
			row.Recommendation = "Chưa có counters TikTok cho video; đồng bộ lại sau khi đăng."
		}
		if row.KindLabel != "Affiliate TikTok" {
			continue
		}
		if !row.OrdersKnown {
			row.SalesAdvice = "Chưa có lần đồng bộ đơn AccessTrade; chưa kết luận chuyển đổi."
			continue
		}
		if row.viewsValue > 0 {
			row.OrderPerView = fmt.Sprintf("%.3f%%", float64(row.Orders.TotalCount)/float64(row.viewsValue)*100)
		}
		switch {
		case row.Orders.ApprovedCount > 0:
			row.SalesAdvice = fmt.Sprintf("Có %d đơn đã duyệt; theo dõi tiếp hoa hồng và giữ lại offer/CTA của video.", row.Orders.ApprovedCount)
		case row.Orders.PendingCount > 0:
			row.SalesAdvice = "Có đơn đang chờ duyệt; đợi kết quả trước khi đổi offer."
		case row.Orders.RejectedCount > 0:
			row.SalesAdvice = "Đơn đã bị từ chối; kiểm tra trạng thái offer và điều kiện ghi nhận AccessTrade."
		default:
			row.SalesAdvice = "Chưa ghi nhận đơn theo mã video; kiểm tra tracking, giá và CTA. Chưa có dữ liệu click để tách nguyên nhân."
		}
	}
}

func metricLabel(value *int64) string {
	if value == nil {
		return "—"
	}
	return strconv.FormatInt(*value, 10)
}

func shortMetricTime(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Local().Format("02/01/2006 15:04")
}

func shortMetricTimeString(value string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return "—"
	}
	return shortMetricTime(parsed)
}

func numericTikTokVideoID(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
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
			case growth.ItemEnqueueing, growth.ItemProducing, growth.ItemQC, growth.ItemQueued, growth.ItemProduced, growth.ItemFailed:
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
			if isOverdue || item.Status == growth.ItemProduced || item.Status == growth.ItemFailed {
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
		if r.Header.Get("HX-Request") == "true" {
			s.renderGrowthSyncResult(w, r, s.ctx("SyncError", err.Error()))
			return
		}
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
	if r.Header.Get("HX-Request") == "true" {
		s.renderGrowthSyncResult(w, r, data)
		return
	}
	s.render(w, "growth", data)
}

func (s *Server) renderGrowthSyncResult(w http.ResponseWriter, r *http.Request, data map[string]any) {
	t := s.templates["growth"]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "growth-sync-result", data); err != nil {
		http.Error(w, "Không hiển thị được kết quả đồng bộ", http.StatusInternalServerError)
	}
}

// ------------------------------------------------------------- plans

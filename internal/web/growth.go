package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
)

// ------------------------------------------------------------- /growth

// growthAccountRow is one account's growth state on the overview page.
type growthAccountRow struct {
	Account     network.Account
	Stage       string
	NextLabel   string
	NextMetric  string
	NextValue   float64
	NextTarget  float64
	NextPct     float64
	HasNext     bool
	HasSnapshot bool
	Followers   int64
	ViewsTotal  int64
	YTConnected bool
	YTUpload    string // YouTube upload readiness label (phase 2)
	YTReady     bool
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
	ctx["QuotaUsed"] = 0
	ctx["QuotaLeft"] = growth.QuotaUploadsLeft(0)
	ctx["QuotaLimit"] = growth.DailyQuotaUnits
	ctx["QuotaPerUpload"] = growth.UploadCostUnits
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
	var totalFollowers, totalViews int64
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
		if latest != nil {
			row.HasSnapshot = true
			if latest.Followers != nil {
				row.Followers = *latest.Followers
				totalFollowers += *latest.Followers
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
				case growth.ItemPlanned, growth.ItemProducing, growth.ItemProduced,
					growth.ItemWaitingConnect, growth.ItemWaitingQuota:
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
	ctx["TotalFollowers"] = totalFollowers
	ctx["TotalViews"] = totalViews
	ctx["HasYTKey"] = ytKey
	// Phase-2 production state: toggle, quota today, upload privacy.
	ctx["ProdEnabled"] = s.GrowthProductionEnabled()
	ctx["ProdDryRun"] = s.Cfg != nil && s.Cfg.DryRun()
	if used, err := s.Growth.QuotaUsed(today); err == nil {
		ctx["QuotaUsed"] = used
		ctx["QuotaLeft"] = growth.QuotaUploadsLeft(used)
	}
	ctx["QuotaLimit"] = growth.DailyQuotaUnits
	ctx["QuotaPerUpload"] = growth.UploadCostUnits
	if priv, _ := s.effectiveEnv("YOUTUBE_DEFAULT_PRIVACY"); priv != "" {
		ctx["YTPrivacy"] = priv
	} else {
		ctx["YTPrivacy"] = "private"
	}
	deadline, _ := time.Parse("2006-01-02", growth.YPPDeadline)
	ctx["YPPDeadline"] = growth.YPPDeadline
	ctx["YPPDaysLeft"] = int(time.Until(deadline).Hours() / 24)
	ctx["Today"] = today
	return ctx
}

func (s *Server) handleGrowth(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	for k, v := range s.growthView() {
		data[k] = v
	}
	s.render(w, "growth", data)
}

// ------------------------------------------------------- sync + decide

// syncOneAccount pulls fresh metrics for one account from every wired
// source, persists what came back, and runs the growth decision loop.
// It returns honest status lines (Vietnamese) for the UI — a source that
// is not connected is reported as such, never silently skipped.
func (s *Server) syncOneAccount(ctx context.Context, a *network.Account) []string {
	if s.Growth == nil {
		return []string{a.Username + ": module growth chưa sẵn sàng"}
	}
	var notes []string
	if _, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err != nil {
		return append(notes, a.Username+": "+err.Error())
	}
	srcAcct := growth.SourceAccount{ID: a.ID, Username: a.Username, YoutubeChannel: a.YoutubeChannel}
	ytKeyVal, _ := s.effectiveEnv("YOUTUBE_API_KEY")
	sources := []growth.MetricsSource{
		growth.NewYouTubeSource(ytKeyVal),
		growth.TikTokSource{},
	}
	gotSnapshot := false
	for _, src := range sources {
		snap, err := growth.RecordSnapshot(ctx, s.Growth, src, srcAcct)
		if err != nil {
			notes = append(notes, a.Username+": "+err.Error())
			continue
		}
		gotSnapshot = true
		notes = append(notes, fmt.Sprintf("%s: đã đồng bộ số liệu từ %s", a.Username, src.Name()))
		if snap.Followers != nil {
			_ = s.Ledger.SetFollowers(a.ID, *snap.Followers)
			cur := map[string]float64{"followers": float64(*snap.Followers), "subs": float64(*snap.Followers)}
			if reached, err := s.Growth.MarkTargetsReached(a.ID, cur); err == nil {
				for _, label := range reached {
					target := a.Username
					_ = s.Ledger.Decide("growth_analyst", "milestone_reached", &target,
						"Đạt mốc "+label, nil)
					notes = append(notes, a.Username+": đạt mốc "+label)
				}
			}
		}
	}
	if !gotSnapshot {
		return notes // nothing real came back; decide nothing on stale air
	}
	res, err := s.Growth.Evaluate(a.ID, a.Username, growth.DefaultConfig(), time.Now())
	if err != nil {
		return append(notes, a.Username+": lỗi đánh giá growth — "+err.Error())
	}
	for _, act := range res.Actions {
		notes = append(notes, a.Username+": "+act)
	}
	if res.PauseAccount {
		if _, err := s.Mgr.Transition(a.ID, "paused", map[string]any{
			"reason": "growth: " + res.PauseReason,
		}); err != nil {
			log.Printf("web: growth auto-pause %s: %v", a.Username, err)
		} else {
			notes = append(notes, a.Username+": ĐÃ TỰ TẠM DỪNG — "+res.PauseReason)
		}
	}
	return notes
}

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

func pillarsFor(a *network.Account) []string {
	if a.Persona != "" {
		if p, ok := network.PERSONAS[a.Persona]; ok && len(p.ContentPillars) > 0 {
			return p.ContentPillars
		}
	}
	if a.Niche != "" {
		return []string{a.Niche}
	}
	return []string{"video ngắn mỗi ngày"}
}

// generatePlanFor creates (or replaces) the account's active 30-day plan
// from its current growth stage. Returns the number of scheduled items.
func (s *Server) generatePlanFor(a *network.Account, rationale string) (int, error) {
	prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a))
	if err != nil {
		return 0, err
	}
	drafts := growth.GeneratePlan(growth.PlanRequest{
		Stage:      prof.Stage,
		HasYouTube: hasYouTube(a),
		Pillars:    pillarsFor(a),
		Start:      time.Now(),
		Days:       30,
	})
	if _, err := s.Growth.InsertPlan(a.ID, "d30", rationale, drafts); err != nil {
		return 0, err
	}
	target := a.Username
	_ = s.Ledger.Decide("growth_director", "generate_plan", &target,
		fmt.Sprintf("Sinh kế hoạch 30 ngày (giai đoạn %s, %d vị trí)", prof.Stage, len(drafts)),
		map[string]any{"stage": prof.Stage, "items": len(drafts)})
	return len(drafts), nil
}

func (s *Server) handleGrowthPlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.Mgr.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base := "/accounts/" + strconv.FormatInt(id, 10)
	if s.Growth == nil {
		seeOther(w, r, base+"?err="+url.QueryEscape("Module Phát triển kênh chưa sẵn sàng."))
		return
	}
	n, err := s.generatePlanFor(a, "Sinh từ giao diện")
	if err != nil {
		seeOther(w, r, base+"?err="+url.QueryEscape("Không sinh được kế hoạch: "+err.Error()))
		return
	}
	seeOther(w, r, base+"?ok="+url.QueryEscape(fmt.Sprintf("Đã sinh kế hoạch 30 ngày: %d vị trí nội dung.", n)))
}

// growthNum renders a metric value with Vietnamese dot thousands separators.
func growthNum(f float64) string {
	n := int64(f)
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// growthTargetView is one ladder milestone with its live progress.
type growthTargetView struct {
	growth.Target
	Value    float64
	Pct      float64
	HasValue bool
}

// accountGrowthView assembles the growth block for the account detail page.
func (s *Server) accountGrowthView(a *network.Account) map[string]any {
	out := map[string]any{}
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
	// Followers on the detail page come from the latest synced snapshot
	// only — there is no manual entry anymore (kills the fake-number
	// door); no snapshot yet -> the page says "chưa kết nối".
	if v, ok := metricValue(latest, "followers"); ok {
		out["GrowthFollowers"] = int64(v)
		out["GrowthHasFollowers"] = true
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

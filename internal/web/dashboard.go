package web

// Dashboard overview page (GET /): active jobs, synced Accesstrade orders,
// automation controls, and worker status.

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
	"github.com/ninhlee99/ai-creator-os/internal/sysmon"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		http.NotFound(w, r)
		return
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	atOrders, err := s.atOrderSummary()
	if err != nil {
		log.Printf("web: dashboard Accesstrade order stats: %v", err)
	}
	recentDecisions, err := s.recentDashboardDecisions(6)
	if err != nil {
		s.fail(w, err, "load recent system decisions")
		return
	}
	recentGrowthAlerts, err := s.recentGrowthAlerts(accounts)
	if err != nil {
		s.fail(w, err, "load recent Growth alerts")
		return
	}
	growthQueue, growthAttention, err := s.dashboardGrowthState(accounts)
	if err != nil {
		s.fail(w, err, "load current Growth items")
		return
	}
	jobsKnown := s.Studio != nil
	activeJobs := 0
	if jobsKnown {
		activeJobs, err = s.Studio.CountActiveJobs()
		if err != nil {
			s.fail(w, err, "count active Studio jobs")
			return
		}
	}
	var recentJobs []studioJobView
	var recentPublishLegs []studio.PublishLeg
	var failedStudioJobs []studioJobView
	var failedPublishLegs []studio.PublishLeg
	if s.Studio != nil {
		recentJobs = s.studioJobViews(s.Studio.ListJobs(6))
		recentPublishLegs, err = s.Studio.RecentPublishLegs(6)
		if err != nil {
			s.fail(w, err, "load recent publish outcomes")
			return
		}
		failedJobs, err := s.Studio.RecentFailedJobs(10)
		if err != nil {
			s.fail(w, err, "load failed Studio jobs")
			return
		}
		failedStudioJobs = s.studioJobViews(failedJobs)
		failedPublishLegs, err = s.Studio.RecentPublishLegIssues(10)
		if err != nil {
			s.fail(w, err, "load failed automatic publish legs")
			return
		}
	}
	recentPublishOutcomes := s.recentPublishOutcomes(recentPublishLegs, 6)
	tickBeats := s.tickBeatViews()
	hasTickIssue := false
	for _, beat := range tickBeats {
		if beat.Stale || beat.LastErr != "" {
			hasTickIssue = true
			break
		}
	}
	quotaCircuit := s.quotaCircuitView()
	videoPerformance := s.dashboardVideoPerformance(r.Context(), accounts)
	attentionItems := s.dashboardAttention(growthAttention, failedStudioJobs, failedPublishLegs,
		tickBeats, quotaCircuit)
	attentionSourcesKnown := jobsKnown || s.Growth != nil || s.TickBeats != nil || quotaCircuit.Known
	if len(attentionItems) > 10 {
		attentionItems = attentionItems[:10]
	}
	activeWorkflowNodes := s.collectActiveWorkflowNodes()
	sysSnapshot := sysmon.Collect(activeWorkflowNodes)

	s.render(w, "dashboard", s.ctx(
		"Notice", r.URL.Query().Get("ok"),
		"Warning", r.URL.Query().Get("warn"),
		"Error", r.URL.Query().Get("err"),
		"Metrics", sysSnapshot,
		"Accounts", accounts,
		"ATOrders", atOrders,
		"RecentDecisions", recentDecisions,
		"GrowthKnown", s.Growth != nil,
		"RecentGrowthAlerts", recentGrowthAlerts,
		"GrowthQueue", growthQueue,
		"JobsKnown", jobsKnown,
		"AutoSwitches", s.autoSwitches(),
		"MoneyReadiness", s.moneyReadiness(),
		"TickBeats", tickBeats,
		"HasTickIssue", hasTickIssue,
		"QuotaCircuit", quotaCircuit,
		"VideoPerformance", videoPerformance,
		"AttentionItems", attentionItems,
		"AttentionSourcesKnown", attentionSourcesKnown,
		"NJobs", activeJobs,
		"RecentJobs", recentJobs,
		"RecentPublishOutcomes", recentPublishOutcomes,
	))
}

// collectActiveWorkflowNodes tìm kiếm các node workflow đang chạy thực tế trong hệ thống.
func (s *Server) collectActiveWorkflowNodes() []sysmon.ActiveNodeInfo {
	if s.Studio == nil {
		return nil
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil || fStore == nil {
		return nil
	}
	runs, err := fStore.ListRuns(10)
	if err != nil {
		return nil
	}

	var active []sysmon.ActiveNodeInfo
	for _, run := range runs {
		if run.Status != "running" {
			continue
		}
		var graph *flow.FlowGraph
		if run.GraphID != "" {
			graph, _ = fStore.GetGraph(run.GraphID)
		}

		for nodeID, nState := range run.NodeStates {
			if nState.Status != "running" {
				continue
			}

			nodeTitle := nodeID
			nodeType := ""
			category := ""
			if graph != nil {
				for _, n := range graph.Nodes {
					if n.ID == nodeID {
						nodeTitle = n.Title
						if nodeTitle == "" {
							nodeTitle = n.Type
						}
						nodeType = n.Type
						category = string(n.Category)
						break
					}
				}
			}
			if nodeType == "" {
				nodeType = nodeID
			}

			var elapsedSec int64
			if nState.StartedAt != "" {
				for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
					if t, err := time.Parse(layout, nState.StartedAt); err == nil {
						elapsedSec = int64(time.Since(t).Seconds())
						if elapsedSec < 0 {
							elapsedSec = 0
						}
						break
					}
				}
			}

			tier, label := sysmon.ClassifyNodeResource(nodeType)

			active = append(active, sysmon.ActiveNodeInfo{
				RunID:        run.ID,
				WorkflowName: run.GraphName,
				NodeID:       nodeID,
				NodeTitle:    nodeTitle,
				NodeType:     nodeType,
				Category:     category,
				StartedAt:    nState.StartedAt,
				ElapsedSec:   elapsedSec,
				ResourceTier: tier,
				TierLabel:    label,
			})
		}
	}
	return active
}

// handleSystemMetrics trả về partial HTML để HTMX tự động polling cập nhật RAM/CPU mỗi 3-4s.
func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	activeNodes := s.collectActiveWorkflowNodes()
	snapshot := sysmon.Collect(activeNodes)
	data := s.ctx("Metrics", snapshot)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.templates["dashboard"]
	if !ok {
		http.Error(w, "dashboard template missing", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "system-metrics-panel", data); err != nil {
		log.Printf("web: render system metrics partial: %v", err)
		http.Error(w, "render metrics error", http.StatusInternalServerError)
	}
}

// handleSystemSidebarMetrics trả về partial HTML thanh tài nguyên RAM/CPU ở cuối sidebar.
func (s *Server) handleSystemSidebarMetrics(w http.ResponseWriter, r *http.Request) {
	activeNodes := s.collectActiveWorkflowNodes()
	snapshot := sysmon.Collect(activeNodes)
	data := s.ctx("Metrics", snapshot)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.templates["dashboard"]
	if !ok {
		for _, tmpl := range s.templates {
			t = tmpl
			break
		}
	}
	if t == nil {
		http.Error(w, "template missing", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "sidebar-sysmon", data); err != nil {
		log.Printf("web: render sidebar metrics partial: %v", err)
		http.Error(w, "render sidebar metrics error", http.StatusInternalServerError)
	}
}

// handleKillProcess dừng an toàn tiến trình nền (FFmpeg / AI swapper / Zombie Crawler) đang ngốn tài nguyên.
func (s *Server) handleKillProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pidStr := r.FormValue("pid")
	if pidStr == "" {
		pidStr = r.URL.Query().Get("pid")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	if err := sysmon.KillProcess(pid); err != nil {
		log.Printf("web: kill process %d: %v", pid, err)
	}

	if r.Header.Get("HX-Request") == "true" {
		s.handleSystemMetrics(w, r)
		return
	}
	http.Redirect(w, r, "/?ok=Đã+dừng+tiến+trình+thành+công", http.StatusSeeOther)
}

// ---------------------------------------------------------------- accounts

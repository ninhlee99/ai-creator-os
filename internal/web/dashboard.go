package web

// Dashboard overview page (GET /): active jobs, synced Accesstrade orders,
// automation controls, and worker status.

import (
	"log"
	"net/http"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func (s *Server) dashboardJobViews(jobs []studio.Job) ([]studioJobView, error) {
	views := s.studioJobViews(jobs)
	for i := range views {
		if views[i].Kind == studio.KindStory && s.Ledger != nil {
			_, published, err := s.Ledger.GetSetting(automation.KeyStoryPublishedPrefix + views[i].ID)
			if err != nil {
				return nil, err
			}
			views[i].StoryPublished = published
			views[i].StoryPublishKnown = true
		}
	}
	return views, nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
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
		recentJobs, err = s.dashboardJobViews(s.Studio.ListJobs(6))
		if err != nil {
			s.fail(w, err, "load Story publish markers")
			return
		}
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
	recentPublishOutcomes, err := s.recentPublishOutcomes(recentPublishLegs, 6)
	if err != nil {
		s.fail(w, err, "load recent publish outcomes")
		return
	}
	storyPublishFailures, err := s.unresolvedStoryPublishFailures(10)
	if err != nil {
		s.fail(w, err, "load unresolved Story publish failures")
		return
	}
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
		storyPublishFailures, tickBeats, quotaCircuit)
	attentionSourcesKnown := jobsKnown || s.Growth != nil || s.TickBeats != nil || quotaCircuit.Known
	if len(attentionItems) > 10 {
		attentionItems = attentionItems[:10]
	}
	s.render(w, "dashboard", s.ctx(
		"Notice", r.URL.Query().Get("ok"),
		"Warning", r.URL.Query().Get("warn"),
		"Error", r.URL.Query().Get("err"),
		"Accounts", accounts,
		"ATOrders", atOrders,
		"RecentDecisions", recentDecisions,
		"GrowthKnown", s.Growth != nil,
		"RecentGrowthAlerts", recentGrowthAlerts,
		"GrowthQueue", growthQueue,
		"JobsKnown", jobsKnown,
		"AutoSwitches", s.autoSwitches(),
		"MoneyReadiness", s.moneyReadinessForAccounts(accounts),
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

// ---------------------------------------------------------------- accounts

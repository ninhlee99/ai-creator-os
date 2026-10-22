package harness

import (
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

const maxFormatEvidencePerChannel = 5
const maxLLMEvidenceRunes = 6000

// AutonomyLevel quy định mức độ tự chủ của Harness Agent.
type AutonomyLevel int

const (
	AutonomyAssisted        AutonomyLevel = 1 // Đề xuất; cần người vận hành duyệt.
	AutonomySemiAutonomous  AutonomyLevel = 2 // Legacy policy value; current pipelines still require job-specific inputs.
	AutonomyFullyAutonomous AutonomyLevel = 3 // Legacy policy value; current pipelines do not publish autonomously.
)

// ChannelState là ảnh chụp trạng thái vận hành của một kênh trong danh mục.
type ChannelState struct {
	AccountID          int64                   `json:"account_id"`
	Username           string                  `json:"username"`
	Platform           string                  `json:"platform"`
	Followers          int64                   `json:"followers"`
	Stage              string                  `json:"stage"` // cold_start, format_testing, scaling, monetized, stalled
	BrandNiche         string                  `json:"brand_niche"`
	RecentMetricVideos int                     `json:"recent_metric_videos"`
	AvgViews           float64                 `json:"avg_views"`
	MetricsUpdatedAt   string                  `json:"metrics_updated_at,omitempty"`
	MetricsStatus      string                  `json:"metrics_status"`      // available, missing, unavailable, error
	FormatStatsStatus  string                  `json:"format_stats_status"` // available, missing, unavailable, error
	FormatStats        []ChannelFormatEvidence `json:"format_stats,omitempty"`
	FormatStatsOmitted int                     `json:"format_stats_omitted,omitempty"`
	HealthStatus       string                  `json:"health_status"` // "thriving", "normal", "needs_attention", "stalled"
}

// ChannelFormatEvidence is a read-only projection of stored Growth verdicts.
// UpdatedAt is exposed so consumers can assess staleness instead of treating
// every stored winner/kill decision as current.
type ChannelFormatEvidence struct {
	FormatID         string   `json:"format_id"`
	Videos           int      `json:"videos"`
	Verdict          string   `json:"verdict"`
	VerdictReason    string   `json:"verdict_reason,omitempty"`
	MedianViews      float64  `json:"median_views"`
	AvgViews         float64  `json:"avg_views"`
	MedianCompletion *float64 `json:"median_completion,omitempty"`
	MedianProxyRate  *float64 `json:"median_proxy_rate,omitempty"`
	FollowsPer1k     *float64 `json:"follows_per_1k,omitempty"`
	UpdatedAt        string   `json:"updated_at,omitempty"`
}

// OODAPerception là toàn bộ dữ liệu quan sát môi trường mà Agent Harness thu thập được.
type OODAPerception struct {
	Timestamp       string                   `json:"timestamp"`
	TotalChannels   int                      `json:"total_channels"`
	Channels        []ChannelState           `json:"channels"`
	ActiveJobsCount int                      `json:"active_jobs_count"`
	DoneJobsCount   int                      `json:"done_jobs_count"`
	TopTrendingSong string                   `json:"top_trending_song"`
	HighROIProduct  string                   `json:"high_roi_product"`
	VideoProduction VideoProductionReadiness `json:"video_production"`
	SystemHealth    string                   `json:"system_health"` // "optimal", "busy", "degraded"
}

// VideoProductionReadiness records capabilities reported by the running
// Studio. A storyboard or manual clip workflow does not count as automatic
// production readiness.
type VideoProductionReadiness struct {
	AffiliateAutomaticReady bool   `json:"affiliate_automatic_ready"`
	AffiliateStatus         string `json:"affiliate_status"`
	ViralCharacterEditReady bool   `json:"viral_character_edit_ready"`
	ViralStatus             string `json:"viral_status"`
}

// OODADecisionTask là một hành động cụ thể do Agent quyết định thực thi.
type OODADecisionTask struct {
	ID         string `json:"id"`
	ActionType string `json:"action_type"` // "produce_viral", "produce_affiliate", "replan_channel", "advance_stage"
	TargetUser string `json:"target_user"`
	Niche      string `json:"niche"`
	Topic      string `json:"topic"`
	Reasoning  string `json:"reasoning"` // Lý do AI đưa ra quyết định này
	Priority   int    `json:"priority"`  // 1 (cao nhất) -> 5
	Status     string `json:"status"`    // "pending", "executing", "completed", "failed"
	ResultID   string `json:"result_id,omitempty"`
}

// OODACycleResult gói gọn kết quả của 1 chu kỳ nhận thức và hành động tự chủ.
type OODACycleResult struct {
	CycleID      string             `json:"cycle_id"`
	StartedAt    string             `json:"started_at"`
	FinishedAt   string             `json:"finished_at"`
	Observations OODAPerception     `json:"observations"`
	Diagnosis    string             `json:"diagnosis"` // Phân tích tình hình từ LLM
	Decisions    []OODADecisionTask `json:"decisions"`
	ActionCount  int                `json:"action_count"`
	Status       string             `json:"status"`
}

// OODAEngine thực hiện chu trình Nhận thức - Định hướng - Quyết định - Hành động.
type OODAEngine struct {
	memory *MemoryStore
	growth *growth.Store
	llm    studio.LLM
}

// NewOODAEngine khởi tạo bộ máy OODA.
func NewOODAEngine(memory *MemoryStore, llm studio.LLM, stores ...*growth.Store) *OODAEngine {
	e := &OODAEngine{
		memory: memory,
		llm:    llm,
	}
	if len(stores) > 0 {
		e.growth = stores[0]
	}
	return e
}

// Observe thu thập dữ liệu 360 độ từ các kênh và hệ thống.

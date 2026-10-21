package harness

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
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
func (e *OODAEngine) Observe(ctx context.Context, accounts []*network.Account, activeJobs, doneJobs int) OODAPerception {
	now := time.Now().Format("2006-01-02 15:04:05")
	p := OODAPerception{
		Timestamp:       now,
		TotalChannels:   len(accounts),
		ActiveJobsCount: activeJobs,
		DoneJobsCount:   doneJobs,
		SystemHealth:    "unknown",
	}

	if activeJobs > 5 {
		p.SystemHealth = "busy"
	}

	for _, a := range accounts {
		if a == nil {
			continue
		}
		status := "unknown"
		if a.Status == "paused" || a.Status == "stalled" || a.Status == "penalized" {
			status = "stalled"
		}

		niche := a.Niche

		channel := ChannelState{
			AccountID:         a.ID,
			Username:          a.Username,
			Platform:          "unknown",
			Followers:         a.Followers,
			Stage:             a.Status,
			BrandNiche:        niche,
			MetricsStatus:     "unavailable",
			FormatStatsStatus: "unavailable",
			HealthStatus:      status,
		}
		if e.growth != nil {
			metrics, err := e.growth.RecentVideoMetrics(ctx, a.ID, time.Now().Add(-7*24*time.Hour))
			if err != nil {
				channel.MetricsStatus = "error"
			} else if metrics.VideoCount > 0 {
				channel.MetricsStatus = "available"
				channel.RecentMetricVideos = metrics.VideoCount
				if metrics.AverageViews != nil {
					channel.AvgViews = *metrics.AverageViews
				}
				if !metrics.LatestAt.IsZero() {
					channel.MetricsUpdatedAt = metrics.LatestAt.Local().Format("2006-01-02 15:04:05")
				}
			} else {
				channel.MetricsStatus = "missing"
			}
			formatStats, err := e.growth.ListFormatStats(a.ID)
			if err != nil {
				channel.FormatStatsStatus = "error"
			} else if len(formatStats) == 0 {
				channel.FormatStatsStatus = "missing"
			} else {
				channel.FormatStatsStatus = "available"
				sort.SliceStable(formatStats, func(i, j int) bool {
					pi, pj := formatEvidencePriority(formatStats[i].Verdict), formatEvidencePriority(formatStats[j].Verdict)
					if pi != pj {
						return pi < pj
					}
					if formatStats[i].Videos != formatStats[j].Videos {
						return formatStats[i].Videos > formatStats[j].Videos
					}
					return formatStats[i].FormatID < formatStats[j].FormatID
				})
				if len(formatStats) > maxFormatEvidencePerChannel {
					channel.FormatStatsOmitted = len(formatStats) - maxFormatEvidencePerChannel
					formatStats = formatStats[:maxFormatEvidencePerChannel]
				}
				for _, stat := range formatStats {
					channel.FormatStats = append(channel.FormatStats, ChannelFormatEvidence{
						FormatID:         stat.FormatID,
						Videos:           stat.Videos,
						Verdict:          stat.Verdict,
						VerdictReason:    stat.VerdictReason,
						MedianViews:      stat.MedianViews,
						AvgViews:         stat.AvgViews,
						MedianCompletion: stat.MedianCompletion,
						MedianProxyRate:  stat.MedianProxyRate,
						FollowsPer1k:     stat.FollowsPer1k,
						UpdatedAt:        stat.UpdatedAt,
					})
				}
			}
		}
		p.Channels = append(p.Channels, channel)
	}

	return p
}

func formatEvidencePriority(verdict string) int {
	switch verdict {
	case growth.VerdictWinner:
		return 0
	case growth.VerdictKilled:
		return 1
	case growth.VerdictTesting:
		return 2
	default:
		return 3
	}
}

// OrientAndDecide writes an evidence-bound diagnosis. Autonomy is retained in
// the signature for API compatibility; production actions stay empty until
// source-grounded viral candidates and verified affiliate inputs are wired.
func (e *OODAEngine) OrientAndDecide(ctx context.Context, obs OODAPerception, _ AutonomyLevel) (string, []OODADecisionTask) {
	var tasks []OODADecisionTask

	// Tạo phân tích tình huống
	var diag strings.Builder
	diag.WriteString(fmt.Sprintf("Hệ thống đang quan sát %d kênh. Sức khỏe dây chuyền: %s. ", obs.TotalChannels, obs.SystemHealth))
	videoEvidence := describeVideoProductionReadiness(obs.VideoProduction)
	diag.WriteString(videoEvidence)
	var channelEvidence strings.Builder
	var formatEvidence strings.Builder
	for _, ch := range obs.Channels {
		if ch.MetricsStatus == "available" {
			fmt.Fprintf(&channelEvidence, "@%s: metrics mới cho %d video trong 7 ngày, trung bình %.0f views/video, cập nhật %s. ",
				ch.Username, ch.RecentMetricVideos, ch.AvgViews, ch.MetricsUpdatedAt)
			diag.WriteString(fmt.Sprintf("@%s có metrics mới cho %d video trong 7 ngày; trung bình %.0f views/video. ",
				ch.Username, ch.RecentMetricVideos, ch.AvgViews))
		} else {
			status := ch.MetricsStatus
			if status == "" {
				status = "unknown"
			}
			fmt.Fprintf(&channelEvidence, "@%s: chưa có metrics video dùng được trong 7 ngày (trạng thái %s). ", ch.Username, status)
			diag.WriteString(fmt.Sprintf("@%s chưa có metrics video dùng được trong 7 ngày. ", ch.Username))
		}
		formatSummary := describeChannelFormatEvidence(ch)
		formatEvidence.WriteString(formatSummary)
		diag.WriteString(formatSummary)
	}

	if len(obs.Channels) == 0 {
		diag.WriteString("Chưa có kênh nào được cấu hình trong hệ thống.")
		return diag.String(), nil
	}

	// Do not turn aggregate channel metrics into a production task. A viral
	// decision needs a concrete source video; an affiliate decision needs a
	// verified product and an available motion-production path. Neither input
	// is part of this observation yet.
	if obs.HighROIProduct != "" && !obs.VideoProduction.AffiliateAutomaticReady {
		diag.WriteString("Có tên sản phẩm nhưng chưa đủ bằng chứng listing và footage để tạo job affiliate. ")
	}

	// Nếu LLM sẵn sàng, cho LLM trau chuốt và làm sắc nét chiến lược
	if e.llm != nil && e.llm.Healthy(ctx) {
		evidence := diag.String()
		metricsPromptEvidence := boundLLMEvidence(channelEvidence.String())
		formatPromptEvidence := boundLLMEvidence(formatEvidence.String())
		prompt := fmt.Sprintf(`Bạn là điều phối viên vận hành nội dung.
Chỉ tóm tắt dữ kiện và task đã có trong danh sách; không tự tạo task hoặc gợi ý hành động ngoài danh sách. Nêu rõ trường nào đang thiếu; không suy luận doanh thu, độ an toàn kênh, hiệu quả hay xu hướng nếu dữ liệu không chứng minh. Verdict format là bản lưu; chỉ tường thuật verdict hiện có, không tự tính lại. Nếu updated_at quá 7 ngày, ghi là dữ liệu cũ. Evidence có thể đã bị cắt theo giới hạn; không khái quát phần bị lược.
Khả năng production: %s
Dữ liệu: %d kênh, job hoạt động %d, job hoàn tất %d, sản phẩm được truyền vào: %s.
Metrics theo kênh: %s
Verdict format đã lưu theo kênh: %s
Nhiệm vụ sơ bộ: %d. Tối đa 3 câu tiếng Việt.`,
			videoEvidence, obs.TotalChannels, obs.ActiveJobsCount, obs.DoneJobsCount, obs.HighROIProduct, metricsPromptEvidence, formatPromptEvidence, len(tasks))

		if resp, err := e.llm.Complete(ctx, "Báo cáo điều phối dữ kiện; không phóng đại năng lực.", prompt); err == nil && len(resp) > 20 {
			diag.Reset()
			diag.WriteString(evidence)
			diag.WriteString("Tóm tắt LLM: ")
			diag.WriteString(strings.TrimSpace(resp))
		}
	}

	return diag.String(), tasks
}

func boundLLMEvidence(value string) string {
	runes := []rune(value)
	if len(runes) <= maxLLMEvidenceRunes {
		return value
	}
	return string(runes[:maxLLMEvidenceRunes]) + "… [đã cắt theo giới hạn context]"
}

func describeChannelFormatEvidence(channel ChannelState) string {
	switch channel.FormatStatsStatus {
	case "available":
		if len(channel.FormatStats) == 0 {
			return fmt.Sprintf("@%s: trạng thái format available nhưng không có hàng dữ liệu. ", channel.Username)
		}
	case "missing":
		return fmt.Sprintf("@%s: chưa có verdict format lưu trong Growth. ", channel.Username)
	case "error":
		return fmt.Sprintf("@%s: lỗi đọc verdict format. ", channel.Username)
	default:
		return fmt.Sprintf("@%s: Growth store chưa được nối để đọc verdict format. ", channel.Username)
	}
	var summary strings.Builder
	for _, format := range channel.FormatStats {
		fmt.Fprintf(&summary, "@%s format=%q verdict=%s videos=%d median_views=%.0f avg_views=%.0f updated=%s",
			channel.Username, format.FormatID, format.Verdict, format.Videos, format.MedianViews, format.AvgViews, format.UpdatedAt)
		if format.VerdictReason != "" {
			fmt.Fprintf(&summary, " reason=%q", format.VerdictReason)
		}
		summary.WriteString(". ")
	}
	if channel.FormatStatsOmitted > 0 {
		fmt.Fprintf(&summary, "@%s còn %d format khác không đưa vào evidence rút gọn. ", channel.Username, channel.FormatStatsOmitted)
	}
	return summary.String()
}

// ExecuteCycle thực thi trọn vẹn 1 chu kỳ OODA tự chủ.
func (e *OODAEngine) ExecuteCycle(
	ctx context.Context,
	accounts []*network.Account,
	studioInst *studio.Studio,
	level AutonomyLevel,
) (*OODACycleResult, error) {
	cycleID := fmt.Sprintf("ooda-%d", time.Now().UnixNano())
	start := time.Now().Format("2006-01-02 15:04:05")

	activeCount := 0
	doneCount := 0
	if studioInst != nil {
		for _, j := range studioInst.ListJobs(50) {
			if j.Status == studio.StatusRunning || j.Status == studio.StatusQueued {
				activeCount++
			} else if j.Status == studio.StatusDone {
				doneCount++
			}
		}
	}

	// 1. OBSERVE (Quan sát)
	obs := e.Observe(ctx, accounts, activeCount, doneCount)
	obs.VideoProduction = observeVideoProductionReadiness(ctx, studioInst)

	// 2. ORIENT & DECIDE (Định hướng & Quyết định)
	diag, tasks := e.OrientAndDecide(ctx, obs, level)

	// 3. ACT (Only supported actions may run; currently these require operator input.)
	executedCount := 0
	for i := range tasks {
		t := &tasks[i]
		if level == AutonomyAssisted {
			// Cấp 1 chỉ đề xuất, không tự động enqueue
			t.Status = "waiting_approval"
			continue
		}

		switch t.ActionType {
		case "produce_viral":
			t.Status = "waiting_approval"
			t.Reasoning = "Cần người vận hành chọn video nguồn, kênh và ảnh nhân vật trước khi thay nhận diện."
		case "produce_affiliate":
			t.Status = "waiting_approval"
			if studioInst == nil {
				t.Reasoning = "Studio chưa sẵn sàng; chưa tạo job."
			} else {
				t.Reasoning = "Cần người vận hành xác nhận sản phẩm có ảnh listing thật, kênh và ảnh nhân vật đúng kênh."
			}
		}
	}

	// 4. REFLECT (Phản tỉnh & Ghi nhớ)
	if e.memory != nil && executedCount > 0 {
		_ = e.memory.RecordInsight(ctx, AgentMemory{
			MemoryType:      MemChannelLearning,
			Title:           "Chu kỳ OODA tự hành hoàn tất",
			Insight:         fmt.Sprintf("Đã tự động triển khai %d nhiệm vụ trên %d kênh; nhịp độ phân bổ tối ưu.", executedCount, obs.TotalChannels),
			ConfidenceScore: 0.95,
			ImpactScore:     0.8,
			SampleSize:      executedCount,
		})
	}

	cycleStatus := "completed"
	for _, task := range tasks {
		if task.Status == "waiting_approval" {
			cycleStatus = "waiting_approval"
			break
		}
		if task.Status == "failed" {
			cycleStatus = "completed_with_blockers"
		}
	}
	res := &OODACycleResult{
		CycleID:      cycleID,
		StartedAt:    start,
		FinishedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Observations: obs,
		Diagnosis:    diag,
		Decisions:    tasks,
		ActionCount:  executedCount,
		Status:       cycleStatus,
	}

	return res, nil
}

func observeVideoProductionReadiness(ctx context.Context, studioInst *studio.Studio) VideoProductionReadiness {
	if studioInst == nil {
		return VideoProductionReadiness{
			AffiliateStatus: "Studio chưa khởi tạo; không thể kiểm tra khả năng tạo video.",
			ViralStatus:     "Studio chưa khởi tạo; không thể kiểm tra khả năng thay toàn thân.",
		}
	}
	affiliateReady, affiliateStatus := studioInst.AutoAffiliateProductionReady()
	viralReady, viralStatus := studioInst.ViralCharacterEditorStatus(ctx)
	return VideoProductionReadiness{
		AffiliateAutomaticReady: affiliateReady,
		AffiliateStatus:         affiliateStatus,
		ViralCharacterEditReady: viralReady,
		ViralStatus:             viralStatus,
	}
}

func describeVideoProductionReadiness(readiness VideoProductionReadiness) string {
	affiliate := strings.TrimSpace(readiness.AffiliateStatus)
	if affiliate == "" {
		affiliate = "chưa được kiểm tra"
	}
	viral := strings.TrimSpace(readiness.ViralStatus)
	if viral == "" {
		viral = "chưa được kiểm tra"
	}
	return fmt.Sprintf("Khả năng video: affiliate tự động=%t (%s); thay toàn thân viral=%t (%s). ",
		readiness.AffiliateAutomaticReady, affiliate, readiness.ViralCharacterEditReady, viral)
}

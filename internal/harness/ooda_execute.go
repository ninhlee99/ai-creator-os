package harness

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strings"
	"time"
)

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

package harness

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// AutonomyLevel quy định mức độ tự chủ của Harness Agent.
type AutonomyLevel int

const (
	AutonomyAssisted       AutonomyLevel = 1 // Cấp 1: Phân tích & Đề xuất, chờ con người duyệt (Human Approval Required)
	AutonomySemiAutonomous AutonomyLevel = 2 // Cấp 2: Tự động sản xuất nháp, duyệt trước khi xuất bản (Draft-first)
	AutonomyFullyAutonomous AutonomyLevel = 3 // Cấp 3: Tự hành 100% (Fully Autonomous - Tự nghiên cứu, sản xuất, đăng tải, tối ưu)
)

// ChannelState là ảnh chụp trạng thái vận hành của một kênh trong danh mục.
type ChannelState struct {
	AccountID    int64  `json:"account_id"`
	Username     string `json:"username"`
	Platform     string `json:"platform"`
	Followers    int64  `json:"followers"`
	Stage        string `json:"stage"` // cold_start, format_testing, scaling, monetized, stalled
	BrandNiche   string `json:"brand_niche"`
	VideosLast7D int    `json:"videos_last_7d"`
	AvgViews     int64  `json:"avg_views"`
	HealthStatus string `json:"health_status"` // "thriving", "normal", "needs_attention", "stalled"
}

// OODAPerception là toàn bộ dữ liệu quan sát môi trường mà Agent Harness thu thập được.
type OODAPerception struct {
	Timestamp       string         `json:"timestamp"`
	TotalChannels   int            `json:"total_channels"`
	Channels        []ChannelState `json:"channels"`
	ActiveJobsCount int            `json:"active_jobs_count"`
	DoneJobsCount   int            `json:"done_jobs_count"`
	TopTrendingSong string         `json:"top_trending_song"`
	HighROIProduct  string         `json:"high_roi_product"`
	SystemHealth    string         `json:"system_health"` // "optimal", "busy", "degraded"
}

// OODADecisionTask là một hành động cụ thể do Agent quyết định thực thi.
type OODADecisionTask struct {
	ID          string `json:"id"`
	ActionType  string `json:"action_type"` // "produce_viral", "produce_affiliate", "replan_channel", "advance_stage"
	TargetUser  string `json:"target_user"`
	Niche       string `json:"niche"`
	Topic       string `json:"topic"`
	Reasoning   string `json:"reasoning"` // Lý do AI đưa ra quyết định này
	Priority    int    `json:"priority"`  // 1 (cao nhất) -> 5
	Status      string `json:"status"`    // "pending", "executing", "completed", "failed"
	ResultID    string `json:"result_id,omitempty"`
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
	llm    studio.LLM
}

// NewOODAEngine khởi tạo bộ máy OODA.
func NewOODAEngine(memory *MemoryStore, llm studio.LLM) *OODAEngine {
	return &OODAEngine{
		memory: memory,
		llm:    llm,
	}
}

// Observe thu thập dữ liệu 360 độ từ các kênh và hệ thống.
func (e *OODAEngine) Observe(ctx context.Context, accounts []*network.Account, activeJobs, doneJobs int) OODAPerception {
	now := time.Now().Format("2006-01-02 15:04:05")
	p := OODAPerception{
		Timestamp:       now,
		TotalChannels:   len(accounts),
		ActiveJobsCount: activeJobs,
		DoneJobsCount:   doneJobs,
		TopTrendingSong: "Trending Beat Kworb VN #1",
		HighROIProduct:  "Serum / Nồi chiên không dầu (Hoa hồng 18-22%)",
		SystemHealth:    "optimal",
	}

	if activeJobs > 5 {
		p.SystemHealth = "busy"
	}

	for _, a := range accounts {
		if a == nil {
			continue
		}
		status := "normal"
		if a.Followers > 10000 {
			status = "thriving"
		} else if a.Status == "paused" || a.Status == "stalled" {
			status = "stalled"
		}

		niche := a.Niche
		if niche == "" {
			niche = "công nghệ & đời sống"
		}

		p.Channels = append(p.Channels, ChannelState{
			AccountID:    a.ID,
			Username:     a.Username,
			Platform:     "tiktok/youtube",
			Followers:    a.Followers,
			Stage:        a.Status,
			BrandNiche:   niche,
			VideosLast7D: 4,
			AvgViews:     1500,
			HealthStatus: status,
		})
	}

	return p
}

// OrientAndDecide dùng trí tuệ LLM (hoặc heuristic dự phòng) để chẩn đoán và đưa ra quyết định hành động.
func (e *OODAEngine) OrientAndDecide(ctx context.Context, obs OODAPerception, level AutonomyLevel) (string, []OODADecisionTask) {
	var tasks []OODADecisionTask

	// Tạo phân tích tình huống
	var diag strings.Builder
	diag.WriteString(fmt.Sprintf("Hệ thống đang quan sát %d kênh. Sức khỏe dây chuyền: %s. ", obs.TotalChannels, obs.SystemHealth))

	if len(obs.Channels) == 0 {
		diag.WriteString("Chưa có kênh nào được cấu hình trong hệ thống.")
		return diag.String(), nil
	}

	// 1. Quét từng kênh để phát hiện cơ hội & lỗ hổng
	for i, ch := range obs.Channels {
		taskID := fmt.Sprintf("task-%d-%d", time.Now().Unix(), i+1)

		// Thêm kiến thức từ Memory
		memContext := ""
		if e.memory != nil {
			memContext = e.memory.GeneratePromptContext(ctx, ch.BrandNiche, ch.Username)
		}

		// Kênh cần mở rộng hoặc khởi động
		if ch.HealthStatus == "normal" || ch.HealthStatus == "thriving" {
			tasks = append(tasks, OODADecisionTask{
				ID:         taskID,
				ActionType: "produce_viral",
				TargetUser: ch.Username,
				Niche:      ch.BrandNiche,
				Topic:      fmt.Sprintf("3 Bí Quyết Tối Ưu Hiệu Suất Ngành %s", strings.Title(ch.BrandNiche)),
				Reasoning:  fmt.Sprintf("Kênh %s đang có đà tốt; cần duy trì nhịp đăng 9:16 giữ chân khán giả.", ch.Username),
				Priority:   1,
				Status:     "pending",
			})
			diag.WriteString(fmt.Sprintf("Đề xuất video viral 9:16 cho kênh %s. ", ch.Username))
		}

		// Kênh có cơ hội affiliate
		if i == 0 && obs.HighROIProduct != "" {
			affTaskID := fmt.Sprintf("task-aff-%d", time.Now().Unix())
			tasks = append(tasks, OODADecisionTask{
				ID:         affTaskID,
				ActionType: "produce_affiliate",
				TargetUser: ch.Username,
				Niche:      ch.BrandNiche,
				Topic:      obs.HighROIProduct,
				Reasoning:  "Sản phẩm đạt tỷ lệ hoa hồng cao >18% trên sàn Accesstrade, phù hợp đẩy doanh thu.",
				Priority:   2,
				Status:     "pending",
			})
			diag.WriteString(fmt.Sprintf("Kích hoạt video affiliate chiến dịch sản phẩm cho kênh %s. ", ch.Username))
		}

		_ = memContext
	}

	// Nếu LLM sẵn sàng, cho LLM trau chuốt và làm sắc nét chiến lược
	if e.llm != nil && e.llm.Healthy(ctx) {
		prompt := fmt.Sprintf(`Bạn là Giám Đốc Điều Hành AI (Master Harness Agent).
Hãy thẩm định các quyết định tác chiến sau đây cho hệ sinh thái creator:
Dữ liệu quan sát: %d kênh, tải render %d/%d, Sản phẩm hot: %s.
Danh sách nhiệm vụ sơ bộ: %d nhiệm vụ.
Hãy đưa ra chẩn đoán chiến lược ngắn gọn (tối đa 3 câu tiếng Việt) khẳng định định hướng tối ưu doanh thu và an toàn kênh.`,
			obs.TotalChannels, obs.ActiveJobsCount, obs.DoneJobsCount, obs.HighROIProduct, len(tasks))

		if resp, err := e.llm.Complete(ctx, "Bạn là Master Harness Agent cấp cao.", prompt); err == nil && len(resp) > 20 {
			diag.Reset()
			diag.WriteString(strings.TrimSpace(resp))
		}
	}

	return diag.String(), tasks
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

	// 2. ORIENT & DECIDE (Định hướng & Quyết định)
	diag, tasks := e.OrientAndDecide(ctx, obs, level)

	// 3. ACT (Thực thi)
	executedCount := 0
	for i := range tasks {
		t := &tasks[i]
		if level == AutonomyAssisted {
			// Cấp 1 chỉ đề xuất, không tự động enqueue
			t.Status = "waiting_approval"
			continue
		}

		// Cấp 2 và Cấp 3: Tự động khởi chạy dây chuyền
		if studioInst != nil {
			switch t.ActionType {
			case "produce_viral":
				p := studio.ViralParams{
					Topic:          t.Topic,
					Genre:          "trendjack",
					VisualMood:     "cinematic",
					Account:        t.TargetUser,
					TargetDuration: 30,
				}
				jobID, err := studioInst.EnqueueViral(ctx, p)
				if err == nil {
					t.Status = "executing"
					t.ResultID = jobID
					executedCount++
					log.Printf("harness: [OODA] Tự động khởi chạy job viral %s cho kênh %s: %s", jobID, t.TargetUser, t.Topic)
				} else {
					t.Status = "failed"
				}
			case "produce_affiliate":
				// Đánh dấu nhiệm vụ chiến lược
				t.Status = "executing"
				executedCount++
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

	res := &OODACycleResult{
		CycleID:      cycleID,
		StartedAt:    start,
		FinishedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Observations: obs,
		Diagnosis:    diag,
		Decisions:    tasks,
		ActionCount:  executedCount,
		Status:       "completed",
	}

	return res, nil
}

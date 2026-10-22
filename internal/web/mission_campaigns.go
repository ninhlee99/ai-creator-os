package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func (s *Server) executeStoryMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.CurrentStep = "Xếp job truyện theo yêu cầu"
	if s.Studio == nil {
		m.Status = "failed"
		m.CurrentStep = "Studio chưa sẵn sàng"
		m.ResultSummary = "Studio chưa sẵn sàng; chưa tạo job truyện."
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: m.CurrentStep, Status: "failed", Detail: m.ResultSummary, Timestamp: now})
		return
	}
	topic := strings.TrimSpace(m.Goal)
	jobID, err := s.Studio.CreateStoryJob(studio.StoryParams{
		Topic: topic, Words: 450, Scenes: 5,
	})
	if err != nil {
		m.Status = "failed"
		m.CurrentStep = "Không xếp được job truyện"
		m.ResultSummary = "Không tạo được job truyện: " + err.Error()
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: m.CurrentStep, Status: "failed", Detail: m.ResultSummary, Timestamp: now})
		return
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Xếp job truyện vào Studio", Status: "done",
		Detail: fmt.Sprintf("Đã xếp job %s theo yêu cầu gốc; truyện/video chưa hoàn tất.", jobID), Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/jobs?tab=running"
	m.ResultSummary = fmt.Sprintf("Đã xếp job truyện %s theo yêu cầu; kết quả sẽ xuất hiện khi job hoàn tất.", jobID)
}

func (s *Server) executeViralSourceReplaceMission(m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Status = "waiting_approval"
	m.Progress = 20
	m.CurrentStep = "Chọn video nguồn và ảnh nhân vật"
	m.ArtifactURL = "/viral"
	m.ResultSummary = "Cần người vận hành chọn video nguồn 9:16, kênh đích và ảnh nhân vật thuộc đúng kênh tại /viral. Chưa tạo job."
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Chờ đầu vào source_replace", Status: "pending",
		Detail: m.ResultSummary, Timestamp: now,
	})
}

func (s *Server) executeTrendMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Quét tín hiệu xu hướng TikTok & Douyin"
	fail := func(title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	sounds, err := studio.FetchVNTrending(ctx)
	if err != nil || len(sounds) == 0 {
		if err == nil {
			err = fmt.Errorf("nguồn không trả bài hát")
		}
		fail("Đọc bảng xu hướng hiện tại", "Không lấy được tín hiệu xu hướng trực tiếp: "+err.Error())
		return
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Đọc bảng âm thanh xu hướng hiện tại", Status: "done",
		Detail: fmt.Sprintf("Đã nhận %d bài từ %s.", len(sounds), studio.TrendsSource), Timestamp: now,
	})

	var brand *growth.BrandDNA
	if s.Growth != nil && m.AccountID > 0 {
		brand, _ = s.Growth.GetBrandDNA(m.AccountID)
	}
	signals := make([]growth.TrendSignal, 0, len(sounds))
	for _, sound := range sounds {
		signals = append(signals, growth.TrendSignal{
			Title: sound.Title, Artist: sound.Artist, Movement: sound.Movement,
			Source: studio.TrendsSource, Rank: sound.Rank,
		})
	}
	opps := growth.AnalyzeTrendSignals(ctx, brand, signals)
	var topAngle string
	if len(opps) > 0 && len(opps[0].OriginalAngles) > 0 {
		topAngle = opps[0].OriginalAngles[0]
	}
	detail := fmt.Sprintf("Gợi ý heuristic từ dữ liệu hiện tại: %s", topAngle)
	if topAngle == "" {
		detail = "Đã lưu tín hiệu thực; chưa tạo được góc nội dung từ bộ phân tích hiện tại."
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-3", Title: "Tạo gợi ý nội dung từ tín hiệu thật", Status: "done",
		Detail: detail, Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/trends"
	m.ResultSummary = fmt.Sprintf("Đã đọc %d tín hiệu hiện tại từ %s. Gợi ý nội dung mang tính heuristic, không phải dự báo lượt xem.", len(sounds), studio.TrendsSource)
}

func (s *Server) executeGrowthMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Tạo lịch nội dung theo cấu hình kênh"
	fail := func(title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	if s.Mgr == nil || m.AccountID <= 0 {
		fail("Chọn kênh đích", "Không có kênh để lập kế hoạch.")
		return
	}
	if s.Growth == nil {
		fail("Kiểm tra kho tăng trưởng", "Kho tăng trưởng chưa sẵn sàng; chưa lập kế hoạch.")
		return
	}
	acct, err := s.Mgr.Get(m.AccountID)
	if err != nil || acct == nil {
		fail("Đọc cấu hình kênh", "Không đọc được kênh đích.")
		return
	}
	stats, statsErr := s.Growth.ListFormatStats(m.AccountID)
	if statsErr != nil {
		fail("Đọc thống kê format", "Không đọc được dữ liệu format: "+statsErr.Error())
		return
	}
	killed := 0
	for _, stat := range stats {
		if stat.Verdict == growth.VerdictKilled {
			killed++
		}
	}
	count, err := s.generatePlanFor(acct, "Lịch nội dung theo ngách và giai đoạn kênh")
	if err != nil {
		fail("Tạo lịch nội dung", "Không lập được lịch: "+err.Error())
		return
	}
	snapshot, snapshotErr := s.Growth.LatestSnapshot(m.AccountID)
	snapshotDetail := "Chưa có snapshot chỉ số kênh."
	if snapshotErr == nil && snapshot != nil {
		snapshotDetail = "Có snapshot chỉ số kênh; lịch vẫn dựa trên ngách, giai đoạn và format đã đánh dấu dừng."
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Đọc cấu hình tăng trưởng hiện có", Status: "done",
		Detail: fmt.Sprintf("%d format đã đánh dấu dừng. %s Không dự báo mức tăng view/retention.", killed, snapshotDetail), Timestamp: now,
	})
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-3", Title: "Tạo kế hoạch nội dung 30 ngày", Status: "done",
		Detail: fmt.Sprintf("Đã tạo %d vị trí theo ngách và giai đoạn kênh; loại %d format đã đánh dấu dừng.", count, killed), Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/growth"
	m.ResultSummary = fmt.Sprintf("Đã tạo lịch 30 ngày cho @%s từ cấu hình kênh và format đã đánh dấu dừng. Không khẳng định mức tăng hiệu quả.", acct.Username)
}

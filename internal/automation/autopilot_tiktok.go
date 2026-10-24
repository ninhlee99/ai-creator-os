package automation

import (
	"context"
	"fmt"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// autoPublishTikTokAffiliate uploads finished affiliate output as a TikTok
// Creator Inbox draft. Creator still attaches product link and sound in app.
func (s *Service) autoPublishTikTokAffiliate(ctx context.Context, jobID string, job studio.Job, params studio.AffiliateParams, account *network.Account) {
	unlock, locked := tryAffiliatePublishLock(jobID, "tiktok")
	if !locked {
		s.JobStore.AppendLog(jobID, "TikTok draft đang được xử lý; bỏ qua lượt gọi trùng.")
		return
	}
	defer unlock()

	switch s.JobStore.PublishLegStatus(jobID, "tiktok") {
	case studio.LegDone:
		s.JobStore.AppendLog(jobID, "TikTok draft đã tải lên; bỏ qua lượt gọi trùng.")
		return
	case studio.LegNeedsReview, studio.LegPublishing:
		s.JobStore.AppendLog(jobID, "Kết quả gửi TikTok chưa rõ; kiểm tra Creator Inbox trước khi thử lại.")
		return
	}

	publisher := publishers.NewTikTokPublisher(account.Username)
	if !publisher.IsConfigured() || !publisher.Handles("short_video") {
		s.JobStore.AppendLog(jobID, "TikTok chưa kết nối OAuth; video vẫn nằm trong output Studio.")
		return
	}
	if err := s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegPublishing, ""); err != nil {
		s.JobStore.AppendLog(jobID, "Không lưu được trạng thái gửi TikTok; chưa gọi nền tảng.")
		return
	}

	title := job.Title
	if params.ProductName != "" {
		title = params.ProductName
	}
	s.JobStore.AppendLog(jobID, "Đang tải video lên TikTok Creator Inbox…")
	result := publisher.Publish(ctx, s.OutputPath(job.Output), title, job.Caption, "short_video")
	if !result.Ok {
		if result.DeliveryUncertain {
			note := "TikTok chưa xác nhận kết quả; dừng retry để tránh tạo draft trùng. Kiểm tra Creator Inbox trước."
			s.JobStore.AppendLog(jobID, note+" Lỗi: "+result.Error)
			_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegNeedsReview, "")
			_ = s.decide("autopilot", "tiktok_draft_uncertain", &jobID, note, map[string]any{"error": result.Error})
			return
		}
		s.JobStore.AppendLog(jobID, "Tải TikTok draft thất bại: "+result.Error)
		_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegFailed, "")
		_ = s.decide("autopilot", "tiktok_draft_failed", &jobID, result.Error, nil)
		return
	}

	_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegDone, result.RemoteID)
	s.JobStore.AppendLog(jobID, fmt.Sprintf("Đã tải TikTok draft (%s). Mở TikTok để gắn link sản phẩm, sound Radar và duyệt đăng.", result.RemoteID))
	_ = s.decide("autopilot", "tiktok_draft_uploaded", &jobID, "đã tải video lên Creator Inbox", map[string]any{"publish_id": result.RemoteID})
}

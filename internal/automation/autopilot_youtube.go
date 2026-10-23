package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

var youtubeAffiliatePublisher = func(acct *network.Account) *publishers.YouTubePublisher {
	pub := publishers.NewYouTubePublisher(acct.Username, acct.YoutubeContentTypes, acct.YoutubeChannel)
	pub.Synthetic = true // AI disclosure bắt buộc — video dựng từ ảnh AI
	return pub
}

// autoPublishYouTubeAffiliate tải video affiliate lên YouTube Shorts theo
// YOUTUBE_DEFAULT_PRIVACY (mặc định private). Quota-guarded: hết quota ngày
// thì ghi log và bỏ qua (video vẫn nằm ở output).
func (s *Service) autoPublishYouTubeAffiliate(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account) {
	// Đợt Y: idempotency — chân này đã done thì bỏ qua.
	legStatus := s.JobStore.PublishLegStatus(jobID, "youtube")
	if legStatus == studio.LegDone {
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube: đã đăng rồi — bỏ qua.")
		return
	}
	if legStatus == studio.LegNeedsReview {
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube: kết quả lần trước chưa rõ — kiểm tra YouTube Studio trước khi thử lại.")
		return
	}
	pub := youtubeAffiliatePublisher(acct)
	if !pub.IsConfigured() || !pub.Handles("short_video") {
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube: kênh chưa cấu hình OAuth hoặc chưa bật loại short_video — bỏ qua.")
		return
	}
	if s.Growth == nil {
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube: không có bộ đếm quota — dừng trước khi tải lên.")
		_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegFailed, "")
		return
	}
	quotaDay := s.quotaDay()
	used, reserved, err := s.Growth.ReserveUploadQuota(quotaDay)
	if err != nil {
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube: không ghi được bộ đếm quota — dừng trước khi tải lên: "+err.Error())
		_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegFailed, "")
		return
	}
	if !reserved {
		s.JobStore.AppendLog(jobID, fmt.Sprintf("Tự đăng YouTube: chờ quota (%d/%d lượt trong ngày reset Pacific) — video nằm ở output, đăng tay nếu cần.", used, growth.DailyQuotaUnits))
		// Leg failed so next cycle can check again after Google's Pacific reset.
		_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegFailed, "")
		return
	}
	title := j.Title
	if p.ProductName != "" {
		title = p.ProductName
	}
	desc := "Video giới thiệu sản phẩm dựng bằng AI."
	if p.AffLink != "" {
		desc += "\nLink mua: " + p.AffLink
	}
	desc += "\n\n" + growth.DisclosureLine
	s.JobStore.AppendLog(jobID, "Tự đăng YouTube Shorts (private)…")
	videoPath := s.OutputPath(j.Output)
	res := pub.Publish(ctx, videoPath, title, desc, "short_video")
	if !res.Ok {
		if !res.UploadRequestAttempted {
			_ = s.Growth.ReleaseUploadQuota(quotaDay)
		}
		if isYouTubeQuotaExceeded(res.Error) {
			_ = s.Growth.ExhaustUploadQuota(quotaDay)
		}
		if res.DeliveryUncertain {
			note := "Kết quả upload chưa xác định; dừng retry để tránh đăng trùng. Kiểm tra YouTube Studio trước khi xử lý tiếp."
			s.JobStore.AppendLog(jobID, note+" Lỗi: "+res.Error)
			_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegNeedsReview, "")
			_ = s.decide("autopilot", "autopublish_yt_uncertain", &jobID,
				note, map[string]any{"error": res.Error, "delivery_uncertain": true})
			return
		}
		s.JobStore.AppendLog(jobID, "Tự đăng YouTube thất bại: "+res.Error)
		_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegFailed, "")
		_ = s.decide("autopilot", "autopublish_yt_failed", &jobID,
			"tự đăng YouTube thất bại: "+res.Error, nil)
		return
	}
	_ = s.JobStore.SetPublishLeg(jobID, "youtube", studio.LegDone, res.RemoteID)
	note := "Đã đăng YouTube Shorts (private)"
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	note += " — xem lại trong YouTube Studio."
	s.JobStore.AppendLog(jobID, note)
}

// AutoPublishLoggable wraps SetOnDone wiring: the closure form main.go
// used, now pointing at the service.

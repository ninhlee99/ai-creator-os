package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strings"
)

var metaPublishersFor = func(acct *network.Account) []publishers.Publisher {
	return publishers.BuildPublishers(acct.Username, acct.YoutubeChannel, acct.YoutubeContentTypes, "short_video")
}

// metaPlatforms là các chân đăng chéo Meta, theo thứ tự đăng.
var metaPlatforms = []string{"facebook", "instagram", "threads"}

// autoPublishMetaAffiliate gửi nội dung affiliate qua các adapter Meta đã
// cấu hình. Facebook/Instagram nhận video; Threads đăng bài chữ kèm link.
// Mỗi nền tảng là một chân riêng (idempotent theo leg).
func (s *Service) autoPublishMetaAffiliate(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account) {
	s.autoPublishMetaAffiliatePlatforms(ctx, jobID, j, p, acct, metaPlatforms)
}

func (s *Service) autoPublishMetaAffiliatePlatform(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account, platform string) {
	s.autoPublishMetaAffiliatePlatforms(ctx, jobID, j, p, acct, []string{platform})
}

func (s *Service) autoPublishMetaAffiliatePlatforms(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account, platforms []string) {
	pubs := map[string]publishers.Publisher{}
	for _, c := range metaPublishersFor(acct) {
		pubs[c.Name()] = c
	}
	title := j.Title
	if p.ProductName != "" {
		title = p.ProductName
	}
	desc := strings.TrimSpace(j.Caption)
	if p.AffLink != "" {
		desc += "\nLink mua: " + p.AffLink
	}
	desc = strings.TrimSpace(desc + "\n\n" + growth.DisclosureLine)
	for _, name := range platforms {
		pub, ok := pubs[name]
		if !ok {
			continue
		}
		s.publishMetaAffiliatePlatform(ctx, jobID, name, pub, j.Output, title, desc)
	}
}

func (s *Service) publishMetaAffiliatePlatform(ctx context.Context, jobID, name string, pub publishers.Publisher, output, title, desc string) {
	unlock, locked := tryAffiliatePublishLock(jobID, name)
	if !locked {
		s.JobStore.AppendLog(jobID, "Tự đăng "+name+": đang có yêu cầu xử lý — bỏ qua lượt gọi trùng.")
		return
	}
	defer unlock()
	legStatus := s.JobStore.PublishLegStatus(jobID, name)
	if legStatus == studio.LegDone {
		return
	}
	if legStatus == studio.LegNeedsReview || legStatus == studio.LegPublishing {
		s.JobStore.AppendLog(jobID, "Tự đăng "+name+": kết quả lần trước chưa rõ hoặc đang gửi — kiểm tra trạng thái trước khi thử lại.")
		return
	}
	if err := s.JobStore.SetPublishLeg(jobID, name, studio.LegPublishing, ""); err != nil {
		s.JobStore.AppendLog(jobID, "Tự đăng "+name+": không lưu được trạng thái gửi; chưa gọi nền tảng.")
		return
	}
	res := pub.Publish(ctx, s.OutputPath(output), title, desc, "short_video")
	if !res.Ok {
		if res.DeliveryUncertain {
			note := "Kết quả publish chưa xác định; dừng retry để tránh đăng trùng. Kiểm tra " + name + " trước khi xử lý tiếp."
			s.JobStore.AppendLog(jobID, note+" Lỗi: "+res.Error)
			_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegNeedsReview, "")
			_ = s.decide("autopilot", "autopublish_meta_uncertain", &jobID,
				note, map[string]any{"platform": name, "error": res.Error, "delivery_uncertain": true})
			return
		}
		s.JobStore.AppendLog(jobID, "Tự đăng "+name+" thất bại: "+res.Error)
		_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegFailed, "")
		_ = s.decide("autopilot", "autopublish_meta_failed", &jobID,
			"tự đăng "+name+" thất bại: "+res.Error, nil)
		return
	}
	if name == "threads" && strings.TrimSpace(res.RemoteID) == "" {
		note := "Threads API báo thành công nhưng thiếu post ID; kiểm tra bài trên Threads trước khi xác nhận hoặc thử lại."
		s.JobStore.AppendLog(jobID, note)
		_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegNeedsReview, "")
		_ = s.decide("autopilot", "autopublish_meta_uncertain", &jobID,
			"Threads API báo thành công nhưng thiếu post ID; cần đối chiếu trước khi thử lại.", map[string]any{
				"platform": name, "delivery_uncertain": true,
			})
		return
	}
	_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegDone, res.RemoteID)
	s.JobStore.AppendLog(jobID, metaPublishSuccessNote(name, res.Draft, res.RemoteID))
}

func metaPublishSuccessNote(platform string, draft bool, remoteID string) string {
	switch platform {
	case "facebook":
		if draft {
			return "Facebook: đã tạo Page draft chờ duyệt."
		}
		return "Đã đăng video lên Facebook."
	case "instagram":
		if draft {
			return "Instagram: video đã tải lên và xử lý; chưa công khai vì IG_AUTO_PUBLISH chưa bật."
		}
		return "Đã đăng Instagram Reels."
	case "threads":
		if strings.TrimSpace(remoteID) == "" {
			return "Threads API báo thành công nhưng thiếu post ID; kiểm tra bài trên Threads trước khi thử lại."
		}
		return "Đã đăng bài chữ kèm link lên Threads; không tải video."
	default:
		return "API đã nhận publish cho " + platform + "."
	}
}

// autoPublishTikTokAffiliate đăng video affiliate lên TikTok (nháp).
func (s *Service) autoPublishTikTokAffiliate(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account) {
	unlock, locked := tryAffiliatePublishLock(jobID, "tiktok")
	if !locked {
		s.JobStore.AppendLog(jobID, "Tự đăng TikTok: đang có yêu cầu xử lý — bỏ qua lượt gọi trùng.")
		return
	}
	defer unlock()
	// Đợt Y: idempotency — chân này đã done thì bỏ qua (chống hook fire
	// 2 lần hoặc retry tick chạy lại sau khi đã đăng).
	legStatus := s.JobStore.PublishLegStatus(jobID, "tiktok")
	if legStatus == studio.LegDone {
		s.JobStore.AppendLog(jobID, "Tự đăng TikTok: đã đăng rồi — bỏ qua.")
		return
	}
	if legStatus == studio.LegNeedsReview || legStatus == studio.LegPublishing {
		s.JobStore.AppendLog(jobID, "Tự đăng TikTok: kết quả lần trước chưa rõ — kiểm tra TikTok trước khi thử lại.")
		return
	}
	var pub publishers.Publisher
	for _, c := range publishers.BuildPublishers(acct.Username, acct.YoutubeChannel, acct.YoutubeContentTypes) {
		if c.Name() == "tiktok" && c.IsConfigured() && c.Handles("short_video") {
			pub = c
			break
		}
	}
	if pub == nil {
		s.JobStore.AppendLog(jobID, "Tự đăng: TikTok chưa cấu hình OAuth — video nằm ở output, đăng tay hoặc bật sau khi OAuth xong.")
		return
	}
	if err := s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegPublishing, ""); err != nil {
		s.JobStore.AppendLog(jobID, "Tự đăng TikTok: không lưu được trạng thái gửi; chưa gọi nền tảng.")
		return
	}
	title := j.Title
	if p.ProductName != "" {
		title = p.ProductName
	}
	s.JobStore.AppendLog(jobID, "Tự đăng TikTok…")
	videoPath := s.OutputPath(j.Output)
	res := pub.Publish(ctx, videoPath, title, j.Caption, "short_video")
	if !res.Ok {
		if res.DeliveryUncertain {
			note := "Kết quả upload TikTok chưa xác định; dừng retry để tránh draft trùng. Kiểm tra TikTok trước khi xử lý tiếp."
			s.JobStore.AppendLog(jobID, note+" Lỗi: "+res.Error)
			_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegNeedsReview, "")
			_ = s.decide("autopilot", "autopublish_tiktok_uncertain", &jobID,
				note, map[string]any{"error": res.Error, "delivery_uncertain": true})
			return
		}
		s.JobStore.AppendLog(jobID, "Tự đăng thất bại: "+res.Error)
		_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegFailed, "")
		_ = s.decide("autopilot", "autopublish_failed", &jobID,
			"tự đăng TikTok thất bại: "+res.Error, nil)
		return
	}
	_ = s.JobStore.SetPublishLeg(jobID, "tiktok", studio.LegDone, res.RemoteID)
	if res.Draft {
		s.JobStore.AppendLog(jobID, "Đã đăng lên TikTok dưới dạng NHÁP (draft id "+res.RemoteID+") — gắn giỏ hàng + sound trong app TikTok rồi hãy public.")
		if s.Settings != nil && atOn(s.Settings, KeyNotifyTelegramOn, false) {
			bot, _ := s.Settings.Get(KeyNotifyTelegramBot)
			chat, _ := s.Settings.Get(KeyNotifyTelegramChat)
			if bot != "" && chat != "" {
				msg := fmt.Sprintf("📱 [TikTok Nháp] Video \"%s\" (Kênh @%s) đã tải lên nháp thành công!\n👉 Hãy mở app TikTok trên điện thoại để gắn giỏ hàng và duyệt đăng vào khung giờ vàng.", title, acct.Username)
				_ = notify.SendTelegram(bot, chat, msg)
			}
		}
	} else {
		s.JobStore.AppendLog(jobID, "Đã đăng TikTok (id "+res.RemoteID+").")
	}
}

// youtubeAffiliatePublisher dựng YouTube publisher cho tài khoản
// (package-level seam để test inject HTTP giả).

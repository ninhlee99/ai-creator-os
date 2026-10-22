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

// autoPublishMetaAffiliate đăng video affiliate lên các nền tảng Meta đã
// cấu hình. Mỗi nền tảng là một chân riêng (idempotent theo leg).
func (s *Service) autoPublishMetaAffiliate(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account) {
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
	for _, name := range metaPlatforms {
		pub, ok := pubs[name]
		if !ok {
			continue
		}
		if s.JobStore.PublishLegStatus(jobID, name) == studio.LegDone {
			continue
		}
		videoPath := s.OutputPath(j.Output)
		res := pub.Publish(ctx, videoPath, title, desc, "short_video")
		if !res.Ok {
			s.JobStore.AppendLog(jobID, "Tự đăng "+name+" thất bại: "+res.Error)
			_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegFailed, "")
			_ = s.decide("autopilot", "autopublish_meta_failed", &jobID,
				"tự đăng "+name+" thất bại: "+res.Error, nil)
			continue
		}
		_ = s.JobStore.SetPublishLeg(jobID, name, studio.LegDone, res.RemoteID)
		note := "Đã đăng " + name
		if res.Draft {
			note += " (nháp — duyệt rồi công khai)"
		}
		s.JobStore.AppendLog(jobID, note+".")
	}
}

// autoPublishTikTokAffiliate đăng video affiliate lên TikTok (nháp).
func (s *Service) autoPublishTikTokAffiliate(ctx context.Context, jobID string, j studio.Job, p studio.AffiliateParams, acct *network.Account) {
	// Đợt Y: idempotency — chân này đã done thì bỏ qua (chống hook fire
	// 2 lần hoặc retry tick chạy lại sau khi đã đăng).
	if s.JobStore.PublishLegStatus(jobID, "tiktok") == studio.LegDone {
		s.JobStore.AppendLog(jobID, "Tự đăng TikTok: đã đăng rồi — bỏ qua.")
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
	title := j.Title
	if p.ProductName != "" {
		title = p.ProductName
	}
	s.JobStore.AppendLog(jobID, "Tự đăng TikTok…")
	videoPath := s.OutputPath(j.Output)
	res := pub.Publish(ctx, videoPath, title, j.Caption, "short_video")
	if !res.Ok {
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

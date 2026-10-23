package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strings"
	"time"
)

// ------------------------------------------------- scheduler boundaries

// JobStore is the studio-job boundary the autopilot needs.
// *studio.Studio satisfies it.
type JobStore interface {
	GetJob(id string) (studio.Job, bool)
	AppendLog(id, line string)
	// Publish legs (Đợt Y): idempotency cho auto-publish — check trước
	// khi đăng, ghi sau khi đăng, tick thử lại chân failed.
	PublishLegStatus(jobID, platform string) string
	PublishLegRemoteID(jobID, platform string) string
	SetPublishLeg(jobID, platform, status, remoteID string) error
	FailedPublishLegs() []studio.PublishLeg
}

// ----------------------------------------------------------- the tick

// AutopilotTick attempts one scheduled affiliate cycle. The current Studio
// runner stops before product discovery until per-account Google Flow credit
// authorization exists. Returns log notes; the Studio's SetOnDone hook
// handles publishing separately. Cadence:
// the UI-managed interval (default 6h); gates: kill switch, DRY-RUN,
// the autopilot_enabled switch (default ON when unset — zero-touch,
// a stored "0" opts out).
func (s *Service) AutopilotTick(ctx context.Context) []string {
	if s.Autopilot == nil {
		return nil
	}
	if s.killed() || s.dryRun() {
		return nil
	}
	if !AutopilotEnabled(s.Settings) {
		return nil
	}
	hours := AutopilotIntervalHours(s.Settings)
	if last := AutopilotLastRun(s.Settings); last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil {
			if time.Since(t) < time.Duration(hours)*time.Hour {
				return nil
			}
		}
	}
	results := s.Autopilot.RunAll(ctx)
	done := 0
	for _, r := range results {
		if r.JobID != "" {
			done++
			continue
		}
		// Đợt Y: lỗi autopilot theo account trước đây vô hình (nhét vào
		// Title rồi chỉ đi vào server log). Giờ alert warn để Ninh thấy.
		if msg, ok := strings.CutPrefix(r.Product.Title, "LỖI: "); ok {
			s.alertAccountOnce(r.AccountID, r.AccountName,
				"Autopilot "+r.AccountName+" lỗi: "+msg,
				"kiểm tra theme/ảnh mẫu/API key trong UI rồi bật lại")
		}
	}
	// Đợt Y: thử lại các chân đăng lỗi tạm thời ở chu kỳ trước.
	retryNotes := s.retryFailedPublishLegs(ctx)
	if s.Settings != nil {
		_ = s.Settings.Set("autopilot_last_run", time.Now().Format(time.RFC3339))
	}
	notes := []string{fmt.Sprintf("autopilot: chu kỳ đã xong (%d video đã xếp)", done)}
	return append(notes, retryNotes...)
}

// alertAccountOnce gửi alert warn cho account, tối đa 1 lần/24h cho cùng
// account (tránh spam Telegram mỗi chu kỳ khi lỗi kéo dài).
func (s *Service) alertAccountOnce(accountID int64, accountName, msg, action string) {
	if s.Growth == nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05")
	if alerts, err := s.Growth.ListAlerts(0, 100); err == nil {
		for _, a := range alerts {
			if a.Kind == "autopilot_account" && a.CreatedAt > cutoff &&
				strings.Contains(a.Message, accountName) {
				return
			}
		}
	}
	s.alert("autopilot_account", "warn", msg, action)
}

// retryFailedPublishLegs thử lại các chân đăng (TikTok/YouTube/Meta) bị lỗi tạm
// thời ở chu kỳ trước (Đợt Y). Job phải còn done + output còn nguyên.
func (s *Service) retryFailedPublishLegs(ctx context.Context) []string {
	if s.JobStore == nil || s.Accounts == nil {
		return nil
	}
	var notes []string
	for _, leg := range s.JobStore.FailedPublishLegs() {
		j, ok := s.JobStore.GetJob(leg.JobID)
		if !ok || j.Kind != studio.KindAffiliate || j.Status != studio.StatusDone || j.Output == "" {
			continue
		}
		var p studio.AffiliateParams
		if err := json.Unmarshal([]byte(j.Params), &p); err != nil || p.AccountID == 0 {
			continue
		}
		acct, err := s.Accounts.Get(p.AccountID)
		if err != nil {
			continue
		}
		switch leg.Platform {
		case "tiktok":
			if AutopilotAutoPublish(s.Settings) {
				s.autoPublishTikTokAffiliate(ctx, leg.JobID, j, p, acct)
				notes = append(notes, "thử lại chân đăng TikTok cho job "+leg.JobID)
			}
		case "youtube":
			if AutopilotYouTubeEnabled(s.Settings) {
				s.autoPublishYouTubeAffiliate(ctx, leg.JobID, j, p, acct)
				notes = append(notes, "thử lại chân đăng YouTube cho job "+leg.JobID)
			}
		case "facebook", "instagram", "threads":
			if AutopilotMetaEnabled(s.Settings) {
				s.autoPublishMetaAffiliatePlatform(ctx, leg.JobID, j, p, acct, leg.Platform)
				notes = append(notes, "thử lại chân đăng Meta "+leg.Platform+" cho job "+leg.JobID)
			}
		}
	}
	return notes
}

// ------------------------------------------------------ auto-publish

// AutoPublishAffiliate posts a finished autopilot affiliate video to
// TikTok (the Studio SetOnDone hook calls this in the background).
// The toggle defaults ON when unset (Đợt 3); a stored "0" opts out.
// Fail-closed at every step: kill switch / DRY-RUN, non-affiliate jobs,
// manual jobs (no AccountID), disabled toggle, and missing TikTok OAuth
// all skip quietly with a job-log line. TikTok posts as draft by default,
// so Ninh/Claude attaches the product link in the TikTok app before going
// public.
func (s *Service) AutoPublishAffiliate(ctx context.Context, jobID string) {
	// Đợt 3: the shared kill switch and DRY-RUN gate every real publish.
	if s.killed() || s.dryRun() {
		return
	}
	if s.JobStore == nil {
		return
	}
	j, ok := s.JobStore.GetJob(jobID)
	if !ok || j.Kind != studio.KindAffiliate || j.Status != studio.StatusDone || j.Output == "" {
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil || p.AccountID == 0 {
		return // manual studio job — never auto-publish
	}
	if s.Accounts == nil {
		return
	}
	acct, err := s.Accounts.Get(p.AccountID)
	if err != nil {
		s.JobStore.AppendLog(jobID, "Tự đăng: không tìm thấy account — bỏ qua.")
		return
	}
	// TikTok: nháp (giới hạn nền tảng — gắn giỏ hàng tay trong app).
	if AutopilotAutoPublish(s.Settings) {
		s.autoPublishTikTokAffiliate(ctx, jobID, j, p, acct)
	}
	// Đợt Q: YouTube Shorts theo YOUTUBE_DEFAULT_PRIVACY — kênh phân phối thứ
	// hai, tự động (không cần thao tác trong app như TikTok draft).
	if AutopilotYouTubeEnabled(s.Settings) {
		s.autoPublishYouTubeAffiliate(ctx, jobID, j, p, acct)
	}
	// Đăng chéo Meta (Facebook Page nháp · Instagram Reels · Threads bài
	// chữ kèm link). Chỉ nền tảng đã cấu hình key mới chạy.
	if AutopilotMetaEnabled(s.Settings) {
		s.autoPublishMetaAffiliate(ctx, jobID, j, p, acct)
	}
}

// metaPublishersFor là seam để test inject publisher giả.
func (s *Service) AutoPublishHook() func(id string) {
	return func(id string) {
		go s.AutoPublishAffiliate(context.Background(), id)
	}
}

// Compile-time boundary checks: *network.AccountManager satisfies
// AccountManager, *studio.Autopilot satisfies AutopilotRunner.
var (
	_ AccountManager  = (*network.AccountManager)(nil)
	_ AutopilotRunner = (*studio.Autopilot)(nil)
	_ JobStore        = (*studio.Studio)(nil)
)

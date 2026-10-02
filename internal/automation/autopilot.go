package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ------------------------------------------------- scheduler boundaries

// JobStore is the studio-job boundary the autopilot needs.
// *studio.Studio satisfies it.
type JobStore interface {
	GetJob(id string) (studio.Job, bool)
	AppendLog(id, line string)
}

// ----------------------------------------------------------- the tick

// AutopilotTick runs one scheduled affiliate cycle: theme ->
// high-commission product -> video, hands-off. Returns log notes; the
// Studio's SetOnDone hook handles publishing separately. Cadence:
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
		}
	}
	if s.Settings != nil {
		_ = s.Settings.Set("autopilot_last_run", time.Now().Format(time.RFC3339))
	}
	return []string{fmt.Sprintf("autopilot: chu kỳ đã xong (%d video đã xếp)", done)}
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
	if !AutopilotAutoPublish(s.Settings) {
		return
	}
	if s.Accounts == nil {
		return
	}
	acct, err := s.Accounts.Get(p.AccountID)
	if err != nil {
		s.JobStore.AppendLog(jobID, "Tự đăng: không tìm thấy account — bỏ qua.")
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
	res := pub.Publish(ctx, j.Output, title, j.Caption, "short_video")
	if !res.Ok {
		s.JobStore.AppendLog(jobID, "Tự đăng thất bại: "+res.Error)
		_ = s.decide("autopilot", "autopublish_failed", &jobID,
			"tự đăng TikTok thất bại: "+res.Error, nil)
		return
	}
	if res.Draft {
		s.JobStore.AppendLog(jobID, "Đã đăng lên TikTok dưới dạng NHÁP (draft id "+res.RemoteID+") — gắn giỏ hàng + sound trong app TikTok rồi hãy public.")
	} else {
		s.JobStore.AppendLog(jobID, "Đã đăng TikTok (id "+res.RemoteID+").")
	}
}

// AutoPublishLoggable wraps SetOnDone wiring: the closure form main.go
// used, now pointing at the service.
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

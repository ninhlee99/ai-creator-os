package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strings"
	"time"
)

// ----------------------------------------------------------- the tick

// AutopilotTick attempts one scheduled affiliate cycle. The current Studio
// runner stops before product discovery until per-account Google Flow credit
// authorization exists. Returns log notes. TikTok handoff stays operator-
// triggered after per-video review. Cadence:
// the UI-managed interval (default 6h); gates: kill switch, DRY-RUN,
// the autopilot_enabled switch (default ON when unset; a stored "0" opts
// out). The current producer may still stop before discovery at its
// per-account production authorization gate.
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
	if s.Settings != nil {
		_ = s.Settings.Set("autopilot_last_run", time.Now().Format(time.RFC3339))
	}
	notes := []string{fmt.Sprintf("autopilot: chu kỳ đã xong (%d video đã xếp)", done)}
	return notes
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

// Compile-time boundary checks: *network.AccountManager satisfies
// AccountManager, *studio.Autopilot satisfies AutopilotRunner.
var (
	_ AccountManager  = (*network.AccountManager)(nil)
	_ AutopilotRunner = (*studio.Autopilot)(nil)
)

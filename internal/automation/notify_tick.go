package automation

// Đợt O2 — Telegram: "máy gọi Ninh".
//
// Alert warn/critical mới (đĩa đầy, job treo, nguồn reup chết, sao lưu
// lỗi...) được gửi tới Telegram của Ninh 1 giờ/lần. Chỉ gửi khi Ninh đã
// bật + nhập đủ bot token/chat ID ở Cài đặt · Hệ thống; chưa cấu hình thì
// im lặng (fail-closed). Watermark notify.telegram_last_id đảm bảo mỗi
// alert chỉ gửi 1 lần.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ninhlee99/ai-creator-os/internal/notify"
)

const (
	KeyNotifyTelegramOn     = "notify.telegram_on"      // "1"/"0", mặc định tắt
	KeyNotifyTelegramBot    = "notify.telegram_bot"     // bot token (kín)
	KeyNotifyTelegramChat   = "notify.telegram_chat"    // chat ID
	KeyNotifyTelegramLastID = "notify.telegram_last_id" // watermark alert đã gửi
)

// NotifyTick gửi alert mới qua Telegram. Tôn trọng kill switch + dry-run.
func (s *Service) NotifyTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.Growth == nil || s.Settings == nil {
		return nil
	}
	if !atOn(s.Settings, KeyNotifyTelegramOn, false) {
		return nil
	}
	bot, _ := s.Settings.Get(KeyNotifyTelegramBot)
	chat, _ := s.Settings.Get(KeyNotifyTelegramChat)
	if bot == "" || chat == "" {
		return nil // chưa nhập đủ → im lặng, không spam log
	}
	alerts, err := s.Growth.ListAlerts(0, 20)
	if err != nil {
		return nil
	}
	lastID := int64(atInt(s.Settings, KeyNotifyTelegramLastID, 0))
	var maxID = lastID
	var sent int
	for _, a := range alerts {
		if a.ID <= lastID {
			continue
		}
		if a.ID > maxID {
			maxID = a.ID
		}
		if a.Severity != "warn" && a.Severity != "critical" {
			continue // info → chỉ xem trên dashboard, không làm phiền
		}
		msg := fmt.Sprintf("⚠️ [%s] %s", a.Kind, a.Message)
		if a.ActionTaken != "" {
			msg += "\nĐã xử lý: " + a.ActionTaken
		}
		if err := notify.SendTelegram(bot, chat, msg); err != nil {
			return []string{fmt.Sprintf("telegram: gửi thất bại — %v", err)}
		}
		sent++
	}
	if maxID > lastID {
		_ = s.Settings.Set(KeyNotifyTelegramLastID, strconv.FormatInt(maxID, 10))
	}
	if sent > 0 {
		return []string{fmt.Sprintf("telegram: đã gửi %d cảnh báo", sent)}
	}
	return nil
}

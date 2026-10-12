package automation

// Đợt O2 — Telegram: "máy gọi Ninh".
//
// Alert warn/critical mới (đĩa đầy, job treo, nguồn reup chết, sao lưu
// lỗi...) được gửi tới Telegram của Ninh 1 giờ/lần. Chỉ gửi khi Ninh đã
// bật + nhập đủ bot token/chat ID ở Cài đặt · Hệ thống; chưa cấu hình thì
// im lặng (fail-closed). Watermark notify.telegram_last_id đảm bảo mỗi
// alert chỉ gửi 1 lần.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
)

const (
	KeyNotifyTelegramOn     = "notify.telegram_on"      // "1"/"0", mặc định tắt
	KeyNotifyTelegramBot    = "notify.telegram_bot"     // bot token (kín)
	KeyNotifyTelegramChat   = "notify.telegram_chat"    // chat ID
	KeyNotifyTelegramLastID = "notify.telegram_last_id" // watermark alert đã gửi
	// Đợt V: webhook dự phòng — khi Telegram gửi lỗi, POST JSON cảnh báo
	// tới URL này (kênh thứ hai, khỏi "mù" alert).
	KeyNotifyWebhookURL = "notify.webhook_url"
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
	var sent, viaWebhook int
	for _, a := range alerts {
		if a.ID <= lastID {
			continue
		}
		if a.Severity != "warn" && a.Severity != "critical" {
			continue // info → chỉ xem trên dashboard, không làm phiền
		}
		msg := fmt.Sprintf("⚠️ [%s] %s", a.Kind, a.Message)
		if a.ActionTaken != "" {
			msg += "\nĐã xử lý: " + a.ActionTaken
		}
		delivered := false
		if err := notify.SendTelegram(bot, chat, msg); err != nil {
			// Đợt V: Telegram lỗi → thử webhook dự phòng (nếu có).
			if wherr := s.sendWebhookFallback(a, msg); wherr == nil && s.webhookConfigured() {
				viaWebhook++
				delivered = true
			} else if wherr != nil {
				// Cả hai kênh đều lỗi → DỪNG, không advance watermark:
				// tick sau thử lại alert này, các alert đã gửi xong không bị gửi trùng.
				return []string{fmt.Sprintf("telegram: gửi thất bại (alert #%d) — %v; webhook cũng lỗi: %v", a.ID, err, wherr)}
			} else {
				// Không có webhook → dừng như cũ, tick sau thử lại.
				return []string{fmt.Sprintf("telegram: gửi thất bại (alert #%d) — %v", a.ID, err)}
			}
		} else {
			sent++
			delivered = true
		}
		if delivered {
			// Đợt V: advance watermark theo TỪNG alert đã tới tay — 1 alert
			// lỗi giữa chừng không làm gửi trùng cả batch ở tick sau.
			_ = s.Settings.Set(KeyNotifyTelegramLastID, strconv.FormatInt(a.ID, 10))
		}
	}
	if sent > 0 || viaWebhook > 0 {
		note := fmt.Sprintf("telegram: đã gửi %d cảnh báo", sent)
		if viaWebhook > 0 {
			note += fmt.Sprintf(" (+%d qua webhook dự phòng)", viaWebhook)
		}
		return []string{note}
	}
	return nil
}

// webhookConfigured báo đã cấu hình URL webhook dự phòng chưa.
func (s *Service) webhookConfigured() bool {
	if s.Settings == nil {
		return false
	}
	whURL, _ := s.Settings.Get(KeyNotifyWebhookURL)
	return strings.TrimSpace(whURL) != ""
}

// sendWebhookFallback POST cảnh báo tới webhook dự phòng (nếu đã cấu hình).
// Trả về nil khi gửi thành công HOẶC khi không cấu hình (caller phân biệt
// bằng webhookConfigured).
func (s *Service) sendWebhookFallback(a growth.Alert, msg string) error {
	if !s.webhookConfigured() {
		return nil
	}
	whURL, _ := s.Settings.Get(KeyNotifyWebhookURL)
	payload := map[string]string{
		"severity": a.Severity,
		"kind":     a.Kind,
		"message":  msg,
		"source":   "aicos",
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSpace(whURL), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d", resp.StatusCode)
	}
	return nil
}

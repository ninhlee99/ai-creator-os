package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
	"net/http"
	"net/url"
	"strings"
)

// handleNotifySave lưu cấu hình Telegram (bot token, chat ID, bật/tắt).
func (s *Server) handleNotifySave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse notify form")
		return
	}
	set := func(k, v string) {
		_ = s.Ledger.SetSetting(k, v)
	}
	set(automation.KeyNotifyTelegramOn, map[bool]string{true: "1", false: "0"}[r.PostFormValue("notify_on") == "1"])
	if bot := strings.TrimSpace(r.PostFormValue("notify_bot")); bot != "" {
		set(automation.KeyNotifyTelegramBot, bot) // chỉ ghi đè khi nhập mới
	}
	set(automation.KeyNotifyTelegramChat, strings.TrimSpace(r.PostFormValue("notify_chat")))
	set(automation.KeyNotifyWebhookURL, strings.TrimSpace(r.PostFormValue("notify_webhook")))
	_ = s.Ledger.Decide("human", "notify_save", nil, "Đã lưu cấu hình thông báo Telegram.", map[string]any{})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã lưu cấu hình Telegram."))
}

// handleNotifyTest gửi tin nhắn thử qua Telegram với cấu hình hiện tại.
func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	bot, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
	chat, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramChat)
	if err := notify.SendTelegram(bot, chat, "✅ MiuNova đã nối Telegram. Từ giờ cảnh báo sẽ gửi về đây."); err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Gửi thử thất bại: "+err.Error()))
		return
	}
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã gửi tin nhắn thử — kiểm tra Telegram."))
}

// handleNotifyDiscordTest gửi tin nhắn thử qua Discord Webhook.
func (s *Server) handleNotifyDiscordTest(w http.ResponseWriter, r *http.Request) {
	webhook, _, _ := s.Ledger.GetSetting(automation.KeyNotifyWebhookURL)
	if strings.TrimSpace(webhook) == "" {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa cấu hình Discord Webhook URL."))
		return
	}
	if err := notify.SendDiscord(webhook, "🔔 Kiểm tra Discord Webhook", "✅ Kết nối thành công! MiuNova đã liên kết với kênh Discord này. Từ giờ các sự kiện render video, xuất bản và cảnh báo sẽ gửi về đây."); err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Gửi Discord thất bại: "+err.Error()))
		return
	}
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã gửi tin nhắn thử thành công tới Discord!"))
}

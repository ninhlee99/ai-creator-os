package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SendDiscord gửi thông báo tới một kênh Discord qua Incoming Webhook.
// webhookURL/text trống -> báo lỗi rõ ràng (fail-closed).
func SendDiscord(webhookURL, title, text string) error {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return fmt.Errorf("chưa cấu hình Discord (thiếu webhook URL)")
	}
	text = strings.TrimSpace(text)
	if text == "" && strings.TrimSpace(title) == "" {
		return fmt.Errorf("nội dung thông báo trống")
	}

	if r := []rune(text); len(r) > 4000 {
		text = string(r[:4000]) + "…"
	}

	payload := map[string]any{
		"username": "AI Creator OS",
	}

	if title != "" {
		payload["embeds"] = []map[string]any{
			{
				"title":       title,
				"description": text,
				"color":       3447003, // Blue
			},
		}
	} else {
		payload["content"] = text
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mã hóa webhook discord: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tạo yêu cầu discord: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gửi discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord từ chối: HTTP %d", resp.StatusCode)
	}
	return nil
}

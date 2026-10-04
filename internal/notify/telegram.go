// Package notify gửi thông báo ra ngoài app (Đợt O2).
//
// Máy chạy 24/7 không người trông: alert chỉ nằm trên dashboard thì Ninh
// phải nhớ mở app mới thấy. Telegram là kênh "máy gọi Ninh" khi có việc
// cần biết (đĩa đầy, job treo, nguồn reup chết...). Pure Go, không cgo,
// không dependency mới.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// apiBase cho phép test trỏ vào httptest server.
var apiBase = "https://api.telegram.org"

// SendTelegram gửi tin nhắn text tới 1 chat qua Bot API.
// botToken/chatID trống → lỗi rõ ràng (fail-closed, không gửi lén đi đâu).
func SendTelegram(botToken, chatID, text string) error {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	if botToken == "" || chatID == "" {
		return fmt.Errorf("chưa cấu hình Telegram (thiếu bot token hoặc chat ID)")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("tin nhắn trống")
	}
	// Telegram giới hạn 4096 ký tự — cắt an toàn theo rune.
	if r := []rune(text); len(r) > 4000 {
		text = string(r[:4000]) + "…"
	}
	body, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    text,
	})
	req, err := http.NewRequest("POST",
		apiBase+"/bot"+botToken+"/sendMessage",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gửi telegram: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("đọc phản hồi telegram: %w", err)
	}
	if !out.OK {
		if out.Description == "" {
			out.Description = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return fmt.Errorf("telegram từ chối: %s", out.Description)
	}
	return nil
}

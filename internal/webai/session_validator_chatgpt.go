package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type chatGPTSessionValidator struct{}

func (chatGPTSessionValidator) target() WebSessionTarget {
	return WebSessionTarget{Platform: PlatformChatGPT, Name: "ChatGPT"}
}

func (chatGPTSessionValidator) validateSession(ctx context.Context, client *http.Client, auth *WebSessionAuth) error {
	body, err := webSessionProbeCaptured(ctx, client, auth, "https://chatgpt.com/api/auth/session", "chatgpt.com", strings.TrimSpace(auth.CookieHeader), "application/json", nil)
	if err != nil {
		return fmt.Errorf("ChatGPT chưa xác thực cookie: %w", err)
	}
	var data struct {
		User *struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &data) != nil || data.User == nil || (strings.TrimSpace(data.User.ID) == "" && strings.TrimSpace(data.User.Email) == "") {
		return fmt.Errorf("ChatGPT chưa trả về người dùng đã xác thực")
	}
	if email := accountEmail(data.User.Email); email != "" {
		auth.Email = email
	}
	return nil
}

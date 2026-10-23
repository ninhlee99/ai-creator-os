package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type grokSessionValidator struct{}

func (grokSessionValidator) target() WebSessionTarget {
	return WebSessionTarget{Platform: PlatformGrok, Name: "Grok (xAI)"}
}

func (grokSessionValidator) validateSession(ctx context.Context, client *http.Client, auth *WebSessionAuth) error {
	body, err := webSessionProbeCaptured(ctx, client, auth, "https://grok.com/rest/auth/user", "grok.com", strings.TrimSpace(auth.CookieHeader), "application/json", nil)
	if err != nil {
		return fmt.Errorf("Grok chưa xác thực cookie: %w", err)
	}
	var data struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		ID       string `json:"id"`
		UserID   string `json:"user_id"`
		User     struct {
			Email    string `json:"email"`
			Username string `json:"username"`
			ID       string `json:"id"`
			UserID   string `json:"user_id"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &data) != nil {
		return fmt.Errorf("Grok chưa trả về danh tính tài khoản")
	}
	email := firstAccountEmail(data.Email, data.User.Email)
	identity := firstNonEmpty(data.Username, data.ID, data.UserID, data.User.Username, data.User.ID, data.User.UserID)
	if email == "" && identity == "" {
		return fmt.Errorf("Grok chưa trả về người dùng đã xác thực")
	}
	if email != "" {
		if savedEmail := accountEmail(auth.Email); savedEmail != "" && savedEmail != email {
			return fmt.Errorf("email Grok từ cookie không khớp account đã lưu")
		}
		auth.Email = email
	}
	return nil
}

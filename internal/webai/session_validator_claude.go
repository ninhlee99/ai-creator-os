package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type claudeSessionValidator struct{}

func (claudeSessionValidator) target() WebSessionTarget {
	return WebSessionTarget{Platform: PlatformClaude, Name: "Claude"}
}

func (claudeSessionValidator) validateSession(ctx context.Context, client *http.Client, auth *WebSessionAuth) error {
	cookie := strings.TrimSpace(auth.CookieHeader)
	if !strings.Contains(cookie, "sessionKey=") && auth.SessionValue != "" {
		cookie = "sessionKey=" + auth.SessionValue + "; " + cookie
	}
	headers := map[string]string{
		"Origin":                    "https://claude.ai",
		"Referer":                   "https://claude.ai/",
		"anthropic-client-platform": "web_claude_ai",
	}
	body, err := webSessionProbeCaptured(ctx, client, auth, "https://claude.ai/api/account", "claude.ai", cookie, "application/json", headers)
	if err != nil {
		return fmt.Errorf("Claude chưa xác thực cookie: %w", err)
	}
	var data struct {
		Email        string `json:"email"`
		EmailAddress string `json:"email_address"`
		UUID         string `json:"uuid"`
		ID           string `json:"id"`
		Account      struct {
			Email        string `json:"email"`
			EmailAddress string `json:"email_address"`
			UUID         string `json:"uuid"`
			ID           string `json:"id"`
		} `json:"account"`
	}
	if json.Unmarshal(body, &data) != nil {
		return fmt.Errorf("Claude chưa trả về danh tính tài khoản")
	}
	email := firstAccountEmail(data.Email, data.EmailAddress, data.Account.Email, data.Account.EmailAddress)
	identity := firstNonEmpty(data.UUID, data.ID, data.Account.UUID, data.Account.ID)
	if email == "" && identity == "" {
		return fmt.Errorf("Claude chưa trả về người dùng đã xác thực")
	}
	if email != "" {
		auth.Email = email
	}
	return nil
}

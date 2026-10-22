package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type claudeLoginAdapter struct{}

func (claudeLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformClaude,
		Name:        "Claude",
		StartURL:    "https://claude.ai/login",
		CookieHost:  "claude.ai",
		CookieName:  "sessionKey",
		Profile:     "claude",
		LandingHost: "claude.ai",
	}
}

func (claudeLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (claudeLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := strings.TrimSpace(auth.CookieHeader)
	if !strings.Contains(cookie, "sessionKey=") && auth.SessionValue != "" {
		cookie = "sessionKey=" + auth.SessionValue + "; " + cookie
	}
	headers := map[string]string{
		"Origin":                    "https://claude.ai",
		"Referer":                   "https://claude.ai/",
		"anthropic-client-platform": "web_claude_ai",
	}
	body, err := webLoginProbe(ctx, client, "https://claude.ai/api/account", "claude.ai", cookie, "application/json", headers)
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

func (claudeLoginAdapter) matchesIdentityURL(rawURL string) bool {
	return strings.Contains(rawURL, "claude.ai") && !strings.Contains(rawURL, "claude.ai/login")
}

func (claudeLoginAdapter) identityScript() string {
	return `(() => {
			try {
				const els = document.querySelectorAll('[aria-label*="@"], [data-testid*="user"]');
				for (const el of els) {
					const txt = el.getAttribute('aria-label') || el.textContent;
					const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m) return m[1];
				}
			} catch(e) {}
			return "";
		})()`
}

func (adapter claudeLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, _ := loginCookieEmailHints(cookies, target)
	return latestLoginCookieValue(cookies, target, target.CookieName), email
}

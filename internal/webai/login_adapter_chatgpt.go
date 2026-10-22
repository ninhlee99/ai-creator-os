package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type chatGPTLoginAdapter struct{}

func (chatGPTLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformChatGPT,
		Name:        "ChatGPT",
		StartURL:    "https://chatgpt.com/auth/login",
		CookieHost:  "chatgpt.com",
		CookieName:  "__Secure-next-auth.session-token",
		Profile:     "chatgpt",
		LandingHost: "chatgpt.com",
	}
}

func (chatGPTLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (chatGPTLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := strings.TrimSpace(auth.CookieHeader)
	body, err := webLoginProbe(ctx, client, "https://chatgpt.com/api/auth/session", "chatgpt.com", cookie, "application/json", nil)
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

func (chatGPTLoginAdapter) matchesIdentityURL(rawURL string) bool {
	return strings.Contains(rawURL, "chatgpt.com") && !strings.Contains(rawURL, "auth/login")
}

func (chatGPTLoginAdapter) identityScript() string {
	return `(() => {
			try {
				if (window.__NEXT_DATA__ && window.__NEXT_DATA__.props) {
					const str = JSON.stringify(window.__NEXT_DATA__.props);
					const m = str.match(/"email"\s*:\s*"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})"/);
					if (m) return m[1];
				}
			} catch(e) {}
			try {
				const els = document.querySelectorAll('[aria-label*="@"]');
				for (const el of els) {
					const m = el.getAttribute('aria-label').match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m) return m[1];
				}
			} catch(e) {}
			return "";
		})()`
}

func (adapter chatGPTLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, _ := loginCookieEmailHints(cookies, target)
	session := latestLoginCookieValue(cookies, target, target.CookieName)
	if session == "" && strings.Contains(target.CookieName, "session-token") {
		session = chunkedNextAuthSession(cookies, target)
	}
	return session, email
}

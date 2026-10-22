package webai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type grokLoginAdapter struct{}

func (grokLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformGrok,
		Name:        "Grok (xAI)",
		StartURL:    "https://grok.com/",
		CookieHost:  "grok.com",
		CookieName:  "sso",
		Profile:     "grok",
		LandingHost: "grok.com",
	}
}

func (grokLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (grokLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := strings.TrimSpace(auth.CookieHeader)
	body, err := webLoginProbe(ctx, client, "https://grok.com/rest/auth/user", "grok.com", cookie, "application/json", nil)
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
		auth.Email = email
	}
	return nil
}

func (grokLoginAdapter) matchesIdentityURL(rawURL string) bool {
	return strings.Contains(rawURL, "grok.com")
}

func (grokLoginAdapter) identityScript() string {
	return `(async () => {
			const re = /([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4}))/i;
			try {
				const footerBtn = document.querySelector('[data-sidebar="footer"] button, button[aria-label*="user"], [class*="profile"]');
				if (footerBtn) {
					footerBtn.click();
					await new Promise(r => setTimeout(r, 200));
				}
				const menu = document.querySelector('[role="menu"], [data-radix-popper-content-wrapper]');
				if (menu) {
					for (const el of menu.querySelectorAll('*')) {
						const txt = el.textContent ? el.textContent.trim() : '';
						const m = txt.match(re);
						if (m && !m[1].includes('example') && !m[1].includes('sentry')) {
							document.body.click();
							return m[1];
						}
					}
					const m2 = menu.textContent.match(re);
					document.body.click();
					if (m2 && !m2[1].includes('example') && !m2[1].includes('sentry')) return m2[1];
				}
				document.body.click();
			} catch(e) {}
			try {
				const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
				let node;
				while (node = walker.nextNode()) {
					const val = node.nodeValue ? node.nodeValue.trim() : '';
					const m = val.match(re);
					if (m && !m[1].includes('example') && !m[1].includes('sentry') && !m[1].includes('w3.org') && !m[1].includes('schema.org')) {
						return m[1];
					}
				}
			} catch(e) {}
			try {
				for (let i = 0; i < localStorage.length; i++) {
					const val = localStorage.getItem(localStorage.key(i));
					if (val && val.includes('@')) {
						const m = val.match(re);
						if (m && !m[1].includes('example') && !m[1].includes('sentry') && !m[1].includes('w3.org')) return m[1];
					}
				}
			} catch(e) {}
			return "";
		})()`
}

func (adapter grokLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, _ := loginCookieEmailHints(cookies, target)
	session := latestLoginCookieValue(cookies, target, target.CookieName)
	if session == "" {
		session = firstLoginCookieValue(cookies, target, "sso", "sso-rw", "auth_token")
	}
	if email == "" && !hasGrokUserCookie(cookies) {
		session = ""
	}
	return session, email
}

func hasGrokUserCookie(cookies []cdpCookie) bool {
	for _, cookie := range cookies {
		if (cookie.Name == "x-userid" || cookie.Name == "htjs_user_id") && cookie.Value != "" {
			return true
		}
	}
	return false
}

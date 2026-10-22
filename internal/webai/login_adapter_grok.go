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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if grokBrowserIdentityVerified(auth) {
			return nil
		}
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
		if grokBrowserIdentityVerified(auth) {
			return nil
		}
		return fmt.Errorf("Grok chưa trả về danh tính tài khoản")
	}
	email := firstAccountEmail(data.Email, data.User.Email)
	identity := firstNonEmpty(data.Username, data.ID, data.UserID, data.User.Username, data.User.ID, data.User.UserID)
	if email == "" && identity == "" {
		if grokBrowserIdentityVerified(auth) {
			return nil
		}
		return fmt.Errorf("Grok chưa trả về người dùng đã xác thực")
	}
	if email != "" {
		if capturedEmail := accountEmail(auth.Email); capturedEmail != "" && capturedEmail != email {
			return fmt.Errorf("email Grok từ cookie không khớp danh tính đang mở trong trình duyệt")
		}
		auth.Email = email
	}
	return nil
}

func grokBrowserIdentityVerified(auth *CapturedWebAuth) bool {
	return auth != nil && auth.BrowserIdentityVerified && accountEmail(auth.Email) != ""
}

func (grokLoginAdapter) matchesIdentityURL(rawURL string) bool {
	return strings.Contains(rawURL, "grok.com")
}

func (grokLoginAdapter) identityScript() string {
	return `(async () => {
			const re = /([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4}))/i;
			const menuSelector = '[role="menu"], [data-radix-popper-content-wrapper]';
			const findEmail = menu => {
				if (!menu) return '';
				const match = (menu.innerText || menu.textContent || '').match(re);
				if (!match) return '';
				const email = match[1].toLowerCase();
				return email.includes('example') || email.includes('sentry') ? '' : email;
			};
			let menu = Array.from(document.querySelectorAll(menuSelector)).find(el => el.getClientRects().length);
			let openedHere = false;
			if (!menu) {
				const trigger = document.querySelector('[data-sidebar="footer"] button, button[aria-label*="user" i], button[aria-label*="profile" i], [class*="profile"] button');
				if (trigger) {
					trigger.click();
					openedHere = true;
					await new Promise(resolve => setTimeout(resolve, 250));
					menu = Array.from(document.querySelectorAll(menuSelector)).find(el => el.getClientRects().length);
				}
			}
			const email = findEmail(menu);
			if (openedHere) {
				const trigger = document.querySelector('[data-sidebar="footer"] button, button[aria-label*="user" i], button[aria-label*="profile" i], [class*="profile"] button');
				if (trigger) trigger.click();
			}
			if (email) return email;
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
	return session, email
}

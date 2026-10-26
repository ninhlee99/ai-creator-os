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
		// When Go http.Client gets HTTP 403 or redirect block due to Cloudflare Bot Management on server,
		// but the browser has already captured the session cookie and valid identity:
		if (strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "status 403")) && auth.SessionValue != "" {
			if strings.TrimSpace(auth.Email) != "" {
				return nil
			}
			return fmt.Errorf("đang đọc danh tính tài khoản Grok từ trình duyệt...")
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
		if auth.SessionValue != "" && auth.Email != "" {
			return nil
		}
		return fmt.Errorf("Grok chưa trả về danh tính tài khoản")
	}
	email := firstAccountEmail(data.Email, data.User.Email)
	identity := firstNonEmpty(data.Username, data.ID, data.UserID, data.User.Username, data.User.ID, data.User.UserID)
	if email == "" && identity == "" && auth.Email == "" {
		return fmt.Errorf("Grok chưa trả về người dùng đã xác thực")
	}
	if email != "" {
		if savedEmail := accountEmail(auth.Email); savedEmail != "" && savedEmail != email {
			return fmt.Errorf("email Grok từ cookie không khớp account đã lưu")
		}
		auth.Email = email
	} else if identity != "" && auth.Email == "" {
		if !strings.Contains(identity, "@") {
			identity = strings.TrimPrefix(identity, "@") + "@grok.com"
		}
		auth.Email = identity
	}
	return nil
}

func grokIdentityScript() string {
	return `(async function() {
		const re = /([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4}))/i;
		const isReal = (em) => {
			if (!em) return false;
			const low = em.toLowerCase();
			return !low.includes('example') && !low.includes('sentry') && !low.includes('w3.org') && !low.startsWith('support@');
		};

		// 1. Try same-origin fetch inside browser context
		try {
			const res = await fetch('/rest/auth/user', { credentials: 'include', headers: { 'Accept': 'application/json' } });
			if (res.ok) {
				const data = await res.json();
				const em = (data.email || (data.user && data.user.email) || '').trim();
				if (em && isReal(em)) return em.toLowerCase();
				const un = (data.username || (data.user && data.user.username) || data.id || (data.user && data.user.id) || '').trim();
				if (un && un.includes('@')) return un.toLowerCase();
			}
		} catch(e) {}

		// 2. Check if email is already rendered in open popover or sidebar
		try {
			for (const el of document.querySelectorAll('[role="menu"], [data-radix-popper-content-wrapper], [data-sidebar="footer"], [data-testid*="user" i], [data-testid*="profile" i]')) {
				const m = (el.innerText || el.textContent || '').match(re);
				if (m && isReal(m[1])) return m[1].toLowerCase();
			}
		} catch(e) {}

		// 3. Trigger profile menu click to mount Radix popper if not open
		try {
			const triggers = document.querySelectorAll('[data-sidebar="footer"] button, button[aria-label*="user" i], button[aria-label*="profile" i], button[aria-haspopup="menu"], [class*="profile"] button');
			for (const trigger of triggers) {
				trigger.click();
				await new Promise(r => setTimeout(r, 250));
				const menus = document.querySelectorAll('[role="menu"], [data-radix-popper-content-wrapper]');
				for (const menu of menus) {
					const m = (menu.innerText || menu.textContent || '').match(re);
					if (m && isReal(m[1])) {
						trigger.click();
						return m[1].toLowerCase();
					}
				}
				trigger.click();
			}
		} catch(e) {}

		// 4. State & Next.js cache
		try {
			for (const key of ['__INITIAL_STATE__', '__NEXT_DATA__', '__PRELOADED_STATE__']) {
				const val = window[key];
				if (val) {
					const str = JSON.stringify(val);
					const m = str.match(re);
					if (m && isReal(m[1])) return m[1].toLowerCase();
				}
			}
		} catch(e) {}

		// 5. LocalStorage & SessionStorage
		try {
			for (const storage of [localStorage, sessionStorage]) {
				for (let i = 0; i < storage.length; i++) {
					const v = storage.getItem(storage.key(i)) || '';
					const m = v.match(re);
					if (m && isReal(m[1])) return m[1].toLowerCase();
				}
			}
		} catch(e) {}

		// 6. Body text fallback
		try {
			const bodyText = document.body ? (document.body.innerText || '') : '';
			const m = bodyText.match(re);
			if (m && isReal(m[1])) return m[1].toLowerCase();
		} catch(e) {}

		return "";
	})()`
}

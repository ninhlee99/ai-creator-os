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
			if strings.TrimSpace(auth.Email) == "" {
				auth.Email = "grok_user@grok.com"
			}
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
		if auth.SessionValue != "" && auth.Email != "" {
			return nil
		}
		return fmt.Errorf("Grok chưa trả về danh tính tài khoản")
	}
	email := firstAccountEmail(data.Email, data.User.Email)
	identity := firstNonEmpty(data.Username, data.ID, data.UserID, data.User.Username, data.User.ID, data.User.UserID)
	if email == "" && identity == "" && auth.Email == "" {
		if auth.SessionValue != "" {
			auth.Email = "grok_user@grok.com"
			return nil
		}
		return fmt.Errorf("Grok chưa trả về người dùng đã xác thực")
	}
	if email != "" {
		if savedEmail := accountEmail(auth.Email); savedEmail != "" && savedEmail != email {
			return fmt.Errorf("email Grok từ cookie không khớp account đã lưu")
		}
		auth.Email = email
	} else if identity != "" {
		if !strings.Contains(identity, "@") {
			identity = strings.TrimPrefix(identity, "@") + "@grok.com"
		}
		auth.Email = identity
	} else if auth.Email == "" {
		auth.Email = "grok_user@grok.com"
	}
	return nil
}

func grokIdentityScript() string {
	return `(async function() {
		try {
			const res = await fetch('/rest/auth/user', { credentials: 'include', headers: { 'Accept': 'application/json' } });
			if (res.ok) {
				const data = await res.json();
				const em = (data.email || (data.user && data.user.email) || '').trim();
				if (em && em.includes('@')) return em;
				const un = (data.username || (data.user && data.user.username) || data.id || (data.user && data.user.id) || '').trim();
				if (un) return un.replace(/^@/, '') + '@grok.com';
			}
		} catch(e) {}
		try {
			for (const key of ['__INITIAL_STATE__', '__NEXT_DATA__', '__PRELOADED_STATE__']) {
				const val = window[key];
				if (val) {
					const str = JSON.stringify(val);
					const em = str.match(/"email"\s*:\s*"([^"]+@[^"]+)"/i);
					if (em && !em[1].includes('example') && !em[1].includes('sentry')) return em[1];
					const un = str.match(/"(?:username|screen_name|handle)"\s*:\s*"([a-zA-Z0-9_]+)"/i);
					if (un && un[1] !== 'undefined' && un[1] !== 'null') return un[1] + '@grok.com';
				}
			}
		} catch(e) {}
		try {
			for (const sel of ['[data-testid*="profile" i]', '[data-testid*="user" i]', 'button[aria-haspopup="menu"]', '[aria-label*="@"]', 'a[href^="/@"]', 'div[class*="user" i]']) {
				const els = document.querySelectorAll(sel);
				for (const el of els) {
					const txt = el.getAttribute('aria-label') || el.innerText || el.textContent || '';
					const em = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (em && !em[1].includes('example') && !em[1].includes('sentry')) return em[1];
					const un = txt.match(/@([a-zA-Z0-9_]{3,30})/);
					if (un) return un[1] + '@grok.com';
				}
			}
		} catch(e) {}
		try {
			for (const storage of [localStorage, sessionStorage]) {
				for (let i = 0; i < storage.length; i++) {
					const v = storage.getItem(storage.key(i)) || '';
					const em = v.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (em && !em[1].includes('sentry') && !em[1].includes('example')) return em[1];
					const un = v.match(/"(?:username|user_id|screen_name)"\s*:\s*"([a-zA-Z0-9_]+)"/i);
					if (un && un[1] !== 'undefined' && un[1] !== 'null') return un[1] + '@grok.com';
				}
			}
		} catch(e) {}
		return "";
	})()`
}

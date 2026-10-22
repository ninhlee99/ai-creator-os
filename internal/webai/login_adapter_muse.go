package webai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type museLoginAdapter struct{}

func (museLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformMuseAI,
		Name:        "Muse AI",
		StartURL:    "https://muse.ai/signin",
		CookieHost:  "muse.ai",
		CookieName:  "hatch_sess",
		Profile:     "muse",
		LandingHost: "muse.ai",
	}
}

func (museLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureMuseLogin(ctx, browser, target, updateStatus)
}

func (museLoginAdapter) validateSession(ctx context.Context, _ *http.Client, auth *CapturedWebAuth) error {
	if err := ProbeMuseSession(ctx, strings.TrimSpace(auth.CookieHeader)); err != nil {
		return fmt.Errorf("Muse chưa xác thực cookie: %w", err)
	}
	return nil
}

func (museLoginAdapter) matchesIdentityURL(string) bool { return false }

func (museLoginAdapter) identityScript() string { return "" }

func (adapter museLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, providerEmail := loginCookieEmailHints(cookies, target)
	if email == "" {
		email = providerEmail
	}

	var session string
	for _, cookie := range cookies {
		if !loginCookieDomainMatches(cookie.Domain, target) {
			continue
		}
		if cookie.Name == target.CookieName && cookie.Value != "" && !isMusePlaceholderCookie(cookie.Value) {
			session = cookie.Value
		}
		if session != "" || isMusePlaceholderCookie(cookie.Value) {
			continue
		}
		if isMuseSessionCookie(cookie.Name) && len(cookie.Value) > 4 {
			session = cookie.Value
		} else if cookie.Name == "ps_l" && len(cookie.Value) > 5 {
			session = cookie.Value
		}
	}
	return session, email
}

func isMusePlaceholderCookie(value string) bool {
	return value == "0" || value == "1" || len(value) < 3
}

func isMuseSessionCookie(name string) bool {
	switch name {
	case "hatch_sess", "hatch_native_auth_token", "c_user", "session", "auth_token", "token", "connect.sid":
		return true
	default:
		return false
	}
}

func cdpExtractMuseEmailFromTargets(targets []string) string {
	script := `(async () => {
		const emailFromText = value => {
			if (typeof value !== 'string') return '';
			const matches = value.match(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g) || [];
			for (const candidate of matches) {
				const email = candidate.trim();
				const low = email.toLowerCase();
				if (low.includes('sentry') || low.includes('example.') || low.includes('w3.org') || low.includes('schema.org') || low.includes('facebook.com') || low.includes('meta.com') || low.includes('support@')) continue;
				return email;
			}
			return '';
		};
		const emailFromObject = (value, depth = 0) => {
			if (!value || typeof value !== 'object' || depth > 6) return '';
			for (const [key, child] of Object.entries(value)) {
				if (/^email(address)?$/i.test(key)) {
					const email = emailFromText(child);
					if (email) return email;
				}
				if (child && typeof child === 'object') {
					const email = emailFromObject(child, depth + 1);
					if (email) return email;
				}
			}
			return '';
		};
		try {
			if ((location.hostname === 'muse.ai' || location.hostname.endsWith('.muse.ai')) &&
				(!window.__aicosMuseAccountLookupAt || Date.now() - window.__aicosMuseAccountLookupAt > 10000)) {
				window.__aicosMuseAccountLookupAt = Date.now();
				const response = await fetch('/api/account', { credentials: 'include' });
				if (response.ok) {
					const payload = await response.json();
					const email = emailFromObject(payload);
					if (email) return email;
				}
			}
		} catch(e) {}
		try {
			for (const key of ['user', '__INITIAL_STATE__', '__NEXT_DATA__', '__NUXT__', '__APOLLO_STATE__']) {
				const email = emailFromObject(window[key]);
				if (email) return email;
			}
		} catch(e) {}
		try {
			for (const storage of [localStorage, sessionStorage]) {
				for (let i = 0; i < storage.length; i++) {
					const storageKey = storage.key(i) || '';
					const key = storageKey.toLowerCase();
					if (!/(auth|user|account|profile|session)/.test(key)) continue;
					const value = storage.getItem(storageKey);
					if (!value || !value.includes('@')) continue;
					try {
						const email = emailFromObject(JSON.parse(value));
						if (email) return email;
					} catch(e) {}
					const keyed = value.match(/"(?:\$?email|emailAddress)"\s*:\s*"([^"]+)"/i);
					if (keyed) {
						const email = emailFromText(keyed[1]);
						if (email) return email;
					}
				}
			}
		} catch(e) {}
		try {
			const selectors = '[data-email], [aria-label*="@"], [title*="@"], a[href^="mailto:"], a[href*="account"], [class*="profile"], [class*="avatar"], button[aria-label*="user" i], [data-testid*="user"], [data-testid*="account"]';
			for (const el of document.querySelectorAll(selectors)) {
				for (const value of [el.getAttribute('data-email'), el.getAttribute('aria-label'), el.getAttribute('title'), el.getAttribute('href'), el.textContent]) {
					const email = emailFromText(value || '');
					if (email) return email;
				}
			}
		} catch(e) {}
		try {
			for (const input of document.querySelectorAll('input[type="email"], input[name*="email" i], input[autocomplete="email"]')) {
				const email = emailFromText(input.value || '');
				if (email) return email;
			}
		} catch(e) {}
		try {
			if (!window.__aicosMuseProfileOpened) {
				const trigger = document.querySelector('button[aria-label*="account" i], button[aria-label*="profile" i], button[aria-label*="user" i], button[class*="avatar" i], [data-testid*="user-menu"], [data-testid*="account-menu"]');
				if (trigger) {
					window.__aicosMuseProfileOpened = true;
					trigger.click();
					await new Promise(resolve => setTimeout(resolve, 300));
					for (const el of document.querySelectorAll('[role="menu"], [role="dialog"], [data-radix-popper-content-wrapper], [class*="profile"], [class*="account"], [data-testid*="user"], [data-testid*="account"]')) {
						const email = emailFromText(el.innerText || el.textContent || '');
						if (email) return email;
					}
				}
			}
		} catch(e) {}
		return '';
	})()`
	for _, targetWS := range targets {
		dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
		conn, _, err := dialer.Dial(targetWS, nil)
		if err != nil {
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		reqID := cdpReqID.Add(1)
		req := map[string]any{
			"id":     reqID,
			"method": "Runtime.evaluate",
			"params": map[string]any{"expression": script, "returnByValue": true, "awaitPromise": true},
		}
		if err := conn.WriteJSON(req); err != nil {
			_ = conn.Close()
			continue
		}
		for {
			var msg struct {
				ID     int64 `json:"id"`
				Result struct {
					Result struct {
						Value any `json:"value"`
					} `json:"result"`
				} `json:"result"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}
			if msg.ID != reqID {
				continue
			}
			if value, ok := msg.Result.Result.Value.(string); ok {
				if email := cleanExtractedEmail(value); email != "" {
					_ = conn.Close()
					return email
				}
			}
			break
		}
		_ = conn.Close()
	}
	return ""
}

func isMuseIdentityProviderHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, root := range []string{"meta.com", "meta.ai", "facebook.com", "instagram.com", "google.com"} {
		if host == root || strings.HasSuffix(host, "."+root) {
			return true
		}
	}
	return false
}

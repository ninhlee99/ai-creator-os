package webai

import (
	"context"
	"net/http"
	"strings"
)

func googleLoginCookie(auth *CapturedWebAuth) string {
	cookie := strings.TrimSpace(auth.CookieHeader)
	if !strings.Contains(cookie, "__Secure-1PSID=") && auth.SessionValue != "" {
		return "__Secure-1PSID=" + auth.SessionValue + "; " + cookie
	}
	return cookie
}

func probeGoogleAccountEmail(ctx context.Context, client *http.Client, cookie string) string {
	for _, probe := range []struct{ url, host string }{
		{"https://myaccount.google.com/", "myaccount.google.com"},
		{"https://gemini.google.com/app", "gemini.google.com"},
	} {
		body, err := webLoginProbe(ctx, client, probe.url, probe.host, cookie, "text/html,application/xhtml+xml", nil)
		if err != nil {
			continue
		}
		if email := accountEmail(parseGeminiAccountEmail(string(body))); email != "" {
			return email
		}
	}
	return ""
}

func googleIdentityScript() string {
	return `(() => {
			try {
				if (window.WIZ_global_data && typeof window.WIZ_global_data.oPEP7c === 'string') {
					const em = window.WIZ_global_data.oPEP7c.trim();
					if (em.includes('@') && !em.endsWith('@google.com')) return em;
				}
			} catch(e) {}
			try {
				const els = document.querySelectorAll('[aria-label*="@"], [alt*="@"], [data-email]');
				for (const el of els) {
					const txt = el.getAttribute('aria-label') || el.getAttribute('alt') || el.getAttribute('data-email') || '';
					const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m && !m[1].endsWith('@google.com')) return m[1];
				}
			} catch(e) {}
			try {
				for (let i = 0; i < localStorage.length; i++) {
					const v = localStorage.getItem(localStorage.key(i)) || '';
					const m = v.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m && !m[1].endsWith('@google.com')) return m[1];
				}
			} catch(e) {}
			return "";
		})()`
}

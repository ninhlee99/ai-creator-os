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

func ProbeGoogleLoginAccountEmail(ctx context.Context, client *http.Client, cookie string) string {
	return probeGoogleLoginAccountEmail(ctx, client, cookie)
}

func probeGoogleLoginAccountEmail(ctx context.Context, client *http.Client, cookie string) string {
	for _, probe := range []struct{ url, host string }{
		{"https://myaccount.google.com/", "myaccount.google.com"},
	} {
		body, err := webLoginProbe(ctx, client, probe.url, probe.host, cookie, "text/html,application/xhtml+xml", nil)
		if err != nil {
			continue
		}
		if email := accountEmail(parseGoogleAccountEmail(string(body))); email != "" {
			return email
		}
	}
	return ""
}

func googleIdentityScript() string {
	return `(() => {
			const isRealEmail = (em) => {
				if (!em || typeof em !== 'string') return false;
				const low = em.toLowerCase().trim();
				return low.includes('@') && !low.includes('sentry') && !low.includes('example') && !low.includes('w3.org') && !low.startsWith('support@');
			};
			const extract = (txt) => {
				if (!txt || typeof txt !== 'string') return '';
				const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
				return (m && isRealEmail(m[1])) ? m[1] : '';
			};

			try {
				if (window.WIZ_global_data) {
					for (const k in window.WIZ_global_data) {
						const em = extract(window.WIZ_global_data[k]);
						if (em) return em;
					}
				}
			} catch(e) {}

			try {
				const els = document.querySelectorAll('[aria-label*="@"], [alt*="@"], [data-email], [title*="@"], a[href*="SignOutOptions"], a[href*="accounts.google.com"], [data-identifier]');
				for (const el of els) {
					for (const attr of ['data-email', 'data-identifier', 'aria-label', 'alt', 'title', 'href']) {
						const em = extract(el.getAttribute(attr));
						if (em) return em;
					}
					const em = extract(el.innerText || el.textContent || '');
					if (em) return em;
				}
			} catch(e) {}

			try {
				for (const storage of [localStorage, sessionStorage]) {
					for (let i = 0; i < storage.length; i++) {
						const em = extract(storage.getItem(storage.key(i)) || '');
						if (em) return em;
					}
				}
			} catch(e) {}

			try {
				const bodyText = document.body ? (document.body.innerText || '') : '';
				const gm = bodyText.match(/([a-zA-Z0-9._%+-]+@(gmail|googlemail)\.com)/i);
				if (gm && isRealEmail(gm[1])) return gm[1];
			} catch(e) {}

			return "";
		})()`
}

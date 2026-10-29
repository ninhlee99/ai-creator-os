package webai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tikTokLoginAdapter struct{}

func (tikTokLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformTikTok,
		Name:        "TikTok",
		StartURL:    "https://www.tiktok.com/login",
		CookieHost:  "tiktok.com",
		CookieName:  "sessionid",
		LandingHost: "tiktok.com",
	}
}

func (tikTokLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureTikTokLogin(ctx, browser, target, updateStatus)
}

func captureTikTokLogin(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	landingSeenCount := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		cookies, err := cdpListCookies(browser.wsURL)
		if err == nil && len(cookies) > 0 {
			session, identity := pickTikTokSessionAndIdentity(cookies)
			if session != "" && cdpHasLandingHost(browser.port, target.LandingHost) {
				landingSeenCount++
				liveIdentity := cdpExtractPageIdentity(browser.port, target)
				if liveIdentity != "" {
					identity = liveIdentity
				}
				if identity == "" {
					identity = "TikTok User"
				}
				auth := &CapturedWebAuth{
					Platform:     target.Platform,
					SessionValue: session,
					CookieHeader: cookieHeaderForTarget(cookies, target),
					CookieJar:    loginCookieJarForTarget(cookies, target),
					Email:        identity,
					BrowserUsed:  browser.binary,
				}
				if err := validateCapturedWebAuth(ctx, target, auth); err == nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return auth, nil
				}
				if updateStatus != nil {
					message := "Đã nhận cookie TikTok. Đang xác minh phiên làm việc..."
					updateStatus("waiting_for_user", message)
				}
			}
		}

		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func pickTikTokSessionAndIdentity(cookies []cdpCookie) (session, identity string) {
	for _, c := range cookies {
		if !strings.Contains(c.Domain, "tiktok.com") {
			continue
		}
		if (c.Name == "sessionid" || c.Name == "sessionid_ss" || c.Name == "sid_guard" || c.Name == "sid_tt") && len(c.Value) > 6 {
			if session == "" || c.Name == "sessionid" || c.Name == "sessionid_ss" {
				session = c.Value
			}
		}
		if (c.Name == "uid_tt" || c.Name == "uid_tt_ss") && c.Value != "" && identity == "" {
			identity = "uid_" + c.Value
		}
	}
	return session, identity
}

func (tikTokLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	raw := strings.TrimSpace(auth.CookieHeader)
	if !strings.Contains(raw, "sessionid=") && !strings.Contains(raw, "sessionid_ss=") && !strings.Contains(raw, "sid_guard=") && !strings.Contains(raw, "sid_tt=") {
		return fmt.Errorf("chưa tìm thấy cookie sessionid của TikTok")
	}
	return nil
}

func (tikTokLoginAdapter) matchesIdentityURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com")
}

func (tikTokLoginAdapter) identityScript() string {
	return `(() => {
		try {
			// 1. Trích xuất username từ global state TikTok
			if (window.SIGI_STATE && window.SIGI_STATE.AppContext && window.SIGI_STATE.AppContext.user && window.SIGI_STATE.AppContext.user.uniqueId) {
				return '@' + window.SIGI_STATE.AppContext.user.uniqueId;
			}
			if (window.__DEFAULT_SCOPE__ && window.__DEFAULT_SCOPE__['webapp.app-context'] && window.__DEFAULT_SCOPE__['webapp.app-context'].user && window.__DEFAULT_SCOPE__['webapp.app-context'].user.uniqueId) {
				return '@' + window.__DEFAULT_SCOPE__['webapp.app-context'].user.uniqueId;
			}
			// 2. Trích xuất username từ link profile trên thanh điều hướng
			const link = document.querySelector('a[href*="/@"]');
			if (link) {
				const m = link.getAttribute('href').match(/@([a-zA-Z0-9_.-]+)/);
				if (m) return '@' + m[1];
			}
			// 3. Trích xuất username từ title icon profile
			const title = document.querySelector('[data-e2e="user-title"], [data-e2e="profile-icon"]');
			if (title && title.innerText && title.innerText.trim()) {
				const text = title.innerText.trim();
				return text.startsWith('@') ? text : '@' + text;
			}
		} catch(_) {}
		return '';
	})()`
}

func (tikTokLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	return pickTikTokSessionAndIdentity(cookies)
}

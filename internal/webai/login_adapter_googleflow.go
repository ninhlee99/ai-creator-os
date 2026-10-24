package webai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type googleFlowLoginAdapter struct{}

func (googleFlowLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform: PlatformGoogleFlow,
		Name:     "Google Flow",
		// Keep login on the lightweight Google Account page. Opening Flow's
		// project canvas just to capture Google cookies makes the login browser
		// needlessly GPU-heavy.
		StartURL:    "https://myaccount.google.com/",
		CookieHost:  "google.com",
		CookieName:  "__Secure-1PSID",
		LandingHost: "myaccount.google.com",
	}
}

func (googleFlowLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (googleFlowLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := googleLoginCookie(auth)
	if err := ProbeGoogleFlowSession(ctx, cookie); err != nil {
		return fmt.Errorf("Google Flow chưa xác thực cookie: %w", err)
	}
	actualEmail := probeGoogleLoginAccountEmail(ctx, client, cookie)
	if actualEmail == "" {
		return fmt.Errorf("Google Flow đã xác thực phiên nhưng chưa xác minh được email tài khoản")
	}
	expectedEmail := accountEmail(auth.Email)
	if expectedEmail != "" && expectedEmail != actualEmail {
		return fmt.Errorf("cookie Google Flow thuộc %s, không khớp email đang đăng nhập %s", actualEmail, expectedEmail)
	}
	auth.Email = actualEmail
	return nil
}

func (googleFlowLoginAdapter) matchesIdentityURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "myaccount.google.com", "flow.google.com", "labs.google", "aitestkitchen.withgoogle.com":
		return true
	default:
		return false
	}
}

func (googleFlowLoginAdapter) identityScript() string { return googleIdentityScript() }

func (adapter googleFlowLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, _ := loginCookieEmailHints(cookies, target)
	session := latestLoginCookieValue(cookies, target, target.CookieName)
	if session == "" {
		session = firstLoginCookieValue(cookies, target,
			"__Secure-1PSID", "__Secure-3PSID", "SID", "SSID", "SAPISID",
			"__Secure-1PAPISID", "__Secure-1PSIDTS", "__Secure-3PSIDTS",
			"__session", "authToken", "auth_token", "NID",
		)
	}
	return session, email
}

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
		Platform:    PlatformGoogleFlow,
		Name:        "Google Flow",
		StartURL:    "https://labs.google/fx/editor",
		CookieHost:  "google.com",
		CookieName:  "__Secure-1PSID",
		LandingHost: "labs.google",
	}
}

func (googleFlowLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (googleFlowLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := googleLoginCookie(auth)
	var probeErr error
	if len(auth.CookieJar) > 0 {
		probeErr = ProbeGoogleFlowSessionWithCookieSnapshot(ctx, cookie, auth.CookieJar, nil)
	} else {
		probeErr = ProbeGoogleFlowSession(ctx, cookie)
	}
	actualEmail := probeGoogleLoginAccountEmail(ctx, client, cookie)
	if actualEmail == "" {
		actualEmail = accountEmail(auth.Email)
	}
	if probeErr != nil && actualEmail == "" {
		return fmt.Errorf("Google Flow chưa xác thực cookie: %w", probeErr)
	}
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
	case "myaccount.google.com", "flow.google.com", "labs.google", "aitestkitchen.withgoogle.com", "accounts.google.com":
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

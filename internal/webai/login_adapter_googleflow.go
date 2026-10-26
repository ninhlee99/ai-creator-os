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
		StartURL:    "https://flow.google.com/",
		CookieHost:  "flow.google.com",
		CookieName:  "__Secure-1PSID",
		LandingHost: "flow.google.com",
	}
}

func (googleFlowLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (googleFlowLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := googleLoginCookie(auth)
	if !strings.Contains(cookie, "SID=") && !strings.Contains(cookie, "__Secure-1PSID=") && !strings.Contains(cookie, "__Secure-3PSID=") {
		return fmt.Errorf("Google Flow chưa nhận được cookie xác thực Google (SID / __Secure-1PSID)")
	}

	actualEmail := probeGoogleLoginAccountEmail(ctx, client, cookie)
	if actualEmail == "" {
		actualEmail = accountEmail(auth.Email)
	}
	if actualEmail == "" {
		return fmt.Errorf("Google Flow chưa xác minh được email tài khoản Google")
	}

	var probeErr error
	if len(auth.CookieJar) > 0 {
		probeErr = ProbeGoogleFlowSessionWithCookieSnapshot(ctx, cookie, auth.CookieJar, nil)
	} else {
		probeErr = ProbeGoogleFlowSession(ctx, cookie)
	}
	if probeErr != nil {
		if strings.Contains(probeErr.Error(), "Flow HTTP 401") || strings.Contains(probeErr.Error(), "401") {
			return fmt.Errorf("Google Flow chưa xác thực cookie: %w", probeErr)
		}
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
	case "flow.google.com", "labs.google", "aitestkitchen.withgoogle.com", "myaccount.google.com", "accounts.google.com":
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
		)
	}
	return session, email
}

package webai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type geminiLoginAdapter struct{}

func (geminiLoginAdapter) target() WebLoginTarget {
	return WebLoginTarget{
		Platform:    PlatformGemini,
		Name:        "Gemini",
		StartURL:    "https://gemini.google.com/app",
		CookieHost:  "google.com",
		CookieName:  "__Secure-1PSID",
		Profile:     "gemini",
		LandingHost: "gemini.google.com",
	}
}

func (geminiLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (geminiLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	cookie := googleLoginCookie(auth)
	body, err := webLoginProbe(ctx, client, "https://gemini.google.com/app", "gemini.google.com", cookie, "text/html,application/xhtml+xml", nil)
	if err != nil {
		return fmt.Errorf("Gemini chưa xác thực cookie: %w", err)
	}
	email := accountEmail(parseGeminiAccountEmail(string(body)))
	if email == "" {
		email = probeGoogleAccountEmail(ctx, client, cookie)
	}
	if email == "" {
		return fmt.Errorf("Gemini chưa xác nhận được email tài khoản")
	}
	auth.Email = email
	return nil
}

func (geminiLoginAdapter) matchesIdentityURL(rawURL string) bool {
	return strings.Contains(rawURL, "gemini.google.com") && !strings.Contains(rawURL, "accounts.google.com") && !strings.Contains(rawURL, "AccountChooser")
}

func (geminiLoginAdapter) identityScript() string { return googleIdentityScript() }

func (adapter geminiLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	target := adapter.target()
	email, _ := loginCookieEmailHints(cookies, target)
	return latestLoginCookieValue(cookies, target, target.CookieName), email
}

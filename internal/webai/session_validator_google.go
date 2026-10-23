package webai

import (
	"context"
	"net/http"
	"strings"
)

func googleSessionCookie(auth *WebSessionAuth) string {
	cookie := strings.TrimSpace(auth.CookieHeader)
	if !strings.Contains(cookie, "__Secure-1PSID=") && auth.SessionValue != "" {
		return "__Secure-1PSID=" + auth.SessionValue + "; " + cookie
	}
	return cookie
}

func probeGoogleAccountEmail(ctx context.Context, client *http.Client, auth *WebSessionAuth) string {
	if auth == nil {
		return ""
	}
	auth.CookieHeader = googleSessionCookie(auth)
	for _, probe := range []struct{ url, host string }{
		{"https://myaccount.google.com/", "myaccount.google.com"},
		{"https://gemini.google.com/app", "gemini.google.com"},
	} {
		body, err := webSessionProbeCaptured(ctx, client, auth, probe.url, probe.host, auth.CookieHeader, "text/html,application/xhtml+xml", nil)
		if err != nil {
			continue
		}
		if email := accountEmail(parseGeminiAccountEmail(string(body))); email != "" {
			return email
		}
	}
	return ""
}

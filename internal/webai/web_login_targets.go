package webai

import (
	"context"
	"net/http"
)

// WebLoginTarget describes a target service to open and which cookie to wait for.
type WebLoginTarget struct {
	Platform    string
	Name        string
	StartURL    string
	CookieHost  string // cookie domain equal to this host or parent/child domain
	CookieName  string // primary cookie name
	LandingHost string // required host where the user finishes logging in
}

type webLoginAdapter interface {
	target() WebLoginTarget
	validateSession(context.Context, *http.Client, *CapturedWebAuth) error
	sessionFromCookies([]cdpCookie) (string, string)
	capture(context.Context, loginBrowserSession, WebLoginTarget, func(status, msg string)) (*CapturedWebAuth, error)
	matchesIdentityURL(string) bool
	identityScript() string
}

var webLoginAdapters = []webLoginAdapter{
	cookieWebLoginAdapter{config: WebLoginTarget{
		Platform: PlatformChatGPT, Name: "ChatGPT", StartURL: "https://chatgpt.com/",
		CookieHost: "chatgpt.com", CookieName: "__Secure-next-auth.session-token", LandingHost: "chatgpt.com",
	}, cookieNames: []string{"__Secure-next-auth.session-token", "__Host-next-auth.session-token", "oai-client-auth-session",
		"__Secure-next-auth.session-token.0", "__Secure-next-auth.session-token.1"}},
	cookieWebLoginAdapter{config: WebLoginTarget{
		Platform: PlatformClaude, Name: "Claude", StartURL: "https://claude.ai/login",
		CookieHost: "claude.ai", CookieName: "sessionKey", LandingHost: "claude.ai",
	}, cookieNames: []string{"sessionKey"}},
	cookieWebLoginAdapter{config: WebLoginTarget{
		Platform: PlatformGemini, Name: "Gemini", StartURL: "https://gemini.google.com/app",
		CookieHost: "google.com", CookieName: "__Secure-1PSID", LandingHost: "gemini.google.com",
	}, cookieNames: []string{"__Secure-1PSID", "__Secure-3PSID", "SID", "SSID", "SAPISID", "__Secure-1PAPISID",
		"__Secure-1PSIDTS", "__Secure-3PSIDTS", "__Host-1PLSID"}},
	cookieWebLoginAdapter{config: WebLoginTarget{
		Platform: PlatformGrok, Name: "Grok", StartURL: "https://grok.com/",
		CookieHost: "grok.com", CookieName: "sso", LandingHost: "grok.com",
	}, cookieNames: []string{"sso", "sso-rw", "auth_token", "ct0", "twid", "x-anonid", "x-userid"}},
	museLoginAdapter{},
	googleFlowLoginAdapter{},
	tikTokLoginAdapter{},
}

func webLoginAdapterFor(platform string) (webLoginAdapter, bool) {
	for _, adapter := range webLoginAdapters {
		if adapter.target().Platform == platform {
			return adapter, true
		}
	}
	return nil, false
}

// SupportedWebLoginTargets returns all browser-automatable login targets.
func SupportedWebLoginTargets() map[string]WebLoginTarget {
	targets := make(map[string]WebLoginTarget, len(webLoginAdapters))
	for _, adapter := range webLoginAdapters {
		target := adapter.target()
		targets[target.Platform] = target
	}
	return targets
}

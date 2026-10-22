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
	CookieHost  string // substring match on cookie domain
	CookieName  string // primary cookie name
	Profile     string // subdirectory name under ~/.aicos/browser-profiles
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
	chatGPTLoginAdapter{},
	claudeLoginAdapter{},
	geminiLoginAdapter{},
	grokLoginAdapter{},
	museLoginAdapter{},
	googleFlowLoginAdapter{},
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

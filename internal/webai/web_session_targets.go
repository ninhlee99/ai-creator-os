package webai

import (
	"context"
	"net/http"
)

// WebSessionTarget names one provider whose saved web session can be checked.
type WebSessionTarget struct {
	Platform string
	Name     string
}

// BrowserCookieScope filters cookies read by a headless provider fallback.
type BrowserCookieScope struct {
	Platform   string
	CookieHost string
	CookiePath string
}

type webSessionValidator interface {
	target() WebSessionTarget
	validateSession(context.Context, *http.Client, *WebSessionAuth) error
}

var webSessionValidators = []webSessionValidator{
	chatGPTSessionValidator{},
	claudeSessionValidator{},
	geminiSessionValidator{},
	grokSessionValidator{},
	museSessionValidator{},
	googleFlowSessionValidator{},
}

func webSessionValidatorFor(platform string) (webSessionValidator, bool) {
	for _, adapter := range webSessionValidators {
		if adapter.target().Platform == platform {
			return adapter, true
		}
	}
	return nil, false
}

func webSessionTargetFor(platform string) (WebSessionTarget, bool) {
	validator, ok := webSessionValidatorFor(platform)
	if !ok {
		return WebSessionTarget{}, false
	}
	return validator.target(), true
}

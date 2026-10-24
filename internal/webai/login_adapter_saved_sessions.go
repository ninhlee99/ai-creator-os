package webai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// cookieWebLoginAdapter handles providers whose login produces a verifiable
// first-party browser session cookie.
type cookieWebLoginAdapter struct {
	config      WebLoginTarget
	cookieNames []string
}

func (a cookieWebLoginAdapter) target() WebLoginTarget { return a.config }

func (a cookieWebLoginAdapter) capture(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(string, string)) (*CapturedWebAuth, error) {
	return captureCookieLogin(ctx, browser, target, updateStatus)
}

func (a cookieWebLoginAdapter) validateSession(ctx context.Context, client *http.Client, auth *CapturedWebAuth) error {
	validator, ok := webSessionValidatorFor(a.config.Platform)
	if !ok {
		return fmt.Errorf("%s chưa có bộ xác minh phiên", a.config.Name)
	}
	webAuth := &WebSessionAuth{
		Platform: a.config.Platform, SessionValue: auth.SessionValue,
		CookieHeader: auth.CookieHeader, CookieJar: auth.CookieJar, Email: auth.Email,
	}
	if err := validator.validateSession(ctx, client, webAuth); err != nil {
		return err
	}
	auth.Email = accountEmail(webAuth.Email)
	return nil
}

func (a cookieWebLoginAdapter) matchesIdentityURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	root := normalizeCookieDomain(a.config.CookieHost)
	return host == root || strings.HasSuffix(host, "."+root)
}

func (cookieWebLoginAdapter) identityScript() string { return accountIdentityScript() }

func (a cookieWebLoginAdapter) sessionFromCookies(cookies []cdpCookie) (string, string) {
	email, _ := loginCookieEmailHints(cookies, a.config)
	if a.config.Platform == PlatformChatGPT {
		if session := chunkedNextAuthSession(cookies, a.config); session != "" {
			return session, email
		}
	}
	return firstLoginCookieValue(cookies, a.config, a.cookieNames...), email
}

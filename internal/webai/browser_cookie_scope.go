package webai

import (
	"sort"
	"strings"
)

// BrowserCookieScope filters cookies read by a headless provider fallback.
type BrowserCookieScope struct {
	Platform   string
	CookieHost string
	CookiePath string
}

func browserCookieDomainMatches(domain string, scope BrowserCookieScope) bool {
	if scope.CookieHost == "" {
		return true
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	domainCookie := strings.HasPrefix(domain, ".")
	domain = strings.TrimSuffix(strings.TrimPrefix(domain, "."), ".")
	host := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(scope.CookieHost)), "."), ".")
	return domain != "" && host != "" && (host == domain || (domainCookie && strings.HasSuffix(host, "."+domain)))
}

// browserCookieHeaderAllPaths reads cookies scoped to one service host. It
// preserves duplicate names and browser path ordering for headless fallbacks.
func browserCookieHeaderAllPaths(cookies []cdpCookie, scope BrowserCookieScope) string {
	type orderedCookie struct {
		cookie cdpCookie
		order  int
	}
	applicable := make([]orderedCookie, 0, len(cookies))
	for i, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" || !browserCookieDomainMatches(cookie.Domain, scope) {
			continue
		}
		applicable = append(applicable, orderedCookie{cookie: cookie, order: i})
	}
	sort.SliceStable(applicable, func(i, j int) bool {
		left, right := applicable[i].cookie.Path, applicable[j].cookie.Path
		if left == "" {
			left = "/"
		}
		if right == "" {
			right = "/"
		}
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return applicable[i].order < applicable[j].order
	})
	parts := make([]string, 0, len(applicable))
	for _, item := range applicable {
		parts = append(parts, item.cookie.Name+"="+item.cookie.Value)
	}
	return strings.Join(parts, "; ")
}

// browserCookieHeaderForRequest follows browser host/path matching and ordering
// for one request URL, while retaining same-name cookies on distinct paths.
func browserCookieHeaderForRequest(cookies []cdpCookie, scope BrowserCookieScope) string {
	type orderedCookie struct {
		cookie cdpCookie
		order  int
	}
	applicable := make([]orderedCookie, 0, len(cookies))
	for i, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" || !browserCookieDomainMatches(cookie.Domain, scope) || !museCookiePathMatches(cookie.Path, scope.CookiePath) {
			continue
		}
		applicable = append(applicable, orderedCookie{cookie: cookie, order: i})
	}
	sort.SliceStable(applicable, func(i, j int) bool {
		left, right := applicable[i].cookie.Path, applicable[j].cookie.Path
		if left == "" {
			left = "/"
		}
		if right == "" {
			right = "/"
		}
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return applicable[i].order < applicable[j].order
	})
	parts := make([]string, 0, len(applicable))
	for _, item := range applicable {
		parts = append(parts, item.cookie.Name+"="+item.cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func googleFlowSessionCookiePresent(cookies []cdpCookie) bool {
	scope := BrowserCookieScope{Platform: PlatformGoogleFlow, CookieHost: "flow.google.com"}
	for _, cookie := range cookies {
		if !browserCookieDomainMatches(cookie.Domain, scope) || cookie.Value == "" {
			continue
		}
		switch cookie.Name {
		case "__Secure-1PSID", "__Secure-3PSID", "SID", "SSID", "SAPISID",
			"__Secure-1PAPISID", "__Secure-1PSIDTS", "__Secure-3PSIDTS",
			"__session", "authToken", "auth_token", "NID":
			return true
		}
	}
	return false
}

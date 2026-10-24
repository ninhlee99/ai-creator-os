package webai

import (
	"sort"
	"strings"
)

type museSessionCookieCandidate struct {
	name     string
	value    string
	priority int
	pathLen  int
	order    int
}

// museSessionCookieCandidates recognizes Muse session cookies in a headless
// browser snapshot and keeps provider path/domain scope intact.
func museSessionCookieCandidates(cookies []cdpCookie, target BrowserCookieScope) []museSessionCookieCandidate {
	candidates := make([]museSessionCookieCandidate, 0, len(cookies))
	seen := make(map[string]struct{}, len(cookies))
	for i, cookie := range cookies {
		if !browserCookieDomainMatches(cookie.Domain, target) || !museCookiePathMatches(cookie.Path, target.CookiePath) || isMusePlaceholderCookie(cookie.Value) {
			continue
		}
		priority := 0
		switch {
		case cookie.Name == "hatch_sess" && len(cookie.Value) > 4:
			priority = 100
		case cookie.Name == "hatch_native_auth_token" && len(cookie.Value) > 4:
			priority = 80
		case isMuseSessionCookie(cookie.Name) && len(cookie.Value) > 4:
			priority = 60
		case cookie.Name == "ps_l" && len(cookie.Value) > 5:
			priority = 40
		default:
			continue
		}
		key := cookie.Name + "\x00" + cookie.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, museSessionCookieCandidate{
			name: cookie.Name, value: cookie.Value, priority: priority, pathLen: len(cookie.Path), order: i,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		if candidates[i].pathLen != candidates[j].pathLen {
			return candidates[i].pathLen > candidates[j].pathLen
		}
		return candidates[i].order > candidates[j].order
	})
	return candidates
}

func museCookiePathMatches(cookiePath, requestPath string) bool {
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == "" {
		requestPath = "/"
	}
	if cookiePath == requestPath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	return strings.HasSuffix(cookiePath, "/") || strings.HasPrefix(requestPath[len(cookiePath):], "/")
}

func isMusePlaceholderCookie(value string) bool {
	return value == "0" || value == "1" || len(value) < 3
}

func isMuseSessionCookie(name string) bool {
	switch name {
	case "hatch_sess", "hatch_native_auth_token", "c_user", "session", "auth_token", "token", "connect.sid":
		return true
	default:
		return false
	}
}

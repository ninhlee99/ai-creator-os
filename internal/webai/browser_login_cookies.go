package webai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func extractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.RawStdEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	for _, key := range []string{"email", "user_email", "preferred_username"} {
		if val, ok := data[key].(string); ok {
			val = strings.TrimSpace(val)
			if strings.Contains(val, "@") && !isSystemOrStaticEmail(val) {
				return val
			}
		}
	}
	return ""
}
func pickSessionCookieAndEmail(cookies []cdpCookie, target WebLoginTarget) (string, string) {
	adapter, ok := webLoginAdapterFor(target.Platform)
	if !ok {
		return "", ""
	}
	return adapter.sessionFromCookies(cookies)
}

func loginCookieDomainMatches(domain string, target WebLoginTarget) bool {
	if target.CookieHost == "" {
		return true
	}
	domain = strings.TrimPrefix(domain, ".")
	if target.Platform == PlatformMuseAI {
		return domain == "muse.ai" || strings.HasSuffix(domain, ".muse.ai")
	}
	matched := strings.Contains(domain, target.CookieHost) || strings.Contains(target.CookieHost, domain)
	if !matched && (target.Platform == PlatformGoogleFlow || target.CookieHost == "google.com") {
		matched = strings.Contains(domain, "google.com") || strings.Contains(domain, "google") || strings.Contains(domain, "withgoogle.com")
	}
	return matched
}

func loginCookieEmailHints(cookies []cdpCookie, target WebLoginTarget) (email, providerEmail string) {
	for _, cookie := range cookies {
		inTargetDomain := loginCookieDomainMatches(cookie.Domain, target)
		isMuseProviderCookie := target.Platform == PlatformMuseAI && isMuseIdentityProviderHost(strings.TrimPrefix(cookie.Domain, "."))
		if !inTargetDomain && !isMuseProviderCookie {
			continue
		}

		unescapedValue, _ := url.QueryUnescape(cookie.Value)
		var candidate string
		if strings.Contains(strings.ToLower(cookie.Name), "email") || strings.Contains(strings.ToLower(cookie.Name), "user") || isMuseProviderCookie {
			for _, value := range []string{cookie.Value, unescapedValue} {
				if match := generalEmailRe.FindString(value); match != "" && !isSystemOrStaticEmail(match) {
					candidate = match
					break
				}
			}
		}
		if candidate == "" && strings.Count(cookie.Value, ".") == 2 {
			candidate = extractEmailFromJWT(cookie.Value)
		}
		if candidate == "" {
			continue
		}
		if inTargetDomain && email == "" {
			email = candidate
		} else if isMuseProviderCookie && providerEmail == "" {
			providerEmail = candidate
		}
	}
	return email, providerEmail
}

func latestLoginCookieValue(cookies []cdpCookie, target WebLoginTarget, name string) string {
	var value string
	for _, cookie := range cookies {
		if loginCookieDomainMatches(cookie.Domain, target) && cookie.Name == name && cookie.Value != "" {
			value = cookie.Value
		}
	}
	return value
}

func firstLoginCookieValue(cookies []cdpCookie, target WebLoginTarget, names ...string) string {
	for _, cookie := range cookies {
		if !loginCookieDomainMatches(cookie.Domain, target) || cookie.Value == "" {
			continue
		}
		for _, name := range names {
			if cookie.Name == name {
				return cookie.Value
			}
		}
	}
	return ""
}

func chunkedNextAuthSession(cookies []cdpCookie, target WebLoginTarget) string {
	var chunks []string
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("%s.%d", target.CookieName, i)
		if value := firstLoginCookieValue(cookies, target, name); value != "" {
			chunks = append(chunks, name+"="+value)
		}
	}
	return strings.Join(chunks, "; ")
}

func cookieHeaderForHost(cookies []cdpCookie, hostSubstr string) string {
	var parts []string
	seen := map[string]bool{}
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if hostSubstr != "" {
			d := strings.TrimPrefix(c.Domain, ".")
			matched := false
			if hostSubstr == "muse.ai" {
				matched = d == "muse.ai" || strings.HasSuffix(d, ".muse.ai")
			} else {
				matched = strings.Contains(d, hostSubstr) || strings.Contains(hostSubstr, d)
			}
			if !matched && (hostSubstr == "google.com" || hostSubstr == "labs.google" || hostSubstr == "flow.google.com") {
				if strings.Contains(d, "google.com") || strings.Contains(d, "google") || strings.Contains(d, "withgoogle.com") {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}
func replaceCookiePair(header, name, value string) string {
	if name == "" || value == "" {
		return header
	}
	parts := make([]string, 0, strings.Count(header, ";")+1)
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		cookieName, _, ok := strings.Cut(part, "=")
		if ok && cookieName != name {
			parts = append(parts, part)
		}
	}
	parts = append(parts, name+"="+value)
	return strings.Join(parts, "; ")
}
func cookieHeaderHasPair(header, name, value string) bool {
	for _, part := range strings.Split(header, ";") {
		cookieName, cookieValue, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && cookieName == name && cookieValue == value {
			return true
		}
	}
	return false
}

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
	domain = normalizeCookieDomain(domain)
	if target.Platform == PlatformMuseAI {
		return cookieDomainWithin(domain, "muse.ai")
	}
	if target.Platform == PlatformGoogleFlow {
		return cookieDomainWithin(domain, "google.com") || cookieDomainWithin(domain, "withgoogle.com") ||
			(target.Platform == PlatformGoogleFlow && cookieDomainWithin(domain, "labs.google"))
	}
	if target.Platform == PlatformGrok {
		return cookieDomainWithin(domain, "grok.com") || cookieDomainWithin(domain, "x.ai") ||
			cookieDomainWithin(domain, "twitter.com") || cookieDomainWithin(domain, "x.com")
	}
	return cookieDomainWithin(domain, target.CookieHost)
}

func normalizeCookieDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimPrefix(domain, ".")
	return strings.TrimSuffix(domain, ".")
}

// cookieDomainWithin accepts a cookie scoped to root or one of its subdomains.
// It avoids substring matches such as chatgpt.com.attacker.example.
func cookieDomainWithin(domain, root string) bool {
	domain = normalizeCookieDomain(domain)
	root = normalizeCookieDomain(root)
	return domain != "" && root != "" && (domain == root || strings.HasSuffix(domain, "."+root))
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

func cookieHeaderForTarget(cookies []cdpCookie, target WebLoginTarget) string {
	var parts []string
	seen := map[string]bool{}
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if !loginCookieDomainMatches(c.Domain, target) {
			continue
		}
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func loginCookieJarForTarget(cookies []cdpCookie, target WebLoginTarget) []BrowserCookie {
	jar := make([]BrowserCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if loginCookieDomainMatches(cookie.Domain, target) {
			jar = append(jar, cookie)
		}
	}
	return jar
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

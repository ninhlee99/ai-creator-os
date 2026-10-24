package webai

import (
	"regexp"
	"strings"
)

var (
	wizEmailRe       = regexp.MustCompile(`"oPEP7c"\s*:\s*"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})"`)
	ariaLabelEmailRe = regexp.MustCompile(`aria-label="[^"]*?\(([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})\)"`)
	htmlTagEmailRe   = regexp.MustCompile(`>([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})<`)
	generalEmailRe   = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4})`)
)

func accountEmail(value string) string {
	value = strings.TrimSpace(value)
	if match := generalEmailRe.FindString(value); match != "" && !isSystemOrStaticEmail(match) {
		return strings.ToLower(match)
	}
	return ""
}

func cleanExtractedEmail(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if isSystemOrStaticEmail(candidate) {
		return ""
	}
	if match := generalEmailRe.FindString(candidate); match != "" && !isSystemOrStaticEmail(match) {
		return match
	}
	return ""
}

func firstAccountEmail(values ...string) string {
	for _, value := range values {
		if email := accountEmail(value); email != "" {
			return email
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isSystemOrStaticEmail(email string) bool {
	low := strings.ToLower(email)
	return strings.HasSuffix(low, "@google.com") ||
		strings.Contains(low, "example.com") ||
		strings.Contains(low, "w3.org") ||
		strings.Contains(low, "schema.org") ||
		strings.Contains(low, "sentry") ||
		strings.Contains(low, "mixpanel") ||
		strings.Contains(low, "cloudflare")
}

func parseGoogleAccountEmail(html string) string {
	for _, pattern := range []*regexp.Regexp{wizEmailRe, ariaLabelEmailRe, htmlTagEmailRe} {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			if email := accountEmail(match[1]); email != "" {
				return email
			}
		}
	}
	for _, match := range generalEmailRe.FindAllString(html, -1) {
		if email := accountEmail(match); email != "" {
			return email
		}
	}
	return ""
}

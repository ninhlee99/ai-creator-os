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

func cleanExtractedEmail(cand string) string {
	cand = strings.TrimSpace(cand)
	if isSystemOrStaticEmail(cand) {
		return ""
	}
	if m := generalEmailRe.FindString(cand); m != "" {
		if !isSystemOrStaticEmail(m) {
			return m
		}
	}
	if strings.Contains(cand, "@") && !isSystemOrStaticEmail(cand) {
		return cand
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
func parseGeminiAccountEmail(html string) string {
	if m := wizEmailRe.FindStringSubmatch(html); len(m) > 1 {
		cand := strings.TrimSpace(m[1])
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	if m := ariaLabelEmailRe.FindStringSubmatch(html); len(m) > 1 {
		cand := strings.TrimSpace(m[1])
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	for _, m := range htmlTagEmailRe.FindAllStringSubmatch(html, -1) {
		if len(m) > 1 {
			cand := strings.TrimSpace(m[1])
			if !isSystemOrStaticEmail(cand) {
				return cand
			}
		}
	}
	for _, cand := range generalEmailRe.FindAllString(html, -1) {
		cand = strings.TrimSpace(cand)
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	return ""
}

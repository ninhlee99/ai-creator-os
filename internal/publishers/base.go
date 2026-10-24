// Package publishers provides TikTok video draft publishing.
//
// Publish results are surfaced as PublishResult values; the orchestration
// layer records them with ledger.Decide(agent, action, target, reason,
// inputs) — the only ledger call publishers' results flow through.
package publishers

import (
	"os"
	"strings"
	"unicode"
)

// PublishResult is the outcome of one platform publish. Expected API
// failures come back as Ok=false rather than errors — Publish never raises
// for them.
type PublishResult struct {
	Ok       bool
	Platform string
	RemoteID string
	Error    string
	// On successful results, Draft marks a provider-specific non-public outcome.
	// TikTok uploads remain drafts until the creator reviews them in app.
	Draft bool
	// DeliveryUncertain means the provider may have accepted the publish
	// request, but the app did not receive enough confirmation to retry safely.
	DeliveryUncertain bool
	// UploadRequestAttempted marks that the provider's upload endpoint was
	// called, even when its response failed or delivery was ambiguous.
	UploadRequestAttempted bool
}

// envFor ports Python's _env_for: per-account env override first
// (NAME_<USERNAME_SANITIZED>), then the shared NAME. Returns "" when unset.
func envFor(username string, names ...string) string {
	var b strings.Builder
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	uname := strings.ToUpper(b.String())
	for _, n := range names {
		if v := os.Getenv(n + "_" + uname); v != "" {
			return v
		}
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// sanitizeUsername replaces non-alphanumeric runes with '_', mirroring the
// Python token-file naming (tiktok_token_<username>.json).
func sanitizeUsername(username string) string {
	var b strings.Builder
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// handlesKind reports membership of kind in kinds.
func handlesKind(kind string, kinds []string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// truncate shortens s to at most n runes (Python slices to [:300] chars).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

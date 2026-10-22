package freeonly

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync/atomic"
)

var geminiFreeTierConfirmed atomic.Bool

// SetGeminiFreeTierConfirmed records operator attestation; provider state is
// not independently verified.
func SetGeminiFreeTierConfirmed(confirmed bool) {
	geminiFreeTierConfirmed.Store(confirmed)
}

// GeminiFreeTierConfirmed reports whether Gemini API calls have operator
// approval to use the currently configured free-tier key set.
func GeminiFreeTierConfirmed() bool {
	return geminiFreeTierConfirmed.Load()
}

// GeminiKeysFingerprint returns a stable, non-reversible fingerprint for a
// key set, so confirmation stops applying after keys change.
func GeminiKeysFingerprint(keys []string) string {
	seen := make(map[string]struct{}, len(keys))
	clean := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, key)
	}
	sort.Strings(clean)
	digest := sha256.Sum256([]byte(strings.Join(clean, "\n")))
	return hex.EncodeToString(digest[:])
}

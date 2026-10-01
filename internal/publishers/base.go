// Package publishers ports the Python publishers/ tree to Go.
//
// One rendered video can go to TikTok, Facebook (Page) and YouTube — each
// account decides which platforms are wired and, for YouTube, which content
// kinds its channel accepts (e.g. a music channel takes ai_music + ai_remix,
// a story channel takes short_film).
//
// Draft-first everywhere it is supported: nothing goes public without the
// platform's own review surface or an explicit env flip.
//
// Publish results are surfaced as PublishResult values; the orchestration
// layer records them with ledger.Decide(agent, action, target, reason,
// inputs) — the only ledger call publishers' results flow through.
package publishers

import (
	"context"
	"os"
	"strings"
	"unicode"
)

// ContentKinds are the content kinds the factory can produce. A publisher
// declares which of these it handles; YouTube additionally filters per
// account via the youtube content types passed to BuildPublishers.
var ContentKinds = []string{"short_video", "short_film", "ai_music", "ai_remix"}

// PublishResult is the outcome of one platform publish. Expected API
// failures come back as Ok=false rather than errors — Publish never raises
// for them.
type PublishResult struct {
	Ok       bool
	Platform string
	RemoteID string
	URL      string
	Error    string
	Draft    bool
}

// Publisher is one platform endpoint for a rendered video.
type Publisher interface {
	// Name is the platform key: "tiktok", "facebook", "youtube".
	Name() string
	// IsConfigured reports whether credentials for this platform exist.
	IsConfigured() bool
	// Handles reports whether the publisher accepts a content kind.
	Handles(kind string) bool
	// Publish uploads the video and (draft-)publishes it. It never raises
	// for expected API failures — those return PublishResult{Ok:false}.
	Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult
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

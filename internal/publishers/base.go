// Package publishers ports the Python publishers/ tree to Go.
//
// One rendered video can go to TikTok, Facebook (Page) and YouTube — each
// account decides which platforms are wired and, for YouTube, which content
// kinds its channel accepts (e.g. a music channel takes ai_music + ai_remix,
// a story channel takes short_film).
//
// Publish visibility depends on platform and configuration: TikTok uploads a
// creator-inbox draft, Facebook defaults to an unpublished Page video (the
// CLI path uses a native draft), Instagram defaults to an unpublished
// processed container, Threads publishes text immediately, and YouTube
// follows its configured privacy.
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
	// On successful results, Draft marks a provider-specific non-public outcome.
	// It may mean native draft (TikTok or Facebook CLI), unpublished Page
	// video (Facebook Graph), processed container (Instagram), or non-public
	// privacy status (YouTube).
	Draft bool
	// DeliveryUncertain means the provider may have accepted the publish
	// request, but the app did not receive enough confirmation to retry safely.
	DeliveryUncertain bool
	// UploadRequestAttempted marks that the provider's upload endpoint was
	// called, even when its response failed or delivery was ambiguous.
	UploadRequestAttempted bool
	// ThumbnailSet/Error: kết quả đặt thumbnail tùy chỉnh (Đợt M1).
	// Thumbnail là phần tăng thêm — video vẫn đăng thành công dù lỗi.
	ThumbnailSet   bool
	ThumbnailError string
}

// Publisher is one platform endpoint for a rendered video.
type Publisher interface {
	// Name is the platform key: "tiktok", "facebook", "youtube".
	Name() string
	// IsConfigured reports whether credentials for this platform exist.
	IsConfigured() bool
	// Handles reports whether the publisher accepts a content kind.
	Handles(kind string) bool
	// Publish sends provider-specific content from the supplied media and text.
	// Some adapters publish text or an unpublished container instead of video.
	// Expected API failures return PublishResult{Ok:false}.
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

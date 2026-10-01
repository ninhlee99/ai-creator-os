package publishers

import (
	"context"
	"os"

	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// tiktokKinds are the content kinds the TikTok publisher handles.
var tiktokKinds = []string{"short_video", "short_film"}

// TikTokPublisher publishes via the TikTok Content Posting API
// (draft-first). One OAuth token file per TikTok account:
// tiktok_token_<username>.json (gitignored, never logged).
type TikTokPublisher struct {
	Username     string
	tokenPath    string
	clientKey    string
	clientSecret string
	draftOnly    bool
	privacy      string
}

// NewTikTokPublisher wires credentials from the environment:
// TIKTOK_CLIENT_KEY[_<USERNAME>], TIKTOK_CLIENT_SECRET[_<USERNAME>],
// TIKTOK_DRAFT_ONLY (default "1"), TIKTOK_PRIVACY (default "SELF_ONLY").
func NewTikTokPublisher(username string) *TikTokPublisher {
	safe := sanitizeUsername(username)
	draftOnly := true
	if v, ok := os.LookupEnv("TIKTOK_DRAFT_ONLY"); ok {
		draftOnly = v == "1"
	}
	privacy := os.Getenv("TIKTOK_PRIVACY")
	if privacy == "" {
		privacy = "SELF_ONLY"
	}
	return &TikTokPublisher{
		Username:     username,
		tokenPath:    "tiktok_token_" + safe + ".json",
		clientKey:    envFor(username, "TIKTOK_CLIENT_KEY"),
		clientSecret: envFor(username, "TIKTOK_CLIENT_SECRET"),
		draftOnly:    draftOnly,
		privacy:      privacy,
	}
}

// Name returns the platform key.
func (p *TikTokPublisher) Name() string { return "tiktok" }

// IsConfigured reports whether client credentials and a token file exist.
func (p *TikTokPublisher) IsConfigured() bool {
	if p.clientKey == "" || p.clientSecret == "" {
		return false
	}
	_, err := os.Stat(p.tokenPath)
	return err == nil
}

// Handles reports whether kind is one of short_video, short_film.
func (p *TikTokPublisher) Handles(kind string) bool { return handlesKind(kind, tiktokKinds) }

func (p *TikTokPublisher) client() *tiktok.PostingClient {
	return &tiktok.PostingClient{
		ClientKey:    p.clientKey,
		ClientSecret: p.clientSecret,
		Store:        &tiktok.TokenStore{Path: p.tokenPath},
	}
}

// Publish uploads the video via the Content Posting API and returns the
// publish_id. Expected API failures come back as PublishResult{Ok:false}.
func (p *TikTokPublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	if !p.IsConfigured() {
		return PublishResult{Ok: false, Platform: p.Name(), Error: "tiktok not configured"}
	}
	if title == "" {
		title = truncate(description, 90)
	}
	publishID, err := p.client().PublishFile(videoPath, title, p.privacy, p.draftOnly)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	return PublishResult{Ok: true, Platform: p.Name(), RemoteID: publishID, Draft: p.draftOnly}
}

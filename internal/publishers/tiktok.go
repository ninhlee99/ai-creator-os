package publishers

import (
	"context"
	"fmt"
	"net/url"
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
		tokenPath:    TikTokTokenPath(username),
		clientKey:    envFor(username, "TIKTOK_CLIENT_KEY"),
		clientSecret: envFor(username, "TIKTOK_CLIENT_SECRET"),
		draftOnly:    draftOnly,
		privacy:      privacy,
	}
}

// TikTokTokenPath is the OAuth token file for username, relative to the
// app's working directory (non letter/digit runes become "_", case kept).
func TikTokTokenPath(username string) string {
	return "tiktok_token_" + sanitizeUsername(username) + ".json"
}

// DefaultTikTokRedirectURI is the local callback of the dashboard itself
// (TikTok Desktop platform: localhost redirect + PKCE), so OAuth works on a
// Mac with no public website.
const DefaultTikTokRedirectURI = "http://127.0.0.1:8080/publishers/tiktok/callback"

// TikTokRedirectURI is the OAuth redirect registered on the TikTok
// developer app (TIKTOK_REDIRECT_URI, default DefaultTikTokRedirectURI).
// It must match the registered value character for character.
func TikTokRedirectURI() string {
	if v := os.Getenv("TIKTOK_REDIRECT_URI"); v != "" {
		return v
	}
	return DefaultTikTokRedirectURI
}

// TikTokRedirectIsLocal reports a localhost/127.0.0.1 redirect: the
// Desktop flow, which needs PKCE and lands back on this app directly.
func TikTokRedirectIsLocal() bool {
	u, err := url.Parse(TikTokRedirectURI())
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// HasClient reports whether client key + secret are set for this account.
func (p *TikTokPublisher) HasClient() bool { return p.clientKey != "" && p.clientSecret != "" }

// Scope is what the connect flow asks for: drafts need only video.upload;
// video.publish is requested only when direct posting is enabled.
func (p *TikTokPublisher) Scope() string {
	if p.draftOnly {
		return "user.info.basic," + tiktok.ScopeUploadDraft
	}
	return "user.info.basic," + tiktok.ScopeUploadDraft + "," + tiktok.ScopeDirectPost
}

// AuthorizeURL is the Login Kit URL the creator opens to grant access.
// codeChallenge is "" for the Web flow (see tiktok.NewPKCE).
func (p *TikTokPublisher) AuthorizeURL(state, codeChallenge string) string {
	return p.client().AuthorizeURL(TikTokRedirectURI(), p.Scope(), state, codeChallenge)
}

// ExchangeCode trades the code from the redirect for tokens and writes
// the token file. Returns the granted scope string (never the tokens).
func (p *TikTokPublisher) ExchangeCode(code, codeVerifier string) (string, error) {
	if !p.HasClient() {
		return "", fmt.Errorf("thiếu TIKTOK_CLIENT_KEY / TIKTOK_CLIENT_SECRET")
	}
	data, err := p.client().ExchangeCode(code, TikTokRedirectURI(), codeVerifier)
	if err != nil {
		return "", err
	}
	scope, _ := data["scope"].(string)
	return scope, nil
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

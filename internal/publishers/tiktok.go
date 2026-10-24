package publishers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// tiktokKinds are the content kinds the TikTok publisher handles.
var tiktokKinds = []string{"short_video"}

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
// TIKTOK_DRAFT_ONLY (default "1"; Direct Post remains gated),
// TIKTOK_PRIVACY (default "SELF_ONLY"; used only by the gated Direct Post path).
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

// TikTokTokenPath is the OAuth token file for username, inside the
// app's token directory (<dataDir>/tokens, see SetTokenDir).
func TikTokTokenPath(username string) string {
	return tokenPath("tiktok_token_" + sanitizeUsername(username) + ".json")
}

// DefaultTikTokRedirectURI is the local callback of the dashboard itself
// (TikTok Desktop platform: localhost redirect + PKCE), so OAuth works on a
// Mac with no public website.
const DefaultTikTokRedirectURI = "http://127.0.0.1:8080/publishers/tiktok/callback"

// TikTokRedirectURI is the Desktop Login Kit callback registered in TikTok
// Developer Portal. It must be a loopback URI and match character for character.
func TikTokRedirectURI() string {
	if v := strings.TrimSpace(os.Getenv("TIKTOK_REDIRECT_URI")); v != "" {
		return v
	}
	return DefaultTikTokRedirectURI
}

// ValidateTikTokDesktopRedirectURI rejects Web redirects. TikTok Desktop
// Login Kit only accepts localhost/127.0.0.1 callbacks with an explicit port.
func ValidateTikTokDesktopRedirectURI() error {
	raw := TikTokRedirectURI()
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("Redirect URI TikTok Desktop không hợp lệ")
	}
	host := strings.ToLower(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	validHost := host == "localhost" || host == "127.0.0.1"
	validScheme := u.Scheme == "http"
	validPort := portErr == nil && port > 0 && port <= 65535
	validPath := u.Path == "/publishers/tiktok/callback" && u.RawPath == ""
	if !validHost || !validScheme || !validPort || !validPath || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("TikTok Desktop cần Redirect URI loopback có port, ví dụ %s; đăng ký URI này trong TikTok Developer Portal", DefaultTikTokRedirectURI)
	}
	return nil
}

// HasClient reports whether client key + secret are set for this account.
func (p *TikTokPublisher) HasClient() bool { return p.clientKey != "" && p.clientSecret != "" }

// Scope is what the connect flow asks for. Direct Post stays disabled until
// eligibility is verified and the per-post consent flow exists.
func (p *TikTokPublisher) Scope() string {
	return "user.info.basic,user.info.stats,video.list," + tiktok.ScopeUploadDraft
}

// AuthorizeURL is the Login Kit URL the creator opens to grant access.
// codeChallenge comes from tiktok.NewPKCE ("" disables PKCE).
func (p *TikTokPublisher) AuthorizeURL(state, codeChallenge string) string {
	return p.client().AuthorizeURL(TikTokRedirectURI(), p.Scope(), state, codeChallenge)
}

// ExchangeCode trades the code from the redirect for tokens and writes
// the token file. Returns the granted scope string (never the tokens).
func (p *TikTokPublisher) ExchangeCode(code, codeVerifier string) (string, error) {
	if !p.HasClient() {
		return "", fmt.Errorf("thiếu TIKTOK_CLIENT_KEY / TIKTOK_CLIENT_SECRET")
	}
	if err := ValidateTikTokDesktopRedirectURI(); err != nil {
		return "", err
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

// AccessToken returns an active access token, refreshing if necessary.
func (p *TikTokPublisher) AccessToken() (string, error) {
	if !p.HasClient() {
		return "", fmt.Errorf("thiếu client credentials")
	}
	return p.client().AccessToken()
}

// DisplayUserStats reads profile counters authorized through TikTok Display API.
func (p *TikTokPublisher) DisplayUserStats(ctx context.Context) (tiktok.DisplayUserStats, error) {
	if !p.IsConfigured() {
		return tiktok.DisplayUserStats{}, fmt.Errorf("TikTok chưa kết nối OAuth")
	}
	return p.client().DisplayUserStats(ctx)
}

// DisplayVideoMetrics reads current counters for recent public TikTok videos.
func (p *TikTokPublisher) DisplayVideoMetrics(ctx context.Context, limit int) ([]tiktok.DisplayVideoMetrics, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("TikTok chưa kết nối OAuth")
	}
	return p.client().ListDisplayVideoMetrics(ctx, limit)
}

// Handles reports whether kind is a TikTok short video.
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
	if !p.draftOnly {
		return PublishResult{
			Ok:       false,
			Platform: p.Name(),
			Error:    "TikTok Direct Post đang bị khóa: chưa xác minh eligibility và chưa có consent theo từng bài",
		}
	}
	if title == "" {
		title = truncate(description, 90)
	}
	publishID, err := p.client().PublishFile(videoPath, title, p.privacy, p.draftOnly)
	if err != nil {
		var uncertain tiktok.DeliveryUncertainError
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300),
			DeliveryUncertain: errors.As(err, &uncertain)}
	}
	return PublishResult{Ok: true, Platform: p.Name(), RemoteID: publishID, Draft: p.draftOnly}
}

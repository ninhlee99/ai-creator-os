package tiktok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// HTTPFunc is the injectable HTTP transport used by the clients.
// It returns the status code and raw body; err is non-nil for transport
// failures and (in DefaultHTTP) for non-2xx responses.
type HTTPFunc func(method, url string, headers map[string]string, body []byte) (status int, resp []byte, err error)

// DefaultHTTP is the net/http-backed transport (replaces urllib).
func DefaultHTTP(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode >= 400 {
		snip := raw
		if len(snip) > 500 {
			snip = snip[:500]
		}
		return resp.StatusCode, nil, fmt.Errorf("HTTP %d %s: %q", resp.StatusCode, url, snip)
	}
	return resp.StatusCode, raw, nil
}

// decodeJSON unmarshals raw into a generic map; non-JSON bodies come back
// under the "raw" key so callers always get a map.
func decodeJSON(raw []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{"raw": string(raw)}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

// nested walks m through keys, returning (value, true) on success.
func nested(m map[string]any, keys ...string) (any, bool) {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// nestedString is nested plus a string assertion.
func nestedString(m map[string]any, keys ...string) (string, bool) {
	v, ok := nested(m, keys...)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ---------------------------------------------------------------------------
// TokenStore — port of tiktok/posting/client.py TokenStore.
// Persisted OAuth tokens. NEVER commit the file (gitignored); tokens are
// never logged.
// Lifecycle: access_token 24h, refresh_token 365d (rotates on refresh).
// ---------------------------------------------------------------------------

// Tokens is the persisted OAuth token document.
type Tokens struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ObtainedAt   int64  `json:"obtained_at"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

// TokenStore persists OAuth tokens to a JSON file.
type TokenStore struct {
	Path string
}

// Load returns the stored tokens, or zero Tokens when no file exists.
func (s *TokenStore) Load() (Tokens, error) {
	var t Tokens
	raw, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tokens{}, err
	}
	return t, nil
}

// Save stamps obtained_at and writes the tokens (creating parent dirs).
func (s *TokenStore) Save(t Tokens) error {
	t.ObtainedAt = time.Now().Unix()
	dir := filepath.Dir(s.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, raw, 0o600)
}

// ---------------------------------------------------------------------------
// PostingClient — port of tiktok/posting/client.py PostingClient.
//
// Flow: OAuth (Login Kit + PKCE) -> token store -> POST
// /v2/post/publish/video/init/ -> PUT chunks to upload_url ->
// POST /v2/post/publish/status/fetch/ until PUBLISH_COMPLETE.
//
// Gates (human steps, not code):
//  1. Developer app with Content Posting API product enabled.
//  2. App audit (~1-2 wks) + Direct Post audit (~5-10 biz days) for PUBLIC
//     posts. Until then: max 5 test users, SELF_ONLY visibility.
//  3. Request only the scopes you use: video.publish / video.upload.
//
// Rate limit: 6 req/min per user token on publish endpoints.
// ~15 posts/day per creator, shared across ALL API clients (Direct Post).
// ---------------------------------------------------------------------------

const (
	// Base is the TikTok Open API host.
	Base = "https://open.tiktokapis.com"
	// AuthURL is the Login Kit authorization endpoint.
	AuthURL = "https://www.tiktok.com/v2/auth/authorize/"

	// ScopeDirectPost allows publishing straight to the creator's feed.
	ScopeDirectPost = "video.publish"
	// ScopeUploadDraft allows uploading inbox drafts for creator review.
	ScopeUploadDraft = "video.upload"
)

// chunkSize is 10 MB, inside TikTok's 5-64 MB chunk window.
const chunkSize = 10 * 1024 * 1024

// maxSingleChunk: files up to 64 MB upload whole, as one chunk (TikTok
// requires this below 5 MB and allows it up to 64 MB).
const maxSingleChunk = 64 * 1024 * 1024

// PostingClient is a TikTok Content Posting API client.
type PostingClient struct {
	ClientKey    string
	ClientSecret string
	Store        *TokenStore
	// HTTP is the injectable transport; nil means DefaultHTTP.
	HTTP HTTPFunc
	// PollInterval is the delay between status polls (default 15s, which
	// keeps the 6 req/min rate limit with headroom).
	PollInterval time.Duration
}

func (c *PostingClient) http() HTTPFunc {
	if c.HTTP != nil {
		return c.HTTP
	}
	return DefaultHTTP
}

func (c *PostingClient) pollInterval() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return 15 * time.Second
}

// NewPKCE returns a code_verifier and its TikTok code_challenge. TikTok's
// Desktop Login Kit (the localhost-redirect flow) requires PKCE and, unlike
// RFC 7636, expects the challenge as HEX-encoded SHA-256 of the verifier.

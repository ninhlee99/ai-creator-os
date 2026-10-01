// Package tiktok ports the Python tiktok/ tree (posting + shop clients)
// to Go. Stdlib only, no cgo, no Python calls.
package tiktok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
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

// minChunkedSize mirrors the Python threshold: files below 5 MB upload as a
// single chunk.
const minChunkedSize = 5 * 1024 * 1024

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

// AuthorizeURL builds the Login Kit authorization URL.
func (c *PostingClient) AuthorizeURL(redirectURI, scope, state string) string {
	q := url.Values{}
	q.Set("client_key", c.ClientKey)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scope)
	if state != "" {
		q.Set("state", state)
	}
	return AuthURL + "?" + q.Encode()
}

// ExchangeCode trades an authorization code for tokens and persists them.
func (c *PostingClient) ExchangeCode(code, redirectURI string) (map[string]any, error) {
	form := url.Values{}
	form.Set("client_key", c.ClientKey)
	form.Set("client_secret", c.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	_, raw, err := c.http()("POST", Base+"/v2/oauth/token/",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()))
	if err != nil {
		return nil, err
	}
	data := decodeJSON(raw)
	t := Tokens{}
	if v, ok := data["access_token"].(string); ok {
		t.AccessToken = v
	}
	if v, ok := data["refresh_token"].(string); ok {
		t.RefreshToken = v
	}
	if v, ok := data["expires_in"].(float64); ok {
		t.ExpiresIn = int64(v)
	}
	if err := c.Store.Save(t); err != nil {
		return nil, err
	}
	return data, nil
}

// AccessToken returns a fresh access token, refreshing (and persisting the
// rotated refresh_token) when the stored one is expired or near expiry.
func (c *PostingClient) AccessToken() (string, error) {
	t, err := c.Store.Load()
	if err != nil {
		return "", err
	}
	if t.AccessToken != "" && time.Now().Unix() < t.ObtainedAt+t.ExpiresIn-60 {
		return t.AccessToken, nil
	}
	if t.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token: complete OAuth flow first")
	}
	form := url.Values{}
	form.Set("client_key", c.ClientKey)
	form.Set("client_secret", c.ClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", t.RefreshToken)
	_, raw, err := c.http()("POST", Base+"/v2/oauth/token/",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()))
	if err != nil {
		return "", err
	}
	data := decodeJSON(raw)
	nt := Tokens{}
	if v, ok := data["access_token"].(string); ok {
		nt.AccessToken = v
	}
	if v, ok := data["refresh_token"].(string); ok {
		nt.RefreshToken = v
	}
	if v, ok := data["expires_in"].(float64); ok {
		nt.ExpiresIn = int64(v)
	}
	// Persist the ROTATED refresh_token.
	if err := c.Store.Save(nt); err != nil {
		return "", err
	}
	if nt.AccessToken == "" {
		return "", fmt.Errorf("oauth refresh returned no access_token: %.200v", data)
	}
	return nt.AccessToken, nil
}

func (c *PostingClient) auth() (map[string]string, error) {
	tok, err := c.AccessToken()
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": "Bearer " + tok,
		"Content-Type":  "application/json; charset=UTF-8",
	}, nil
}

// CreatorInfo returns max duration / allowed privacy levels — call before init.
func (c *PostingClient) CreatorInfo() (map[string]any, error) {
	h, err := c.auth()
	if err != nil {
		return nil, err
	}
	_, raw, err := c.http()("POST", Base+"/v2/post/publish/creator_info/", h, []byte("{}"))
	if err != nil {
		return nil, err
	}
	data := decodeJSON(raw)
	if v, ok := nested(data, "data"); ok {
		if m, ok := v.(map[string]any); ok {
			return m, nil
		}
	}
	return data, nil
}

// PublishFile uploads a local mp4 and (Direct Post) publishes it, returning
// the publish_id. privacyLevel defaults to SELF_ONLY until the Direct Post
// audit passes.
func (c *PostingClient) PublishFile(path, title, privacyLevel string, draft bool) (string, error) {
	if privacyLevel == "" {
		privacyLevel = "SELF_ONLY"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	size := fi.Size()

	cs := int64(chunkSize)
	if size < minChunkedSize {
		cs = size
	}
	chunks := 1
	if size >= minChunkedSize {
		chunks = int(math.Ceil(float64(size) / float64(chunkSize)))
	}

	endpoint := "/v2/post/publish/video/init/"
	if draft {
		endpoint = "/v2/post/publish/inbox/video/init/"
	}
	h, err := c.auth()
	if err != nil {
		return "", err
	}
	initBody, _ := json.Marshal(map[string]any{
		"post_info": map[string]any{
			"title":           title,
			"privacy_level":   privacyLevel,
			"disable_comment": false,
			"disable_duet":    false,
			"disable_stitch":  false,
		},
		"source_info": map[string]any{
			"source":            "FILE_UPLOAD",
			"video_size":        size,
			"chunk_size":        cs,
			"total_chunk_count": chunks,
		},
	})
	_, raw, err := c.http()("POST", Base+endpoint, h, initBody)
	if err != nil {
		return "", err
	}
	data := decodeJSON(raw)
	if code, _ := nestedString(data, "error", "code"); code != "ok" {
		return "", fmt.Errorf("init failed: %v", mustMap(data["error"]))
	}
	publishID, ok := nestedString(data, "data", "publish_id")
	if !ok || publishID == "" {
		return "", fmt.Errorf("init failed: no publish_id in %.300v", data)
	}
	uploadURL, ok := nestedString(data, "data", "upload_url")
	if !ok || uploadURL == "" {
		return "", fmt.Errorf("init failed: no upload_url in %.300v", data)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, chunkSize)
	for i := 0; i < chunks; i++ {
		n, rerr := io.ReadFull(f, buf)
		if rerr != nil && rerr != io.ErrUnexpectedEOF && rerr != io.EOF {
			return "", rerr
		}
		blob := buf[:n]
		first := int64(i) * chunkSize
		last := first + int64(len(blob)) - 1
		status, _, perr := c.http()("PUT", uploadURL,
			map[string]string{
				"Content-Type":  "video/mp4",
				"Content-Range": fmt.Sprintf("bytes %d-%d/%d", first, last, size),
			}, blob)
		if perr != nil {
			return "", fmt.Errorf("chunk %d upload failed: %w", i, perr)
		}
		if status != 200 && status != 201 && status != 206 {
			return "", fmt.Errorf("chunk %d upload failed: HTTP %d", i, status)
		}
		if n == 0 {
			break
		}
	}
	return publishID, nil
}

// PollStatus polls until PUBLISH_COMPLETE / FAILED. The default 15s interval
// keeps the client under the 6 req/min publish-endpoint rate limit.
func (c *PostingClient) PollStatus(publishID string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	h, err := c.auth()
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"publish_id": publishID})
	for {
		_, raw, err := c.http()("POST", Base+"/v2/post/publish/status/fetch/", h, body)
		if err != nil {
			return "", err
		}
		data := decodeJSON(raw)
		st, _ := nestedString(data, "data", "status")
		if st == "PUBLISH_COMPLETE" || st == "FAILED" {
			return st, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("publish %s not done in %s (last: %s)", publishID, timeout, st)
		}
		time.Sleep(c.pollInterval())
	}
}

// mustMap renders an arbitrary value for error messages.
func mustMap(v any) any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

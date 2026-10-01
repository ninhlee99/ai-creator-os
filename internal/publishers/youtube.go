package publishers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// YouTube OAuth + upload endpoints (ports the Python module constants).
const (
	youtubeOAuthTokenURL = "https://oauth2.googleapis.com/token"
	youtubeUploadURL     = "https://www.googleapis.com/upload/youtube/v3/videos" +
		"?uploadType=resumable&part=snippet,status"
)

// CategoryByKind maps content kinds to YouTube category IDs, mirroring the
// Python CATEGORY_BY_KIND table.
var CategoryByKind = map[string]string{
	"short_film":  "24", // Entertainment
	"ai_music":    "10", // Music
	"ai_remix":    "10", // Music
	"short_video": "22", // People & Blogs
}

// YouTubeHTTPFunc is the injectable transport for the YouTube publisher. It
// also returns response headers because the resumable-upload session URL
// comes back in the Location header.
type YouTubeHTTPFunc func(method, url string, headers map[string]string, body []byte) (status int, respHeaders map[string]string, respBody []byte, err error)

// defaultYouTubeHTTP is the net/http-backed transport (replaces urllib).
func defaultYouTubeHTTP(method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return 0, nil, nil, err
	}
	h := map[string]string{}
	for k, vv := range resp.Header {
		if len(vv) > 0 {
			h[k] = vv[0]
			h[strings.ToLower(k)] = vv[0]
		}
	}
	if resp.StatusCode >= 400 {
		snip := raw
		if len(snip) > 500 {
			snip = snip[:500]
		}
		return resp.StatusCode, h, nil,
			fmt.Errorf("HTTP %d %s: %q", resp.StatusCode, url, snip)
	}
	return resp.StatusCode, h, raw, nil
}

// YouTubePublisher uploads via Data API v3 resumable upload, per-account
// channel. Each account declares which content kinds its channel accepts
// (e.g. a story channel takes short_film, a music channel takes ai_music +
// ai_remix).
//
// Credentials: one OAuth client (YOUTUBE_CLIENT_ID / YOUTUBE_CLIENT_SECRET),
// one refresh-token file per account: youtube_token_<username>.json
// (gitignored, never logged). Scope: youtube.upload.
//
// Default privacy is "private" — flip with YOUTUBE_DEFAULT_PRIVACY=public
// (or unlisted) once the channel is reviewed.
type YouTubePublisher struct {
	Username     string
	allowedKinds []string
	channel      string
	tokenPath    string
	clientID     string
	clientSecret string
	privacy      string
	// HTTP is the injectable transport; nil means defaultYouTubeHTTP.
	HTTP YouTubeHTTPFunc
}

// YouTubeTokenPath is the refresh-token file for username.
func YouTubeTokenPath(username string) string {
	return "youtube_token_" + sanitizeUsername(username) + ".json"
}

// NewYouTubePublisher wires credentials from the environment:
// YOUTUBE_CLIENT_ID[_<USERNAME>], YOUTUBE_CLIENT_SECRET[_<USERNAME>],
// YOUTUBE_DEFAULT_PRIVACY (default "private").
func NewYouTubePublisher(username string, allowedKinds []string, channel string) *YouTubePublisher {
	privacy := os.Getenv("YOUTUBE_DEFAULT_PRIVACY")
	if privacy == "" {
		privacy = "private"
	}
	return &YouTubePublisher{
		Username:     username,
		allowedKinds: append([]string(nil), allowedKinds...),
		channel:      channel,
		tokenPath:    YouTubeTokenPath(username),
		clientID:     envFor(username, "YOUTUBE_CLIENT_ID"),
		clientSecret: envFor(username, "YOUTUBE_CLIENT_SECRET"),
		privacy:      privacy,
	}
}

// Name returns the platform key.
func (p *YouTubePublisher) Name() string { return "youtube" }

// IsConfigured requires OAuth client credentials, a token file, and a
// non-empty allowed-kinds list for the channel.
func (p *YouTubePublisher) IsConfigured() bool {
	if p.clientID == "" || p.clientSecret == "" || len(p.allowedKinds) == 0 {
		return false
	}
	_, err := os.Stat(p.tokenPath)
	return err == nil
}

// Handles reports whether the account's channel accepts the content kind.
func (p *YouTubePublisher) Handles(kind string) bool { return handlesKind(kind, p.allowedKinds) }

// Channel returns the account's YouTube channel, if any.
func (p *YouTubePublisher) Channel() string { return p.channel }

func (p *YouTubePublisher) http() YouTubeHTTPFunc {
	if p.HTTP != nil {
		return p.HTTP
	}
	return defaultYouTubeHTTP
}

func (p *YouTubePublisher) refreshToken() string {
	raw, err := os.ReadFile(p.tokenPath)
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	s, _ := data["refresh_token"].(string)
	return s
}

// accessToken exchanges the stored refresh token for an access token.
func (p *YouTubePublisher) accessToken() (string, error) {
	refresh := p.refreshToken()
	if refresh == "" {
		return "", fmt.Errorf("no youtube refresh token stored")
	}
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("refresh_token", refresh)
	form.Set("grant_type", "refresh_token")
	status, _, raw, err := p.http()("POST", youtubeOAuthTokenURL,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()))
	if err != nil {
		return "", err
	}
	var data map[string]any
	if jerr := json.Unmarshal(raw, &data); jerr != nil {
		return "", fmt.Errorf("oauth refresh failed: HTTP %d (bad JSON)", status)
	}
	tok, _ := data["access_token"].(string)
	if status != 200 || tok == "" {
		return "", fmt.Errorf("oauth refresh failed: HTTP %d %.200v", status, data)
	}
	return tok, nil
}

// Publish uploads the video via resumable upload. Expected API failures
// come back as PublishResult{Ok:false}.
func (p *YouTubePublisher) Publish(ctx context.Context, videoPath, title, description, kind string) PublishResult {
	if !p.IsConfigured() {
		return PublishResult{Ok: false, Platform: p.Name(), Error: "youtube not configured"}
	}
	if !p.Handles(kind) {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: fmt.Sprintf("kind %s not enabled for this channel", kind)}
	}
	fi, err := os.Stat(videoPath)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	size := fi.Size()
	token, err := p.accessToken()
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	if title == "" {
		title = "AI video"
	}
	category := CategoryByKind[kind]
	if category == "" {
		category = "22"
	}
	meta, _ := json.Marshal(map[string]any{
		"snippet": map[string]any{
			"title":       truncate(title, 100),
			"description": truncate(description, 5000),
			"categoryId":  category,
		},
		"status": map[string]any{
			"privacyStatus":           p.privacy,
			"selfDeclaredMadeForKids": false,
		},
	})
	status, headers, raw, err := p.http()("POST", youtubeUploadURL,
		map[string]string{
			"Authorization":           "Bearer " + token,
			"Content-Type":            "application/json; charset=UTF-8",
			"X-Upload-Content-Length": fmt.Sprintf("%d", size),
			"X-Upload-Content-Type":   "video/mp4",
		}, meta)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	session := headers["Location"]
	if session == "" {
		session = headers["location"]
	}
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	if status != 200 || session == "" {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: truncate(fmt.Sprintf("upload init failed: HTTP %d %.200v", status, data), 300)}
	}
	blob, err := os.ReadFile(videoPath)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	status, _, raw, err = p.http()("PUT", session,
		map[string]string{
			"Content-Length": fmt.Sprintf("%d", size),
			"Content-Range":  fmt.Sprintf("bytes 0-%d/%d", size-1, size),
		}, blob)
	if err != nil {
		return PublishResult{Ok: false, Platform: p.Name(), Error: truncate(err.Error(), 300)}
	}
	data = nil
	_ = json.Unmarshal(raw, &data)
	if status != 200 && status != 201 {
		return PublishResult{Ok: false, Platform: p.Name(),
			Error: truncate(fmt.Sprintf("upload failed: HTTP %d %.200v", status, data), 300)}
	}
	var vid, videoURL string
	if data != nil {
		vid, _ = data["id"].(string)
	}
	if vid != "" {
		videoURL = "https://youtu.be/" + vid
	}
	return PublishResult{Ok: true, Platform: p.Name(), RemoteID: vid,
		URL: videoURL, Draft: p.privacy != "public"}
}

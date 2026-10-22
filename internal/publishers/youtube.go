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
	// Synthetic marks uploads as AI-generated content: the API's
	// containsSyntheticMedia disclosure is set on insert. Growth
	// publishing always sets this (the flag is mandatory, never off).
	Synthetic bool
	// HTTP is the injectable transport; nil means defaultYouTubeHTTP.
	HTTP YouTubeHTTPFunc
}

// YouTubeTokenPath is the refresh-token file for username, inside the
// app's token directory (<dataDir>/tokens, see SetTokenDir).
func YouTubeTokenPath(username string) string {
	return tokenPath("youtube_token_" + sanitizeUsername(username) + ".json")
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

// HasClient reports whether the OAuth client ID + secret are set.
// Đợt AA: dùng cho nút "Nối YouTube" ở trang Đa nền tảng.
func (p *YouTubePublisher) HasClient() bool { return p.clientID != "" && p.clientSecret != "" }

// ClientID/ClientSecret expose the OAuth client credentials for the
// in-app connect flow (never rendered to a page).
func (p *YouTubePublisher) ClientID() string     { return p.clientID }
func (p *YouTubePublisher) ClientSecret() string { return p.clientSecret }

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
		// Đợt V: phân biệt "token chết" với "mạng lỗi". invalid_grant =
		// refresh token bị thu hồi hoặc hết hạn (app OAuth ở testing mode
		// thì refresh token hết hạn sau 7 ngày) — retry vô ích, cần Ninh
		// nối lại YouTube.
		if strings.Contains(string(raw), "invalid_grant") {
			return "", ErrOAuthRevoked
		}
		return "", fmt.Errorf("oauth refresh failed: HTTP %d %.200v", status, data)
	}
	return tok, nil
}

// ErrOAuthRevoked: refresh token YouTube hết hạn hoặc bị thu hồi —
// retry không có tác dụng, cần người dùng nối lại YouTube.
var ErrOAuthRevoked = fmt.Errorf("youtube: refresh token hết hạn hoặc bị thu hồi — cần nối lại YouTube ở trang Đa nền tảng")

// FindRecentByTitle kiểm tra kênh có video nào cùng tiêu đề (khớp gần đúng)
// được đăng trong khoảng `within` trở lại đây không (Đợt V — chống đăng
// trùng khi upload dở dang rồi retry). Dùng search.list forMine (tốn quota
// ~100 units/lượt) nên chỉ gọi khi thật cần (marker publishing cũ).
func (p *YouTubePublisher) FindRecentByTitle(ctx context.Context, title string, within time.Duration) (bool, error) {
	tok, err := p.accessToken()
	if err != nil {
		return false, err
	}
	after := time.Now().Add(-within).UTC().Format(time.RFC3339)
	q := url.Values{}
	q.Set("part", "snippet")
	q.Set("forMine", "true")
	q.Set("type", "video")
	q.Set("q", title)
	q.Set("publishedAfter", after)
	q.Set("maxResults", "5")
	q.Set("order", "date")
	status, _, raw, err := p.http()("GET", "https://www.googleapis.com/youtube/v3/search?"+q.Encode(),
		map[string]string{"Authorization": "Bearer " + tok}, nil)
	if err != nil {
		return false, err
	}
	if status != 200 {
		return false, fmt.Errorf("youtube search: HTTP %d", status)
	}
	var data struct {
		Items []struct {
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false, fmt.Errorf("youtube search: bad JSON: %w", err)
	}
	want := strings.ToLower(strings.TrimSpace(title))
	for _, it := range data.Items {
		if strings.ToLower(strings.TrimSpace(it.Snippet.Title)) == want {
			return true, nil
		}
	}
	return false, nil
}

// YouTubeAccessToken đổi refresh token đã lưu của username thành access
// token (dùng cho YouTube Analytics API — cùng OAuth client với upload).
// Fail-closed: chưa có token file / client id → lỗi rõ ràng, không gọi mạng.
//
// LƯU Ý QUYỀN: Analytics cần scope yt-analytics.readonly. Token cũ chỉ có
// youtube.upload sẽ bị API trả 403 — Fetch của growth báo rõ để Ninh chạy
// lại scripts/get-youtube-token.py với scope mới.
func YouTubeAccessToken(username string) (string, error) {
	p := NewYouTubePublisher(username, nil, "")
	if p.clientID == "" || p.clientSecret == "" {
		return "", fmt.Errorf("thiếu YOUTUBE_CLIENT_ID / YOUTUBE_CLIENT_SECRET")
	}
	return p.accessToken()
}

// Publish uploads the video via resumable upload. Expected API failures
// come back as PublishResult{Ok:false}.

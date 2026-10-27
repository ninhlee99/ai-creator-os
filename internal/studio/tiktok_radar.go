package studio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TikTokRadarSourceLabel reports which provider the current process will use.
func TikTokRadarSourceLabel() string {
	return TikTokRadarSourceLabelForTopic(false)
}

// TikTokRadarSourceConfigured reports whether the automated crawler is ready.
func TikTokRadarSourceConfigured() bool {
	return TikTokCrawlerConfigured()
}

// TikTokRadarSourceLabelForTopic reports the provider selected for a regional
// feed or a user-entered topic search.
func TikTokRadarSourceLabelForTopic(_ bool) string {
	if TikTokCrawlerConfigured() {
		return TikTokCrawlerSource
	}
	return "Chưa tìm thấy trình duyệt để chạy TikTok Crawler"
}

// TikTokTrendVideo holds metadata returned by the configured TikTok feed provider.
type TikTokTrendVideo struct {
	Rank            int     `json:"rank"`
	VideoID         string  `json:"video_id"`
	Title           string  `json:"title"`
	Caption         string  `json:"caption,omitempty"`
	Region          string  `json:"region,omitempty"`
	Author          string  `json:"author"`
	UniqueID        string  `json:"unique_id"`
	Views           string  `json:"views"`
	PlayCount       int64   `json:"play_count"`
	PlayCountKnown  bool    `json:"play_count_known"`
	Provider        string  `json:"provider,omitempty"`
	Likes           string  `json:"likes"`
	DiggCount       int64   `json:"digg_count"`
	DiggCountKnown  bool    `json:"digg_count_known"`
	Comments          string  `json:"comments,omitempty"`
	CommentCount      int64   `json:"comment_count"`
	CommentKnown      bool    `json:"comment_count_known"`
	CommentCountKnown bool    `json:"comment_count_known_alias,omitempty"`
	Shares          string  `json:"shares,omitempty"`
	ShareCount      int64   `json:"share_count"`
	ShareCountKnown bool    `json:"share_count_known"`
	Music           string  `json:"music"`
	VideoURL        string  `json:"video_url"`
	CoverURL        string  `json:"cover_url"`
	CollectedAt     string  `json:"collected_at,omitempty"`
	DanceVerified   bool    `json:"dance_verified,omitempty"`
	DanceVerifiedAt string  `json:"dance_verified_at,omitempty"`
	ViewsPerHour    float64 `json:"views_per_hour,omitempty"`
	VelocityKnown   bool    `json:"velocity_known"`
	EngagementRate  float64 `json:"engagement_rate,omitempty"`
	EngagementKnown bool    `json:"engagement_known"`
}

func (v TikTokTrendVideo) SoundTitle() string {
	return strings.TrimSpace(v.Music)
}

// EmbedPlayerURL returns TikTok's official player URL only when the saved
// source URL contains the same numeric video ID.
func (v TikTokTrendVideo) EmbedPlayerURL() string {
	id := TikTokVideoIDFromURL(v.VideoURL)
	if id == "" || id != strings.TrimSpace(v.VideoID) {
		return ""
	}
	return "https://www.tiktok.com/player/v1/" + id + "?controls=1&description=1&music_info=1&rel=0"
}

const TikTokTrendSource = "TikTok Radar"

var tikTokRadarHTTPTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Resolver: &net.Resolver{PreferGo: true}}).DialContext
	return transport
}()

// Cache TTL
const (
	tikTokTrendsTTL            = 15 * time.Minute
	tikTokRadarFeedLimit       = 8
	tikTokRadarTopicCacheLimit = 64
	tikTokRadarMaxTopics       = 3
)

var (
	ttMu           sync.Mutex
	cachedTTTrends []TikTokTrendVideo
	lastTTFetch    time.Time
	topicTTCache   = make(map[string]cachedTikTokRadarSearch)
)

type cachedTikTokRadarSearch struct {
	videos    []TikTokTrendVideo
	fetchedAt time.Time
}

// FormatCount định dạng số lượt xem / like thành chuỗi K/M dễ đọc (vd: 14.5M, 850K).
func FormatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// FetchTikTokTrendingVideos returns records from TikTok Web Crawler.
func FetchTikTokTrendingVideos(ctx context.Context) ([]TikTokTrendVideo, error) {
	if !TikTokCrawlerConfigured() {
		return nil, missingTikTokRadarConfigError()
	}
	ttMu.Lock()
	if time.Since(lastTTFetch) < tikTokTrendsTTL && len(cachedTTTrends) > 0 {
		defer ttMu.Unlock()
		return append([]TikTokTrendVideo(nil), cachedTTTrends...), nil
	}
	ttMu.Unlock()

	result, err := fetchTikTokCrawlerVideos(ctx, "")
	if err != nil {
		return nil, err
	}
	live := validateTikTokRadarResults(result)
	if len(live) == 0 {
		return nil, fmt.Errorf("nguồn TikTok không trả về dữ liệu hiện tại")
	}
	for i := range live {
		live[i].Rank = i + 1
	}
	sortTikTokRadarVideosByPopularity(live)
	if len(live) > tikTokRadarFeedLimit {
		live = live[:tikTokRadarFeedLimit]
	}
	for i := range live {
		live[i].Rank = i + 1
	}

	ttMu.Lock()
	cachedTTTrends = append([]TikTokTrendVideo(nil), live...)
	lastTTFetch = time.Now()
	ttMu.Unlock()
	return live, nil
}

// FetchTikTokVideosForTopic queries TikTok Web Crawler by topic. Empty
// topic keeps the regional feed behavior; non-empty topics never silently
// fall back to caption filtering over that small feed.
func FetchTikTokVideosForTopic(ctx context.Context, topic string) ([]TikTokTrendVideo, error) {
	topic = strings.Join(strings.Fields(strings.TrimSpace(topic)), " ")
	if topic == "" {
		return FetchTikTokTrendingVideos(ctx)
	}
	if len([]rune(topic)) > 120 {
		return nil, fmt.Errorf("chủ đề tìm kiếm tối đa 120 ký tự")
	}
	topics, err := splitTikTokRadarTopics(topic)
	if err != nil {
		return nil, err
	}
	topic = strings.Join(topics, ", ")
	if !TikTokCrawlerConfigured() {
		return nil, missingTikTokRadarConfigError()
	}
	key := "crawler:VN:" + strings.ToLower(topic)
	ttMu.Lock()
	if cached, ok := topicTTCache[key]; ok && time.Since(cached.fetchedAt) < tikTokTrendsTTL {
		videos := append([]TikTokTrendVideo(nil), cached.videos...)
		ttMu.Unlock()
		return videos, nil
	}
	ttMu.Unlock()

	result, err := fetchTikTokCrawlerVideos(ctx, topic)
	if err != nil {
		return nil, err
	}
	live := validateTikTokRadarResults(result)
	for i := range live {
		if live[i].Rank == 0 {
			live[i].Rank = i + 1
		}
	}
	sortTikTokRadarVideosByPopularity(live)
	if len(live) > tikTokRadarFeedLimit {
		live = live[:tikTokRadarFeedLimit]
	}
	for i := range live {
		live[i].Rank = i + 1
	}
	ttMu.Lock()
	if _, exists := topicTTCache[key]; !exists && len(topicTTCache) >= tikTokRadarTopicCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for cacheKey, entry := range topicTTCache {
			if oldestKey == "" || entry.fetchedAt.Before(oldest) {
				oldestKey, oldest = cacheKey, entry.fetchedAt
			}
		}
		delete(topicTTCache, oldestKey)
	}
	topicTTCache[key] = cachedTikTokRadarSearch{videos: append([]TikTokTrendVideo(nil), live...), fetchedAt: time.Now()}
	ttMu.Unlock()
	return live, nil
}

func missingTikTokRadarConfigError() error {
	return fmt.Errorf("không tìm thấy trình duyệt (Microsoft Edge hoặc Google Chrome) trên hệ thống để chạy TikTok Crawler")
}

func splitTikTokRadarTopics(topic string) ([]string, error) {
	parts := strings.Split(topic, ",")
	topics := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		topics = append(topics, value)
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("nhập ít nhất một chủ đề TikTok")
	}
	if len(topics) > tikTokRadarMaxTopics {
		return nil, fmt.Errorf("mỗi lần tìm tối đa %d chủ đề, ngăn cách bằng dấu phẩy", tikTokRadarMaxTopics)
	}
	return topics, nil
}

func validateTikTokRadarResults(items []TikTokTrendVideo) []TikTokTrendVideo {
	live := make([]TikTokTrendVideo, 0, len(items))
	for _, item := range items {
		item.VideoID = strings.TrimSpace(item.VideoID)
		item.VideoURL = strings.TrimSpace(item.VideoURL)
		if item.VideoID == "" || !IsTikTokVideoURL(item.VideoURL) || tiktokVideoID(item.VideoURL) != item.VideoID {
			continue
		}
		item.CollectedAt = time.Now().UTC().Format(time.RFC3339)
		if strings.TrimSpace(item.Provider) == "" {
			item.Provider = TikTokTrendSource
		}
		live = append(live, item)
	}
	return live
}

// ResolveTikTokRadarMusic is intentionally fail-closed. Unattended jobs may
// only use a recent video that an operator explicitly verified as dance.
func ResolveTikTokRadarMusic(context.Context) (TikTokTrendVideo, error) {
	return TikTokTrendVideo{}, fmt.Errorf("Radar chưa có video nhảy được xác nhận; chọn video nhảy thủ công trong Radar TikTok trước khi chạy affiliate tự động")
}

// ResolveTikTokRadarVideo confirms the submitted source was collected by
// Radar. Callers use returned metadata, never form text.
func ResolveTikTokRadarVideo(ctx context.Context, sourceURL string) (TikTokTrendVideo, error) {
	if !IsTikTokVideoURL(sourceURL) {
		return TikTokTrendVideo{}, fmt.Errorf("nguồn phải là video TikTok trong Radar")
	}
	videos, err := FetchTikTokTrendingVideos(ctx)
	if err != nil {
		return TikTokTrendVideo{}, err
	}
	return findTikTokRadarVideo(videos, sourceURL)
}

func findTikTokRadarVideo(videos []TikTokTrendVideo, sourceURL string) (TikTokTrendVideo, error) {
	videoID := tiktokVideoID(sourceURL)
	if videoID == "" {
		return TikTokTrendVideo{}, fmt.Errorf("URL không trỏ tới video TikTok đã thu thập trong Radar")
	}
	for _, video := range videos {
		if video.VideoID != videoID {
			continue
		}
		return video, nil
	}
	return TikTokTrendVideo{}, fmt.Errorf("video không nằm trong kết quả Radar hiện tại; làm mới Radar rồi chọn lại")
}

func tiktokVideoID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "video" {
			return strings.TrimSpace(parts[i+1])
		}
	}
	return ""
}

// TikTokVideoIDFromURL returns the numeric public-video ID from a full TikTok
// video URL. Short share URLs without a /video/{id} path are not accepted.
func TikTokVideoIDFromURL(raw string) string {
	if !IsTikTokVideoURL(raw) {
		return ""
	}
	id := tiktokVideoID(raw)
	if id == "" {
		return ""
	}
	for _, digit := range id {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	return id
}

// IsTikTokVideoURL accepts HTTPS TikTok URLs only.
func IsTikTokVideoURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	if port := u.Port(); port != "" && port != "443" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com")
}

func doTikTokRadarRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 20 * time.Second, Transport: tikTokRadarHTTPTransport}
	resp, err := client.Do(req)
	if err == nil || runtime.GOOS != "darwin" || !strings.Contains(err.Error(), "x509: OSStatus -26276") {
		return resp, err
	}
	fallbackClient, fallbackErr := newTikTokRadarFallbackClient()
	if fallbackErr != nil {
		return nil, fmt.Errorf("TikTok Radar không truy cập được xác minh chứng chỉ macOS; CA bundle fallback lỗi: %v: %w", fallbackErr, err)
	}
	retry := req.Clone(req.Context())
	return fallbackClient.Do(retry)
}

// newTikTokRadarFallbackClient retries only the macOS trustd sandbox failure
// with the OS CA bundle. TLS certificate and hostname checks stay enabled.
func newTikTokRadarFallbackClient() (*http.Client, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("fallback CA bundle chỉ hỗ trợ macOS")
	}
	pem, err := os.ReadFile("/etc/ssl/cert.pem")
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle macOS không có chứng chỉ hợp lệ")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Resolver: &net.Resolver{PreferGo: true}}).DialContext
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	return &http.Client{Timeout: 20 * time.Second, Transport: transport}, nil
}

func parseTikTokRadarFeed(r io.Reader) ([]TikTokTrendVideo, error) {
	var payload struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		if strings.TrimSpace(payload.Msg) != "" {
			return nil, fmt.Errorf("nguồn Radar TikTok: %s", strings.TrimSpace(payload.Msg))
		}
		return nil, fmt.Errorf("nguồn Radar TikTok trả mã lỗi %d", payload.Code)
	}

	type feedItem struct {
		VideoID      string `json:"video_id"`
		Title        string `json:"title"`
		Region       string `json:"region"`
		PlayCount    *int64 `json:"play_count"`
		DiggCount    *int64 `json:"digg_count"`
		CommentCount *int64 `json:"comment_count"`
		ShareCount   *int64 `json:"share_count"`
		Cover        string `json:"cover"`
		Author       struct {
			Nickname string `json:"nickname"`
			UniqueID string `json:"unique_id"`
		} `json:"author"`
		Music     string `json:"music"`
		MusicInfo struct {
			Title string `json:"title"`
		} `json:"music_info"`
	}
	var items []feedItem
	data := strings.TrimSpace(string(payload.Data))
	if strings.HasPrefix(data, "[") {
		if err := json.Unmarshal(payload.Data, &items); err != nil {
			return nil, err
		}
	} else if data != "" && data != "null" {
		var searchData struct {
			Videos []feedItem `json:"videos"`
		}
		if err := json.Unmarshal(payload.Data, &searchData); err != nil {
			return nil, err
		}
		items = searchData.Videos
	}

	var results []TikTokTrendVideo
	for _, item := range items {
		if strings.TrimSpace(item.VideoID) == "" {
			continue
		}
		caption := strings.TrimSpace(item.Title)
		title := caption
		if title == "" {
			title = fmt.Sprintf("Video TikTok của @%s", item.Author.UniqueID)
		}
		music := strings.TrimSpace(item.MusicInfo.Title)
		if music == "" {
			music = strings.TrimSpace(item.Music)
		}

		videoURL := fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", item.Author.UniqueID, item.VideoID)
		if item.Author.UniqueID == "" {
			videoURL = fmt.Sprintf("https://www.tiktok.com/video/%s", item.VideoID)
		}
		video := TikTokTrendVideo{
			VideoID:  item.VideoID,
			Title:    title,
			Caption:  caption,
			Region:   strings.TrimSpace(item.Region),
			Author:   item.Author.Nickname,
			UniqueID: item.Author.UniqueID,
			Music:    music,
			VideoURL: videoURL,
			CoverURL: item.Cover,
		}
		if item.PlayCount != nil {
			video.PlayCount = *item.PlayCount
			video.PlayCountKnown = true
			video.Views = FormatCount(*item.PlayCount)
		}
		if item.DiggCount != nil {
			video.DiggCount = *item.DiggCount
			video.DiggCountKnown = true
			video.Likes = FormatCount(*item.DiggCount)
		}
		if item.CommentCount != nil {
			video.CommentCount = *item.CommentCount
			video.CommentKnown = true
			video.Comments = FormatCount(*item.CommentCount)
		}
		if item.ShareCount != nil {
			video.ShareCount = *item.ShareCount
			video.ShareCountKnown = true
			video.Shares = FormatCount(*item.ShareCount)
		}
		results = append(results, video)
	}

	return results, nil
}

// InvalidateTikTokRadarCache clears the regional video feed cache.
func InvalidateTikTokRadarCache() {
	ttMu.Lock()
	lastTTFetch = time.Time{}
	cachedTTTrends = nil
	topicTTCache = make(map[string]cachedTikTokRadarSearch)
	ttMu.Unlock()
}

package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// TikTokTrendVideo holds metadata returned by the configured TikTok feed provider.
type TikTokTrendVideo struct {
	Rank          int    `json:"rank"`
	VideoID       string `json:"video_id"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	UniqueID      string `json:"unique_id"`
	Views         string `json:"views"`
	PlayCount     int64  `json:"play_count"`
	Likes         string `json:"likes"`
	DiggCount     int64  `json:"digg_count"`
	Music         string `json:"music"`
	VideoURL      string `json:"video_url"`
	CoverURL      string `json:"cover_url"`
	PlayURL       string `json:"play_url"`
	SuggestedHook string `json:"suggested_hook"`
	Topic         string `json:"topic,omitempty"`
	SearchVolume  string `json:"search_volume,omitempty"`
}

// ShowbizDrama đại diện cho 1 video drama nóng từ TikTok (Việt Nam) hoặc Douyin (Trung Quốc).
type ShowbizDrama struct {
	Rank        int    `json:"rank"`
	Title       string `json:"title"`
	Category    string `json:"category"` // "Showbiz Việt" hoặc "Showbiz Trung"
	Region      string `json:"region"`   // "VN" hoặc "CN"
	Platform    string `json:"platform"` // "TikTok VN" hoặc "Douyin CN"
	Summary     string `json:"summary"`
	ImageURL    string `json:"image_url"`
	CoverURL    string `json:"cover_url"`
	PlayURL     string `json:"play_url"`
	SourceURL   string `json:"source_url"`
	VideoURL    string `json:"video_url"`
	Author      string `json:"author"`
	UniqueID    string `json:"unique_id"`
	Views       string `json:"views"`
	Likes       string `json:"likes"`
	Music       string `json:"music"`
	PublishedAt string `json:"published_at"`
	Hook3s      string `json:"hook_3s"`
}

// Cache TTL
const (
	tikTokTrendsTTL = 15 * time.Minute
	showbizDramaTTL = 15 * time.Minute
)

var (
	ttMu           sync.Mutex
	cachedTTTrends []TikTokTrendVideo
	lastTTFetch    time.Time

	dramaMu        sync.Mutex
	cachedDramasVN []ShowbizDrama
	cachedDramasCN []ShowbizDrama
	lastDramaFetch time.Time
	imgRegex       = regexp.MustCompile(`src=["']([^"']+)["']`)
	htmlTagRegex   = regexp.MustCompile(`<[^>]+>`)
)

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

// isVietnameseTikTok kiểm tra tiêu đề, tác giả hoặc bài hát có đúng là tiếng Việt / nội dung Việt Nam không.
// Loại bỏ triệt để video tiếng Thái, Trung, Hàn, Nga, Ả Rập, Tây Ban Nha, Myanmar, Ấn Độ...
func isVietnameseTikTok(title, author, music string) bool {
	combined := title + " " + author + " " + music

	// 1. Loại bỏ các ký tự ngoại ngữ đặc trưng
	for _, r := range combined {
		if unicode.Is(unicode.Thai, r) ||
			unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hangul, r) ||
			unicode.Is(unicode.Cyrillic, r) ||
			unicode.Is(unicode.Arabic, r) ||
			(r >= 0x1000 && r <= 0x109F) || // Burmese
			(r >= 0x0900 && r <= 0x097F) { // Devanagari / Hindi
			return false
		}
	}

	combLower := strings.ToLower(combined)

	// 2. Loại bỏ các hashtag / từ khóa tiếng nước ngoài
	foreignTags := []string{
		"#latinus", "#myanmar", "#thailand", "#mexico", "#cambodia", "guanajuato",
		"#nepal", "kasari banaune", "ayuuto", "hooyo", "#cheems", "#batnat",
		"#creatorsearchinsights", "monetize i'd",
	}
	for _, tag := range foreignTags {
		if strings.Contains(combLower, tag) {
			return false
		}
	}

	// 3. Có dấu tiếng Việt chuẩn
	const vnDiacritics = "àáảãạâầấẩẫậăằắẳẵặèéẻẽẹêềếểễệìíỉĩịòóỏõọôồốổỗộơờớởỡợùúủũụưừứửữựỳýỷỹỵđÀÁẢÃẠÂẦẤẨẪẬĂẰẮẲẴẶÈÉẺẼẸÊỀẾỂỄỆÌÍỈĨỊÒÓỎÕỌÔỒỐỔỖỘƠỜỚỞỠỢÙÚỦŨỤƯỪỨỬỮỰỲÝỶỸỴĐ"
	if strings.ContainsAny(combined, vnDiacritics) {
		return true
	}

	// 4. Có từ khóa văn hóa / nghệ sĩ / kênh Việt Nam không dấu
	vnKeywords := []string{
		"vietnam", "viet nam", "vpop", "v-pop", "tiktokvietnam", "vinahouse",
		"remix viet", "vtv", "vtvcab", "schannel", "dunghoangpham", "sontung", "m-tp",
		"hieuthuhai", "wren evans", "quang hung", "tang duy tan", "amee", "mono",
		"khoai lang thang", "khoailangthang", "den vau", "hoa minzy", "duc phuc",
	}
	for _, kw := range vnKeywords {
		if strings.Contains(combLower, kw) {
			return true
		}
	}

	return false
}

// FetchTikTokTrendingVideos returns only records received from the live TikTok feed.
func FetchTikTokTrendingVideos(ctx context.Context) ([]TikTokTrendVideo, error) {
	ttMu.Lock()
	if time.Since(lastTTFetch) < tikTokTrendsTTL && len(cachedTTTrends) > 0 {
		defer ttMu.Unlock()
		return append([]TikTokTrendVideo(nil), cachedTTTrends...), nil
	}
	ttMu.Unlock()

	result, err := fetchLiveTikTokVideos(ctx)
	if err != nil {
		return nil, err
	}
	var live []TikTokTrendVideo
	for _, item := range result {
		if !isVietnameseTikTok(item.Title, item.Author, item.Music) {
			continue
		}
		item.Rank = len(live) + 1
		live = append(live, item)
		if len(live) == 10 {
			break
		}
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("nguồn TikTok không trả về dữ liệu hiện tại")
	}

	ttMu.Lock()
	cachedTTTrends = append([]TikTokTrendVideo(nil), live...)
	lastTTFetch = time.Now()
	ttMu.Unlock()
	return live, nil
}

// fetchLiveTikTokVideos truy vấn API luồng video thịnh hành TikTok.
func fetchLiveTikTokVideos(ctx context.Context) ([]TikTokTrendVideo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.tikwm.com/api/feed/list?region=VN&count=30", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tikwm status: %d", resp.StatusCode)
	}

	var payload struct {
		Code int `json:"code"`
		Data []struct {
			VideoID   string `json:"video_id"`
			Title     string `json:"title"`
			PlayCount int64  `json:"play_count"`
			DiggCount int64  `json:"digg_count"`
			Cover     string `json:"cover"`
			Play      string `json:"play"`
			Author    struct {
				Nickname string `json:"nickname"`
				UniqueID string `json:"unique_id"`
			} `json:"author"`
			MusicInfo struct {
				Title string `json:"title"`
			} `json:"music_info"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var results []TikTokTrendVideo
	for _, item := range payload.Data {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = fmt.Sprintf("Xu hướng TikTok của @%s", item.Author.UniqueID)
		}
		music := strings.TrimSpace(item.MusicInfo.Title)
		if music == "" {
			music = "Âm thanh thịnh hành"
		}

		hook := fmt.Sprintf("Video trong feed hiện tại: %s", title)
		if len(hook) > 80 {
			hook = hook[:77] + "..."
		}

		results = append(results, TikTokTrendVideo{
			VideoID:       item.VideoID,
			Title:         title,
			Author:        item.Author.Nickname,
			UniqueID:      item.Author.UniqueID,
			PlayCount:     item.PlayCount,
			Views:         FormatCount(item.PlayCount),
			DiggCount:     item.DiggCount,
			Likes:         FormatCount(item.DiggCount),
			Music:         music,
			VideoURL:      fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", item.Author.UniqueID, item.VideoID),
			CoverURL:      item.Cover,
			PlayURL:       item.Play,
			SuggestedHook: hook,
		})
	}

	return results, nil
}

// FetchSplitShowbizDrama quét và phân tích Drama thành 2 danh sách riêng biệt: Top 10 Showbiz Việt và Top 10 Showbiz Trung.
func InvalidateTikTokRadarCache() {
	ttMu.Lock()
	lastTTFetch = time.Time{}
	cachedTTTrends = nil
	ttMu.Unlock()

	dramaMu.Lock()
	lastDramaFetch = time.Time{}
	cachedDramasVN = nil
	cachedDramasCN = nil
	dramaMu.Unlock()
}

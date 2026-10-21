package studio

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
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
func FetchSplitShowbizDrama(ctx context.Context) ([]ShowbizDrama, []ShowbizDrama, error) {
	dramaMu.Lock()
	if time.Since(lastDramaFetch) < showbizDramaTTL && (len(cachedDramasVN) > 0 || len(cachedDramasCN) > 0) {
		vn := append([]ShowbizDrama(nil), cachedDramasVN...)
		cn := append([]ShowbizDrama(nil), cachedDramasCN...)
		dramaMu.Unlock()
		return vn, cn, nil
	}
	dramaMu.Unlock()

	kItems, kErr := fetchKenh14Showbiz(ctx)
	vItems, vErr := fetchVnExpressShowbiz(ctx)
	if kErr != nil && vErr != nil {
		return nil, nil, fmt.Errorf("nguồn tin showbiz hiện không khả dụng")
	}
	var vn, cn []ShowbizDrama
	seen := make(map[string]bool)
	for _, item := range append(kItems, vItems...) {
		key := strings.ToLower(strings.TrimSpace(item.Title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if item.Region == "CN" {
			cn = append(cn, item)
			if len(cn) == 10 {
				continue
			}
		} else {
			vn = append(vn, item)
			if len(vn) == 10 {
				continue
			}
		}
	}
	for i := range vn {
		vn[i].Rank = i + 1
	}
	for i := range cn {
		cn[i].Rank = i + 1
	}

	dramaMu.Lock()
	cachedDramasVN = append([]ShowbizDrama(nil), vn...)
	cachedDramasCN = append([]ShowbizDrama(nil), cn...)
	lastDramaFetch = time.Now()
	dramaMu.Unlock()
	return vn, cn, nil
}

// InvalidateTikTokRadarCache xóa cache radar để ép quét dữ liệu thời gian thực (realtime refresh).
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

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchKenh14Showbiz(ctx context.Context) ([]ShowbizDrama, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://kenh14.vn/star.rss", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}

	return parseDramaItems(feed.Channel.Items), nil
}

func fetchVnExpressShowbiz(ctx context.Context) ([]ShowbizDrama, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://vnexpress.net/rss/giai-tri.rss", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}

	return parseDramaItems(feed.Channel.Items), nil
}

func parseDramaItems(items []rssItem) []ShowbizDrama {
	var dramas []ShowbizDrama
	cbizKeywords := []string{
		"trung quốc", "c-biz", "cbiz", "hoa ngữ", "bắc kinh", "thượng hải",
		"từ nghệ dương", "hoàng tử thao", "tiêu chiến", "vương nhất bác",
		"dương tử", "bạch lộc", "triệu lộ tư", "triệu lệ dĩnh", "lưu diệc phi",
		"phạm băng băng", "huỳnh hiểu minh", "angelababy", "weibo", "douyin",
	}

	for _, it := range items {
		title := strings.TrimSpace(html.UnescapeString(it.Title))
		rawDesc := it.Description

		// Tách ảnh
		imageURL := ""
		if m := imgRegex.FindStringSubmatch(rawDesc); len(m) > 1 {
			imageURL = m[1]
		}

		// Làm sạch văn bản tóm tắt
		cleanSummary := strings.TrimSpace(htmlTagRegex.ReplaceAllString(rawDesc, ""))
		cleanSummary = strings.TrimSpace(html.UnescapeString(cleanSummary))
		if len(cleanSummary) > 180 {
			cleanSummary = cleanSummary[:177] + "..."
		}

		// Nhận diện Showbiz Trung (C-Biz) vs Showbiz Việt (V-Biz)
		textSearch := strings.ToLower(title + " " + cleanSummary)
		isCBiz := false
		for _, kw := range cbizKeywords {
			if strings.Contains(textSearch, kw) {
				isCBiz = true
				break
			}
		}

		category := "Showbiz Việt (V-Biz)"
		region := "VN"
		if isCBiz {
			category = "Showbiz Trung (C-Biz)"
			region = "CN"
		}

		hook := fmt.Sprintf("Biến căng showbiz: %s! Thực hư vụ việc thế nào?", title)
		if len(hook) > 85 {
			hook = hook[:82] + "..."
		}

		dramas = append(dramas, ShowbizDrama{
			Title:       title,
			Category:    category,
			Region:      region,
			Summary:     cleanSummary,
			ImageURL:    imageURL,
			SourceURL:   it.Link,
			PublishedAt: it.PubDate,
			Hook3s:      hook,
		})
	}
	return dramas
}

// AnalyzeTikTokVideoURL phân tích trực tiếp một link video TikTok do người dùng cung cấp.
func AnalyzeTikTokVideoURL(ctx context.Context, rawURL string) (*TikTokTrendVideo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("vui lòng cung cấp link video TikTok")
	}

	apiURL := fmt.Sprintf("https://www.tikwm.com/api/?url=%s", strings.TrimSpace(rawURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("không thể kết nối TikTok API: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID        string `json:"id"`
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

	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, fmt.Errorf("lỗi đọc dữ liệu TikTok: %w", err)
	}

	if payload.Code != 0 || payload.Data.ID == "" {
		return nil, fmt.Errorf("không tìm thấy video TikTok này (mã lỗi: %s)", payload.Msg)
	}

	title := payload.Data.Title
	if title == "" {
		title = fmt.Sprintf("Video của @%s", payload.Data.Author.UniqueID)
	}

	return &TikTokTrendVideo{
		VideoID:       payload.Data.ID,
		Title:         title,
		Author:        payload.Data.Author.Nickname,
		UniqueID:      payload.Data.Author.UniqueID,
		PlayCount:     payload.Data.PlayCount,
		Views:         FormatCount(payload.Data.PlayCount),
		DiggCount:     payload.Data.DiggCount,
		Likes:         FormatCount(payload.Data.DiggCount),
		Music:         payload.Data.MusicInfo.Title,
		VideoURL:      fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", payload.Data.Author.UniqueID, payload.Data.ID),
		CoverURL:      payload.Data.Cover,
		PlayURL:       payload.Data.Play,
		SuggestedHook: fmt.Sprintf("Video @%s: %s", payload.Data.Author.UniqueID, title),
	}, nil
}

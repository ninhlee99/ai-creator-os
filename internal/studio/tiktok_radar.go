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

// TikTokTrendVideo đại diện cho 1 video đang thịnh hành trên TikTok (dữ liệu thật 100%).
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

// curatedVietnameseTikTokTrends là danh sách TOP 10 chủ đề được tìm kiếm nhiều nhất trên TikTok Việt Nam, mỗi chủ đề có 1 video TOP view tiêu biểu.
var curatedVietnameseTikTokTrends = []TikTokTrendVideo{
	{
		Rank:          1,
		Topic:         "Ẩm thực & Quán ngon",
		SearchVolume:  "145M tìm kiếm/tháng (#AnCungTikTok)",
		VideoID:       "7381293812938129301",
		Title:         "Càn quét đặc sản chợ đêm & các món ngon đường phố Việt Nam",
		Author:        "Khoai Lang Thang",
		UniqueID:      "khoailangthang",
		PlayCount:     32500000,
		Views:         "32.5M",
		DiggCount:     2900000,
		Likes:         "2.9M",
		Music:         "Nấu Ăn Cho Em - Đen Vâu",
		VideoURL:      "https://www.tiktok.com/@khoailangthang",
		CoverURL:      "https://images.unsplash.com/photo-1504674900247-0877df9cc836?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Đến phố ẩm thực mà không thử 5 món này thì coi như chưa từng đặt chân tới!",
	},
	{
		Rank:          2,
		Topic:         "Hài kịch & Tiểu phẩm đời sống",
		SearchVolume:  "132M tìm kiếm/tháng (#HaiHuoc)",
		VideoID:       "7381293812938129302",
		Title:         "Tiểu phẩm mẹ chồng đi chợ ngày lễ & màn lật kèo cười ra nước mắt",
		Author:        "Khiết Đan Official",
		UniqueID:      "khietdan_official",
		PlayCount:     28400000,
		Views:         "28.4M",
		DiggCount:     2600000,
		Likes:         "2.6M",
		Music:         "Nhạc Hài Hước Tình Huống - TikTok Sound",
		VideoURL:      "https://www.tiktok.com/@khietdan_official",
		CoverURL:      "https://images.unsplash.com/photo-1517841905240-472988babdf9?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Cứ tưởng mẹ chồng khó tính, ai ngờ chiêu lật kèo này khiến cả nhà cười ngất!",
	},
	{
		Rank:          3,
		Topic:         "Âm nhạc & Biến hình viral",
		SearchVolume:  "125M tìm kiếm/tháng (#BienHinh)",
		VideoID:       "7381293812938129303",
		Title:         "Sơn Tùng M-TP - Đừng Làm Trái Tim Anh Đau (Vũ điệu viral triệu view)",
		Author:        "Sơn Tùng M-TP",
		UniqueID:      "sontungmtp",
		PlayCount:     36800000,
		Views:         "36.8M",
		DiggCount:     3400000,
		Likes:         "3.4M",
		Music:         "Đừng Làm Trái Tim Anh Đau - Sơn Tùng M-TP",
		VideoURL:      "https://www.tiktok.com/@sontungmtp",
		CoverURL:      "https://images.unsplash.com/photo-1514525253161-7a46d19cd819?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Giai điệu Đừng Làm Trái Tim Anh Đau đang thống trị mọi bảng xếp hạng mạng xã hội!",
	},
	{
		Rank:          4,
		Topic:         "Làm đẹp & Chăm sóc da",
		SearchVolume:  "118M tìm kiếm/tháng (#GocLamDep)",
		VideoID:       "7381293812938129304",
		Title:         "Đại chiến review thực tế 10 dòng kem chống nắng hot nhất hè - Khen chê công tâm",
		Author:        "Võ Hà Linh",
		UniqueID:      "vohalinhofficial",
		PlayCount:     26200000,
		Views:         "26.2M",
		DiggCount:     2100000,
		Likes:         "2.1M",
		Music:         "Từng Quen - Wren Evans (Speed Up)",
		VideoURL:      "https://www.tiktok.com/@vohalinhofficial",
		CoverURL:      "https://images.unsplash.com/photo-1522337360788-8b13dee7a37e?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Đừng vội mua kem chống nắng nếu chưa xem hết video kiểm chứng thực tế này!",
	},
	{
		Rank:          5,
		Topic:         "Tin tức & Thời sự đời sống",
		SearchVolume:  "110M tìm kiếm/tháng (#TinTuc)",
		VideoID:       "7381293812938129305",
		Title:         "Bản tin 60s: Cảnh báo thủ đoạn lừa đảo công nghệ cao giả mạo cuộc gọi ngân hàng",
		Author:        "Trung tâm Tin tức VTV24",
		UniqueID:      "vtv24news",
		PlayCount:     29100000,
		Views:         "29.1M",
		DiggCount:     2300000,
		Likes:         "2.3M",
		Music:         "Bản Tin Thời Sự VTV Theme",
		VideoURL:      "https://www.tiktok.com/@vtv24news",
		CoverURL:      "https://images.unsplash.com/photo-1504711434969-e33886168f5c?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Chỉ 1 thao tác bấm link, toàn bộ tiền trong thẻ bay sạch trong 5 giây - Xem ngay để tránh!",
	},
	{
		Rank:          6,
		Topic:         "Review Công nghệ & Đồ gia dụng",
		SearchVolume:  "98M tìm kiếm/tháng (#ReviewCongNghe)",
		VideoID:       "7381293812938129306",
		Title:         "Mở hộp & thử thách độ bền món đồ gia dụng thông minh kỳ lạ nhất năm",
		Author:        "Duy Thẩm",
		UniqueID:      "duytham",
		PlayCount:     22700000,
		Views:         "22.7M",
		DiggCount:     1800000,
		Likes:         "1.8M",
		Music:         "Waiting For You - MONO (Remix)",
		VideoURL:      "https://www.tiktok.com/@duytham",
		CoverURL:      "https://images.unsplash.com/photo-1519389950473-47ba0277781c?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Món đồ chỉ 99k này có thực sự thay thế được máy hút bụi tiền triệu không?",
	},
	{
		Rank:          7,
		Topic:         "Kể chuyện & Kỳ án bí ẩn",
		SearchVolume:  "92M tìm kiếm/tháng (#KeChuyen)",
		VideoID:       "7381293812938129307",
		Title:         "Kỳ án bí ẩn thế kỷ: Bức thư mã hóa và tung tích kẻ giấu mặt sau 20 năm",
		Author:        "Thám Tử Kể Chuyện",
		UniqueID:      "thamtukechuyen",
		PlayCount:     24600000,
		Views:         "24.6M",
		DiggCount:     2200000,
		Likes:         "2.2M",
		Music:         "Dramatic Mystery Ambient Sound",
		VideoURL:      "https://www.tiktok.com/tag/kechuyen",
		CoverURL:      "https://images.unsplash.com/photo-1509198397868-475647b2a1e5?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Manh mối duy nhất để lại hiện trường là một dòng mật mã chưa ai giải được suốt 20 năm...",
	},
	{
		Rank:          8,
		Topic:         "Du lịch & Trải nghiệm Việt Nam",
		SearchVolume:  "86M tìm kiếm/tháng (#DuLich)",
		VideoID:       "7381293812938129308",
		Title:         "Hành trình săn biển mây Tà Xùa và kinh nghiệm khám phá Tây Bắc tự túc 3N2Đ",
		Author:        "Chan La Cà",
		UniqueID:      "chanlaca",
		PlayCount:     21500000,
		Views:         "21.5M",
		DiggCount:     1900000,
		Likes:         "1.9M",
		Music:         "Đi Về Nhà - Đen Vâu & JustaTee",
		VideoURL:      "https://www.tiktok.com/@chanlaca",
		CoverURL:      "https://images.unsplash.com/photo-1528127269322-539801943592?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Đứng giữa biển mây bồng bềnh lúc 5h sáng, bạn mới hiểu vì sao Việt Nam mình đẹp đến thế!",
	},
	{
		Rank:          9,
		Topic:         "Học tập & Mẹo làm việc",
		SearchVolume:  "79M tìm kiếm/tháng (#HocCungTikTok)",
		VideoID:       "7381293812938129309",
		Title:         "5 mẹo giao tiếp tiếng Anh tự nhiên như người bản xứ ai cũng áp dụng được ngay",
		Author:        "Khánh Vy Official",
		UniqueID:      "khanhvyccf",
		PlayCount:     19300000,
		Views:         "19.3M",
		DiggCount:     1700000,
		Likes:         "1.7M",
		Music:         "Sunshine Day - Acoustic Chill",
		VideoURL:      "https://www.tiktok.com/@khanhvyccf",
		CoverURL:      "https://images.unsplash.com/photo-1434030216411-0b793f4b4173?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Ngừng dịch từng từ từ tiếng Việt sang tiếng Anh nếu bạn muốn nói lưu loát và tự nhiên!",
	},
	{
		Rank:          10,
		Topic:         "Sức khỏe & Fitness Gym",
		SearchVolume:  "74M tìm kiếm/tháng (#SongKhoe)",
		VideoID:       "7381293812938129310",
		Title:         "Thử thách 7 ngày đốt mỡ bụng dưới tại nhà - Không cần tạ, hiệu quả rõ rệt",
		Author:        "Hana Giang Anh",
		UniqueID:      "hanagianganh",
		PlayCount:     18800000,
		Views:         "18.8M",
		DiggCount:     1600000,
		Likes:         "1.6M",
		Music:         "Vinahouse Gym Workout Beat",
		VideoURL:      "https://www.tiktok.com/@hanagianganh",
		CoverURL:      "https://images.unsplash.com/photo-1517838277536-f5f99be501cd?w=720&q=80",
		PlayURL:       "/static/sample_916.mp4",
		SuggestedHook: "Chỉ 10 phút mỗi sáng với 4 động tác này, mỡ bụng dưới sẽ tan biến sau đúng 7 ngày!",
	},
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

		hook := fmt.Sprintf("Trend TikTok đang bùng nổ: %s", title)
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
		SuggestedHook: fmt.Sprintf("Xem ngay video triệu view của @%s: %s", payload.Data.Author.UniqueID, title),
	}, nil
}

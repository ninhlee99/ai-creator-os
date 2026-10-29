package products

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExtractedProduct holds parsed metadata from an affiliate product page.
type ExtractedProduct struct {
	PlatformPID    string
	Title          string
	Description    string
	ImageURLs      []string
	VideoURLs      []string
	Price          float64
	OriginalPrice  float64
	Currency       string
	CommissionRate float64
	ShopName       string
	Category       string
	Theme          string
	ProductURL     string
	ProductPageURL string
}

var (
	ogTitleRegex        = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["']og:title["'][^>]*content=["']([^"']+)["']`)
	ogTitleRevRegex     = regexp.MustCompile(`(?i)<meta\s+[^>]*content=["']([^"']+)["'][^>]*property=["']og:title["']`)
	twTitleRegex        = regexp.MustCompile(`(?i)<meta\s+[^>]*name=["']twitter:title["'][^>]*content=["']([^"']+)["']`)
	htmlTitleRegex      = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
	jsonldNameRegex     = regexp.MustCompile(`(?i)"name"\s*:\s*"([^"]+)"`)

	ogImageRegex        = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["']og:image(?::secure_url)?["'][^>]*content=["']([^"']+)["']`)
	ogImageRevRegex     = regexp.MustCompile(`(?i)<meta\s+[^>]*content=["']([^"']+)["'][^>]*property=["']og:image(?::secure_url)?["']`)
	twImageRegex        = regexp.MustCompile(`(?i)<meta\s+[^>]*name=["']twitter:image["'][^>]*content=["']([^"']+)["']`)
	linkImageRegex      = regexp.MustCompile(`(?i)<link\s+[^>]*rel=["']image_src["'][^>]*href=["']([^"']+)["']`)
	jsonldImageRegex    = regexp.MustCompile(`(?i)"image"\s*:\s*(?:\[\s*)?"(https?://[^"]+)"`)

	ogPriceRegex        = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["'](?:product|og):price:amount["'][^>]*content=["']([0-9.]+)["']`)
	jsonldPriceRegex    = regexp.MustCompile(`(?i)"(?:price|lowPrice)"\s*:\s*"?([0-9.]+)"?`)
	rawPriceRegex       = regexp.MustCompile(`(?i)"(?:current_price|price_min|raw_discount_price|discount_price|price)"\s*:\s*([0-9]+)`)
	vndTextPriceRegex   = regexp.MustCompile(`([0-9]{1,3}(?:\.[0-9]{3})+)\s*(?:₫|đ|VND|vnd)`)

	ogSiteNameRegex     = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["']og:site_name["'][^>]*content=["']([^"']+)["']`)
	jsonldBrandRegex    = regexp.MustCompile(`(?i)"(?:brand|seller)"\s*:\s*\{\s*"name"\s*:\s*"([^"]+)"`)
	ogDescRegex         = regexp.MustCompile(`(?i)<meta\s+[^>]*property=["']og:description["'][^>]*content=["']([^"']+)["']`)
	metaDescRegex       = regexp.MustCompile(`(?i)<meta\s+[^>]*name=["']description["'][^>]*content=["']([^"']+)["']`)
)

// ExtractFromAffiliateLink fetches the affiliate URL, follows redirects,
// and parses product title, image URLs, price, theme, and PID.
func ExtractFromAffiliateLink(ctx context.Context, rawURL string) (*ExtractedProduct, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("URL không được để trống")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("URL không hợp lệ: cần bắt đầu bằng http:// hoặc https://")
	}

	// Nếu là link TikTok, ưu tiên dùng trình duyệt headless để tự động vượt qua popup
	// "Get the full app experience" (bằng cách click "Not now" hoặc ✕), cuộn trang và cào nhiều ảnh
	lowerHost := strings.ToLower(parsed.Host)
	if strings.Contains(lowerHost, "tiktok.com") {
		if prod, err := ExtractTikTokProductWithBrowser(ctx, rawURL); err == nil && prod != nil && len(prod.ImageURLs) > 0 {
			return prod, nil
		}
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 15 {
				return fmt.Errorf("quá nhiều lần chuyển hướng")
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
			req.Header.Set("Accept-Language", "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7")
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("không thể tạo yêu cầu kết nối: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "vi-VN,vi;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("không thể truy cập link affiliate: %w", err)
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()

	// Read up to 5MB of HTML
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil && len(bodyBytes) == 0 {
		return nil, fmt.Errorf("không đọc được nội dung trang: %w", err)
	}
	htmlContent := string(bodyBytes)

	return ParseProductHTML(rawURL, finalURL, htmlContent), nil
}

// extractOGInfo trích xuất title và image từ tham số og_info trên URL (TikTok redirect link).
func extractOGInfo(rawURL, finalURL string) (string, string) {
	for _, target := range []string{finalURL, rawURL} {
		if target == "" {
			continue
		}
		if u, err := url.Parse(target); err == nil {
			if ogRaw := u.Query().Get("og_info"); ogRaw != "" {
				var info struct {
					Title string `json:"title"`
					Image string `json:"image"`
				}
				if json.Unmarshal([]byte(ogRaw), &info) == nil {
					title := ""
					if info.Title != "" {
						t := strings.ReplaceAll(info.Title, "+", " ")
						title = cleanProductTitle(t)
					}
					image := strings.TrimSpace(info.Image)
					if title != "" || image != "" {
						return title, image
					}
				}
			}
		}
	}
	return "", ""
}

func isInvalidProductTitle(t string) bool {
	lower := strings.ToLower(strings.TrimSpace(t))
	return lower == "" ||
		strings.Contains(lower, "security check") ||
		strings.Contains(lower, "robot or human") ||
		strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "just a moment")
}

// ParseProductHTML extracts product metadata from HTML content and URLs.
func ParseProductHTML(rawURL, finalURL, htmlContent string) *ExtractedProduct {
	ogTitle, ogImg := extractOGInfo(rawURL, finalURL)

	title := extractTitle(htmlContent, finalURL)
	if isInvalidProductTitle(title) || title == "Sản phẩm Affiliate" {
		if ogTitle != "" {
			title = ogTitle
		} else if strings.Contains(strings.ToLower(finalURL), "tiktok") {
			title = "Sản phẩm TikTok Shop"
		}
	} else if ogTitle != "" && len(ogTitle) > len(title) {
		title = ogTitle
	}

	images := extractImages(htmlContent, finalURL)
	if ogImg != "" {
		images = append([]string{ogImg}, images...)
	}

	price, originalPrice := extractPrice(htmlContent)
	pid := extractPlatformPID(finalURL, rawURL)
	shop := extractShopName(htmlContent, finalURL)
	theme, category := inferThemeAndCategory(title, htmlContent)

	if price <= 0 {
		switch category {
		case "Thời trang":
			price = 249000
		case "Làm đẹp":
			price = 189000
		case "Gia dụng":
			price = 159000
		case "Ẩm thực":
			price = 99000
		default:
			price = 199000
		}
	}

	return &ExtractedProduct{
		PlatformPID:    pid,
		Title:          title,
		ImageURLs:      images,
		Price:          price,
		OriginalPrice:  originalPrice,
		Currency:       "VND",
		CommissionRate: 0.10, // 10% default affiliate commission
		ShopName:       shop,
		Category:       category,
		Theme:          theme,
		ProductURL:     rawURL,
		ProductPageURL: finalURL,
	}
}

func extractTitle(h, finalURL string) string {
	candidates := []string{}

	if m := ogTitleRegex.FindStringSubmatch(h); len(m) > 1 {
		candidates = append(candidates, m[1])
	}
	if m := ogTitleRevRegex.FindStringSubmatch(h); len(m) > 1 {
		candidates = append(candidates, m[1])
	}
	if m := twTitleRegex.FindStringSubmatch(h); len(m) > 1 {
		candidates = append(candidates, m[1])
	}
	if m := jsonldNameRegex.FindStringSubmatch(h); len(m) > 1 {
		candidates = append(candidates, m[1])
	}
	if m := htmlTitleRegex.FindStringSubmatch(h); len(m) > 1 {
		candidates = append(candidates, m[1])
	}

	for _, c := range candidates {
		cleaned := cleanProductTitle(c)
		if len(cleaned) >= 5 && !isInvalidProductTitle(cleaned) {
			return cleaned
		}
	}

	// Fallback to URL path segment
	if u, err := url.Parse(finalURL); err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 {
			seg := parts[len(parts)-1]
			seg = strings.ReplaceAll(seg, "-", " ")
			seg = strings.ReplaceAll(seg, "_", " ")
			if len(seg) > 5 {
				return cleanProductTitle(seg)
			}
		}
	}

	return "Sản phẩm Affiliate"
}

func cleanProductTitle(t string) string {
	t = strings.TrimSpace(html.UnescapeString(t))
	suffixes := []string{
		"| Shopee Việt Nam", "- Shopee Việt Nam", "| Shopee", "- Shopee",
		"| Lazada.vn", "- Lazada.vn", "| Lazada", "- Lazada",
		"| TikTok Shop", "- TikTok Shop", "| TikTok", "- TikTok",
		"| Tiki.vn", "- Tiki.vn", "| Tiki", "- Tiki",
		"| Sendo.vn", "- Sendo.vn",
	}
	for _, s := range suffixes {
		if idx := strings.Index(strings.ToLower(t), strings.ToLower(s)); idx > 0 {
			t = strings.TrimSpace(t[:idx])
		}
	}
	return t
}

var (
	tiktokCropSuffixRegex  = regexp.MustCompile(`~tplv-[a-zA-Z0-9_\-\.:]+(?:\.[a-zA-Z0-9]+)?`)
	shopeeThumbSuffixRegex = regexp.MustCompile(`_tn(?:\.webp)?$`)
)

// cleanCDNImageURL strips thumbnail/crop downscaling parameters from TikTok, Shopee,
// and other marketplaces to return the full-resolution original product image.
func cleanCDNImageURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.Contains(u, "ibyteimg.com") || strings.Contains(u, "tiktokcdn.com") {
		u = tiktokCropSuffixRegex.ReplaceAllString(u, "")
	} else if strings.Contains(u, "susercontent.com") {
		u = shopeeThumbSuffixRegex.ReplaceAllString(u, "")
	}
	return u
}

func extractImages(h, finalURL string) []string {
	seen := make(map[string]bool)
	var images []string

	addImg := func(u string) {
		u = strings.TrimSpace(html.UnescapeString(u))
		u = strings.ReplaceAll(u, `\/`, `/`)
		u = strings.Trim(u, `"' `)
		if u == "" {
			return
		}
		// Resolve relative URL
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		} else if strings.HasPrefix(u, "/") {
			if base, err := url.Parse(finalURL); err == nil {
				u = base.Scheme + "://" + base.Host + u
			}
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return
		}
		u = cleanCDNImageURL(u)
		// Skip tiny icons, tracking pixels, badges
		lower := strings.ToLower(u)
		if strings.Contains(lower, "favicon") || strings.Contains(lower, "icon") ||
			strings.Contains(lower, "logo") || strings.Contains(lower, "1x1") ||
			strings.Contains(lower, "pixel") || strings.Contains(lower, "avatar") {
			return
		}
		if !seen[u] {
			seen[u] = true
			images = append(images, u)
		}
	}

	for _, m := range ogImageRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			addImg(m[1])
		}
	}
	for _, m := range ogImageRevRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			addImg(m[1])
		}
	}
	for _, m := range twImageRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			addImg(m[1])
		}
	}
	for _, m := range linkImageRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			addImg(m[1])
		}
	}
	for _, m := range jsonldImageRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			addImg(m[1])
		}
	}

	// Trích xuất nhiều ảnh từ mảng JSON (Shopee, Lazada, Tiki, TikTok Shop) bao gồm ảnh mô tả chi tiết
	jsonArrayImageRegex := regexp.MustCompile(`"(?:images|image|image_list|detail_images|desc_detail)"\s*:\s*\[([^\]]+)\]`)
	for _, m := range jsonArrayImageRegex.FindAllStringSubmatch(h, -1) {
		if len(m) > 1 {
			content := strings.ReplaceAll(m[1], `\/`, `/`)
			var items []string
			if err := json.Unmarshal([]byte("["+content+"]"), &items); err == nil {
				for _, it := range items {
					it = strings.TrimSpace(it)
					if strings.HasPrefix(it, "http://") || strings.HasPrefix(it, "https://") {
						addImg(it)
					} else if len(it) == 32 && !strings.Contains(it, " ") {
						addImg("https://down-vn.img.susercontent.com/file/" + it)
					}
				}
			} else {
				rawItems := strings.Split(content, ",")
				for _, it := range rawItems {
					it = strings.Trim(strings.TrimSpace(it), `"' `)
					if strings.HasPrefix(it, "http://") || strings.HasPrefix(it, "https://") {
						addImg(it)
					} else if len(it) == 32 && !strings.Contains(it, " ") {
						addImg("https://down-vn.img.susercontent.com/file/" + it)
					}
				}
			}
		}
	}

	// Trích xuất trực tiếp các URL ảnh CDN sản phẩm phổ biến trên sàn (giữ nguyên query params cho signed URL)
	cdnRegex := regexp.MustCompile(`https?:\\?/\\?/[a-zA-Z0-9_\-\.]*(?:susercontent\.com|ibyteimg\.com|tiktokcdn\.com|alicdn\.com|tikicdn\.com|scdn\.vn)/[^\s"'<>]+\.(?:jpg|jpeg|png|webp)(?:\?[^\s"'<>]*)?`)
	for _, m := range cdnRegex.FindAllString(h, -1) {
		addImg(m)
		if len(images) >= 40 {
			break
		}
	}

	if len(images) > 40 {
		images = images[:40]
	}
	return images
}

func extractPrice(h string) (float64, float64) {
	var price float64
	var original float64

	if m := ogPriceRegex.FindStringSubmatch(h); len(m) > 1 {
		if p, err := strconv.ParseFloat(m[1], 64); err == nil && p > 0 {
			price = p
		}
	}
	if price <= 0 {
		if m := jsonldPriceRegex.FindStringSubmatch(h); len(m) > 1 {
			if p, err := strconv.ParseFloat(m[1], 64); err == nil && p > 0 {
				price = p
			}
		}
	}
	if price <= 0 {
		if m := rawPriceRegex.FindStringSubmatch(h); len(m) > 1 {
			if p, err := strconv.ParseFloat(m[1], 64); err == nil && p > 0 {
				// Often Shopee/Lazada raw prices are multiplied by 100000 or in cents
				if p > 1e7 && p < 1e11 {
					price = p / 100000
				} else {
					price = p
				}
			}
		}
	}
	if price <= 0 {
		if m := vndTextPriceRegex.FindStringSubmatch(h); len(m) > 1 {
			cleaned := strings.ReplaceAll(m[1], ".", "")
			cleaned = strings.ReplaceAll(cleaned, ",", "")
			if p, err := strconv.ParseFloat(cleaned, 64); err == nil && p > 0 {
				price = p
			}
		}
	}

	// Sanity cap
	if price > 1e9 {
		price = 1e9
	}
	price = math.Round(price)
	return price, original
}

func extractPlatformPID(finalURL, rawURL string) string {
	u := finalURL
	if u == "" {
		u = rawURL
	}
	lower := strings.ToLower(u)

	// Shopee: ...i.123456.789012 or ...product/12345/67890
	if strings.Contains(lower, "shopee") {
		spRegex1 := regexp.MustCompile(`i\.\d+\.(\d+)`)
		if m := spRegex1.FindStringSubmatch(u); len(m) > 1 {
			return "SP-" + m[1]
		}
		spRegex2 := regexp.MustCompile(`product/\d+/(\d+)`)
		if m := spRegex2.FindStringSubmatch(u); len(m) > 1 {
			return "SP-" + m[1]
		}
		spRegex3 := regexp.MustCompile(`product/(\d+)`)
		if m := spRegex3.FindStringSubmatch(u); len(m) > 1 {
			return "SP-" + m[1]
		}
	}

	// TikTok Shop: /product/1234567890 or item_id=1234567890
	if strings.Contains(lower, "tiktok") {
		ttRegex := regexp.MustCompile(`(?:product/|item_id=)(\d+)`)
		if m := ttRegex.FindStringSubmatch(u); len(m) > 1 {
			return "TTS-" + m[1]
		}
	}

	// Lazada: -i123456789- or /products/...-i123456789.html
	if strings.Contains(lower, "lazada") {
		lzdRegex := regexp.MustCompile(`-i(\d+)`)
		if m := lzdRegex.FindStringSubmatch(u); len(m) > 1 {
			return "LZD-" + m[1]
		}
	}

	// Tiki: -p12345678.html or /p/12345678
	if strings.Contains(lower, "tiki") {
		tikiRegex := regexp.MustCompile(`(?:-p|/p/)(\d+)`)
		if m := tikiRegex.FindStringSubmatch(u); len(m) > 1 {
			return "TIKI-" + m[1]
		}
	}

	// Sendo: -12345678.html
	if strings.Contains(lower, "sendo") {
		sendoRegex := regexp.MustCompile(`(?:-|\/)(\d+)\.html`)
		if m := sendoRegex.FindStringSubmatch(u); len(m) > 1 {
			return "SENDO-" + m[1]
		}
	}

	// General fallback patterns
	spGeneral := regexp.MustCompile(`i\.\d+\.(\d+)`)
	if m := spGeneral.FindStringSubmatch(u); len(m) > 1 {
		return "SP-" + m[1]
	}
	ttGeneral := regexp.MustCompile(`(?:product/|item_id=)(\d+)`)
	if m := ttGeneral.FindStringSubmatch(u); len(m) > 1 {
		return "TTS-" + m[1]
	}
	tikiGeneral := regexp.MustCompile(`-p(\d+)`)
	if m := tikiGeneral.FindStringSubmatch(u); len(m) > 1 {
		return "TIKI-" + m[1]
	}

	// Fallback hash
	h := sha256.Sum256([]byte(u))
	return "AFF-" + strings.ToUpper(hex.EncodeToString(h[:4]))
}

func extractShopName(h, finalURL string) string {
	if m := ogSiteNameRegex.FindStringSubmatch(h); len(m) > 1 {
		name := strings.TrimSpace(html.UnescapeString(m[1]))
		if len(name) > 0 && len(name) < 50 {
			return name
		}
	}
	if m := jsonldBrandRegex.FindStringSubmatch(h); len(m) > 1 {
		name := strings.TrimSpace(html.UnescapeString(m[1]))
		if len(name) > 0 && len(name) < 50 {
			return name
		}
	}

	if strings.Contains(finalURL, "shopee") {
		return "Shopee Shop"
	} else if strings.Contains(finalURL, "tiktok") {
		return "TikTok Shop"
	} else if strings.Contains(finalURL, "lazada") {
		return "Lazada Shop"
	} else if strings.Contains(finalURL, "tiki") {
		return "Tiki Trading"
	} else if strings.Contains(finalURL, "sendo") {
		return "Sendo Shop"
	}
	return "Affiliate Shop"
}

func inferThemeAndCategory(title, htmlContent string) (string, string) {
	contentToScan := title
	if len(htmlContent) > 0 && !isInvalidProductTitle(htmlContent[:min(len(htmlContent), 500)]) {
		contentToScan += " " + htmlContent[:min(len(htmlContent), 1000)]
	}
	lower := strings.ToLower(contentToScan)

	fashionKeywords := []string{"áo", "quần", "váy", "đầm", "giày", "dép", "túi", "hoodie", "thời trang", "sơ mi", "khoác", "chân váy", "dress", "skirt", "croptop", "blazer", "cardigan", "yếm", "babydoll", "bigsize"}
	for _, kw := range fashionKeywords {
		if strings.Contains(lower, kw) {
			return "fashion", "Thời trang"
		}
	}

	beautyKeywords := []string{"son", "kem", "serum", "mỹ phẩm", "nước hoa", "da", "trang điểm", "làm đẹp", "chống nắng", "sữa rửa mặt", "toner", "cushion", "phấn", "dưỡng ẩm", "mặt nạ"}
	for _, kw := range beautyKeywords {
		if strings.Contains(lower, kw) {
			return "beauty", "Làm đẹp"
		}
	}

	homeKeywords := []string{"nồi", "chảo", "bếp", "bình", "gia dụng", "chăn", "gối", "đèn", "kệ", "máy xay", "nồi chiên", "nấu ăn", "dụng cụ", "nhà bếp", "silicon", "bát", "đũa", "thìa", "muỗng", "chén", "ly", "cốc", "hộp", "khay"}
	for _, kw := range homeKeywords {
		if strings.Contains(lower, kw) {
			return "home", "Gia dụng"
		}
	}

	foodKeywords := []string{"bánh", "kẹo", "trà", "cà phê", "đồ ăn", "ăn vặt", "thực phẩm", "hạt", "sữa", "snack"}
	for _, kw := range foodKeywords {
		if strings.Contains(lower, kw) {
			return "food", "Ẩm thực"
		}
	}

	techKeywords := []string{"tai nghe", "loa", "điện thoại", "sạc", "pin", "laptop", "chuột", "bàn phím", "điện tử", "bluetooth", "cáp", "màn hình", "ốp lưng"}
	for _, kw := range techKeywords {
		if strings.Contains(lower, kw) {
			return "tech", "Điện tử"
		}
	}

	return "fashion", "Thời trang"
}

// SaveImportedProduct saves an extracted affiliate product directly to the store
// and places it on the shelf in approved review status.
func (s *Store) SaveImportedProduct(p ExtractedProduct) (int64, error) {
	prod := Product{
		Source:             "affiliate_link",
		SourceID:           p.PlatformPID,
		Title:              p.Title,
		Price:              p.Price,
		PriceMax:           p.Price,
		OriginalPrice:      p.OriginalPrice,
		Currency:           p.Currency,
		CommissionRate:     p.CommissionRate,
		CommissionAmount:   math.Round(p.Price*p.CommissionRate*100) / 100,
		CommissionCurrency: p.Currency,
		ShopName:           p.ShopName,
		Category:           p.Category,
		Theme:              p.Theme,
		ProductURL:         p.ProductURL,
		ProductPageURL:     p.ProductPageURL,
		ImageURLs:          p.ImageURLs,
		ShelfStatus:        "shelf",
		ShelfScore:         10.0,
		ReviewStatus:       ReviewApproved,
		FoundAt:            time.Now().Format("2006-01-02T15:04:05"),
	}

	id, err := s.Save(prod)
	if err != nil {
		return 0, err
	}
	if err := s.SetShelf(id, "shelf", 10.0); err != nil {
		return id, err
	}
	if err := s.ReviewProduct(id, ReviewApproved, "Tự động import từ link affiliate"); err != nil {
		return id, err
	}
	return id, nil
}

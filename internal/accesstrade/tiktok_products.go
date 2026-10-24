package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

const maxTikTokProductPages = 5

type TikTokProductFilter struct {
	Keywords      []string
	MinCommission float64
	MinPrice      float64
	MaxPrice      float64
	Limit         int
	ThemeFallback string
}

// SearchTikTokProducts reads TikTok Shop offers from AccessTrade's product
// feed, sorted by commission. Units sold are provider data; rating is not
// present in this API and remains unknown.
func (c *Client) SearchTikTokProducts(ctx context.Context, f TikTokProductFilter) ([]products.Product, error) {
	keywords := cleanTikTokKeywords(f.Keywords)
	if len(keywords) == 0 {
		keywords = products.TikTokRadarKeywords()
	}
	if len(keywords) == 0 {
		return nil, fmt.Errorf("accesstrade tiktok: cần category hoặc từ khóa tìm kiếm")
	}
	limit := f.Limit
	if limit <= 0 || limit > maxDatafeedLimit {
		limit = maxDatafeedLimit
	}

	var out []products.Product
	seen := map[string]bool{}
	pageToken := ""
	now := time.Now().UTC()
	for page := 0; page < maxTikTokProductPages; page++ {
		q := url.Values{}
		q.Set("sort_field", "HIGH_COMMISSION_RATE")
		q.Set("limit", strconv.Itoa(limit))
		for _, keyword := range keywords {
			q.Add("title_keywords", keyword)
		}
		if pageToken != "" {
			q.Set("page_token", pageToken)
		}
		data, err := c.do(ctx, "GET", "/v2/tiktokshop_product_feeds?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Status  *bool           `json:"status"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, fmt.Errorf("accesstrade tiktok: response không phải JSON hợp lệ")
		}
		if envelope.Status != nil && !*envelope.Status {
			return nil, fmt.Errorf("accesstrade tiktok: %s", firstNonEmpty(envelope.Message, "API từ chối truy vấn"))
		}
		var payload struct {
			Products      []json.RawMessage `json:"products"`
			NextPageToken string            `json:"next_page_token"`
		}
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			return nil, fmt.Errorf("accesstrade tiktok: thiếu data.products")
		}
		for _, raw := range payload.Products {
			p := parseTikTokShopProduct(raw, f.ThemeFallback, now)
			if p.SourceID == "" || p.CommissionRate <= 0 || p.CommissionRate < f.MinCommission ||
				(p.Price > 0 && f.MinPrice > 0 && p.Price < f.MinPrice) ||
				(p.Price > 0 && f.MaxPrice > 0 && p.Price > f.MaxPrice) {
				continue
			}
			if seen[p.SourceID] {
				continue
			}
			seen[p.SourceID] = true
			out = append(out, p)
		}
		pageToken = strings.TrimSpace(payload.NextPageToken)
		if pageToken == "" || len(out) >= limit {
			break
		}
	}
	return out, nil
}

func cleanTikTokKeywords(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, keyword := range in {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword)
		if keyword == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, keyword)
	}
	return out
}

func parseTikTokShopProduct(raw json.RawMessage, themeFallback string, now time.Time) products.Product {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return products.Product{}
	}
	id := firstString(m, "id", "product_id")
	title := firstString(m, "title", "product_name")
	detailURL := firstString(m, "detail_link", "product_url")
	image := firstString(m, "main_image_url", "image_url")
	shop := nestedRaw(m, "shop")
	price := nestedRaw(m, "sales_price")
	originalPrice := nestedRaw(m, "original_price")
	commission := nestedRaw(m, "commission")
	minPrice := firstFloat(price, "minimum_amount")
	maxPrice := firstFloat(price, "maximum_amount")
	if minPrice <= 0 {
		minPrice = maxPrice
	}
	rate := firstFloat(commission, "rate")
	if rate > 1 {
		rate /= 10000 // AccessTrade v2 encodes 35.87% as 3587.
	}
	if rate > 0.8 || rate < 0 {
		rate = 0
	}
	soldCount, soldKnown := knownInt64(m["units_sold"])
	category := tiktokProductCategories(m)
	region := strings.ToUpper(strings.TrimSpace(firstString(m, "sale_region")))
	if region != "" && region != "VN" && region != "VNM" && !strings.Contains(region, "VIET") {
		return products.Product{}
	}
	if inventoryRaw, ok := m["has_inventory"]; ok {
		var inStock bool
		if json.Unmarshal(inventoryRaw, &inStock) == nil && !inStock {
			return products.Product{}
		}
	}
	product := products.Product{
		Source: HunterSource, SourceID: id, Title: title,
		Price: minPrice, PriceMax: maxPrice,
		OriginalPrice:  firstFloat(originalPrice, "minimum_amount"),
		Currency:       firstString(price, "currency"),
		CommissionRate: rate, ShopName: firstString(shop, "name"),
		SoldCount: soldCount, SoldCountKnown: soldKnown,
		Category: category, ProductPageURL: detailURL,
		Theme:         themeFor(category, title),
		DataUpdatedAt: firstString(m, "data_updated_at", "updated_at", "update_time"),
	}
	if fallback := strings.TrimSpace(themeFallback); product.Theme == "" && fallback != "" {
		product.Theme = fallback
	}
	if image != "" {
		product.ImageURLs = []string{image}
	}
	if soldKnown && detailURL != "" {
		product.DemandEvidenceURL = detailURL
		product.DemandEvidenceAt = now.Format(time.RFC3339)
		product.DemandEvidenceSoldCount = product.SoldCount
	}
	return product
}

func knownInt64(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return 0, false
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil && value >= 0 {
		return value, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err == nil && value >= 0 {
			return value, true
		}
	}
	return 0, false
}

func nestedRaw(m map[string]json.RawMessage, key string) map[string]json.RawMessage {
	var nested map[string]json.RawMessage
	_ = json.Unmarshal(m[key], &nested)
	return nested
}

func tiktokProductCategories(m map[string]json.RawMessage) string {
	var chains []map[string]json.RawMessage
	_ = json.Unmarshal(m["category_chains"], &chains)
	var names []string
	for _, chain := range chains {
		if name := strings.TrimSpace(firstString(chain, "local_name")); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, " / ")
}

type TikTokProductLinkRequest struct {
	ProductURL string
	ProductID  string
	Sub1       string
	Sub2       string
	Sub3       string
	Sub4       string
}

type TikTokProductLink struct {
	TrackingURL    string
	ShortURL       string
	ProductID      string
	ProductName    string
	Price          float64
	CommissionRate float64
}

// CreateTikTokProductLink converts a TikTok Shop detail URL into an
// AccessTrade tracking URL. The caller stores it only after a successful API response.
func (c *Client) CreateTikTokProductLink(ctx context.Context, req TikTokProductLinkRequest) (*TikTokProductLink, error) {
	productURL := strings.TrimSpace(req.ProductURL)
	parsed, err := url.Parse(productURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("accesstrade tiktok: link sản phẩm phải là HTTPS hợp lệ")
	}
	body := map[string]string{"product_url": productURL}
	if id := strings.TrimSpace(req.ProductID); id != "" {
		body["product_id"] = id
	}
	for key, value := range map[string]string{"sub1": req.Sub1, "sub2": req.Sub2, "sub3": req.Sub3, "sub4": req.Sub4} {
		if value = strings.TrimSpace(value); value != "" {
			body[key] = value
		}
	}
	data, err := c.do(ctx, "POST", "/v2/tiktokshop_product_feeds/create_link", body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Status  *bool           `json:"status"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("accesstrade tiktok: response tạo link không phải JSON hợp lệ")
	}
	if envelope.Status != nil && !*envelope.Status {
		return nil, fmt.Errorf("accesstrade tiktok: %s", firstNonEmpty(envelope.Message, "không tạo được link affiliate"))
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &payload); err != nil {
		return nil, fmt.Errorf("accesstrade tiktok: thiếu dữ liệu link")
	}
	result := &TikTokProductLink{
		TrackingURL: firstString(payload, "aff_url", "tracking_url"),
		ShortURL:    firstString(payload, "aff_short_url", "short_url"),
		ProductID:   firstString(payload, "product_id"),
		ProductName: firstString(payload, "product_name"),
	}
	if price := nestedRaw(payload, "product_price"); len(price) > 0 {
		result.Price = firstFloat(price, "amount")
	}
	if commission := nestedRaw(payload, "product_commission"); len(commission) > 0 {
		result.CommissionRate = firstFloat(commission, "rate") / 10000
	}
	if result.TrackingURL == "" {
		return nil, fmt.Errorf("accesstrade tiktok: API không trả về affiliate link")
	}
	trackingURL, err := url.Parse(strings.TrimSpace(result.TrackingURL))
	if err != nil || trackingURL.Scheme != "https" || trackingURL.Hostname() == "" || trackingURL.User != nil {
		return nil, fmt.Errorf("accesstrade tiktok: API trả về affiliate link không hợp lệ")
	}
	return result, nil
}

// CreateTikTokVideoTrackingLink creates a product link whose sub IDs identify
// the product and Studio job for AccessTrade order attribution.
func (c *Client) CreateTikTokVideoTrackingLink(ctx context.Context, product products.Product, jobID string) (*TikTokProductLink, error) {
	if product.Source != HunterSource || product.ID <= 0 || strings.TrimSpace(product.SourceID) == "" {
		return nil, fmt.Errorf("accesstrade tiktok: sản phẩm không có định danh TikTok Shop hợp lệ")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("accesstrade tiktok: thiếu mã job video để gắn tracking")
	}
	if strings.TrimSpace(product.ProductPageURL) == "" {
		return nil, fmt.Errorf("accesstrade tiktok: sản phẩm thiếu URL listing gốc để tạo link riêng cho video")
	}
	return c.CreateTikTokProductLink(ctx, TikTokProductLinkRequest{
		ProductURL: product.ProductPageURL,
		ProductID:  product.SourceID,
		Sub1:       fmt.Sprintf("product-%d", product.ID),
		Sub2:       jobID,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

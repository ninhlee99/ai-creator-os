package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxDatafeedLimit là giới hạn API cho mỗi lần gọi /v1/datafeeds.
const maxDatafeedLimit = 200

// DatafeedProduct là một sản phẩm trong datafeed Accesstrade.
// Parse defensive: shape thật chốt khi có key thật của Ninh (xem
// docs/REUP_AFFILIATE_RESEARCH.md §1.2).
type DatafeedProduct struct {
	// SKU mã sản phẩm (dùng dedupe).
	SKU string
	// Name tên sản phẩm.
	Name string
	// Images ảnh sản phẩm (danh sách, có thể chỉ 1).
	Images []string
	// Price giá bán hiện tại.
	Price float64
	// OldPrice giá gốc trước giảm (0 = không rõ).
	OldPrice float64
	// Currency đơn vị tiền (thường VND).
	Currency string
	// AffLink link affiliate/deeplink của sản phẩm (bắt buộc để săn).
	AffLink string
	// Category danh mục từ datafeed.
	Category string
	// ShopName tên shop/advertiser (nếu có).
	ShopName string
	// CommissionRate tỷ lệ hoa hồng chuẩn hoá 0..1 (API có thể trả %
	// hoặc tỉ lệ — tự chuẩn hoá; 0 = không rõ).
	CommissionRate float64
	// CampaignID chiến dịch chứa sản phẩm (nếu API trả).
	CampaignID string
	// Merchant mã merchant/chiến dịch (vd "shopee") — để chỉ săn sản phẩm
	// thuộc chiến dịch đã được duyệt.
	Merchant string
	// UpdatedAt thời điểm datafeed cập nhật sản phẩm (zero = không rõ).
	UpdatedAt time.Time
}

// parseFeedTime đọc "12-10-2026T01:30:14" (định dạng thật của API) hoặc
// RFC3339. Zero khi không đọc được.
func parseFeedTime(s string) time.Time {
	for _, layout := range []string{"02-01-2006T15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Image trả về ảnh đầu tiên ("" khi không có).
func (p DatafeedProduct) Image() string {
	if len(p.Images) > 0 {
		return p.Images[0]
	}
	return ""
}

// DiscountPct tính % giảm giá từ giá gốc; 0 khi không đủ dữ liệu.
func (p DatafeedProduct) DiscountPct() float64 {
	if p.OldPrice > p.Price && p.OldPrice > 0 {
		return (p.OldPrice - p.Price) / p.OldPrice * 100
	}
	return 0
}

// DatafeedFilter tham số lọc cho ListDatafeeds.
type DatafeedFilter struct {
	// Domain lọc theo domain advertiser (vd "shopee.vn"), "" = tất cả.
	Domain string
	// MinPrice/MaxPrice lọc theo giá (0 = không lọc).
	MinPrice, MaxPrice float64
	// Limit số lượng tối đa (0 = mặc định, >200 bị cắt về 200).
	Limit int
	// Merchant lọc theo mã merchant (vd "shopee"), "" = tất cả.
	Merchant string
}

// ListDatafeeds gọi GET /v1/datafeeds, parse danh sách sản phẩm.
// API không có sort theo hoa hồng → sort HIGH_COMMISSION_RATE phía client
// (commission cao nhất trước). Không gọi khi chưa có key (fail-closed).
// Đợt Y: theo phân trang (page_token) tối đa 5 trang thay vì 1 trang duy
// nhất — trước đây sản phẩm ngoài top-200 không bao giờ được săn.
func (c *Client) ListDatafeeds(ctx context.Context, f DatafeedFilter) ([]DatafeedProduct, error) {
	limit := f.Limit
	if limit <= 0 || limit > maxDatafeedLimit {
		limit = maxDatafeedLimit
	}
	var out []DatafeedProduct
	pageToken := ""
	for page := 0; page < 5; page++ {
		q := url.Values{}
		if f.Domain != "" {
			q.Set("domain", f.Domain)
		}
		if f.Merchant != "" {
			q.Set("merchant", f.Merchant)
		}
		if f.MinPrice > 0 {
			q.Set("min_price", strconv.FormatFloat(f.MinPrice, 'f', -1, 64))
		}
		if f.MaxPrice > 0 {
			q.Set("max_price", strconv.FormatFloat(f.MaxPrice, 'f', -1, 64))
		}
		q.Set("limit", strconv.Itoa(limit))
		if pageToken != "" {
			q.Set("page_token", pageToken)
		}
		data, err := c.do(ctx, "GET", "/v1/datafeeds?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		raws, err := decodeList(data, "data", "datafeeds", "products", "results", "items")
		if err != nil {
			return nil, err
		}
		for _, raw := range raws {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			p := DatafeedProduct{
				SKU:      firstString(m, "sku", "product_id", "id", "item_id"),
				Name:     firstString(m, "name", "product_name", "title"),
				Price:    firstFloat(m, "price", "sale_price", "discounted_price"),
				OldPrice: firstFloat(m, "old_price", "original_price", "price_before_discount", "list_price"),
				Currency: firstString(m, "currency"),
				// Chỉ link có mã affiliate — KHÔNG lấy "url" (link gốc của
				// shop, không ra hoa hồng).
				AffLink:    firstString(m, "aff_link", "affiliate_link", "deeplink"),
				Category:   firstString(m, "category", "category_name", "cat"),
				ShopName:   firstString(m, "shop_name", "advertiser", "merchant", "domain"),
				CampaignID: firstString(m, "campaign_id", "campaignId"),
				Merchant:   firstString(m, "merchant", "campaign"),
				UpdatedAt:  parseFeedTime(firstString(m, "update_time", "updated_at")),
			}
			p.Images = stringList(m, "images", "image_urls", "photos")
			if img := firstString(m, "image", "image_url", "thumbnail", "picture"); img != "" {
				p.Images = append([]string{img}, p.Images...)
			}
			p.CommissionRate = normalizeRate(firstFloat(m,
				"commission_rate", "commissionRate", "commission", "rate"))
			if p.Name == "" && p.SKU == "" {
				continue
			}
			out = append(out, p)
		}
		next := datafeedNextToken(data)
		if next == "" {
			break
		}
		pageToken = next
	}
	// Sort client-side: hoa hồng cao nhất trước (ổn định theo tên).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CommissionRate != out[j].CommissionRate {
			return out[i].CommissionRate > out[j].CommissionRate
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// datafeedNextToken tách token trang tiếp theo khỏi response datafeed
// ("" khi hết). API không trả token → chỉ 1 trang, đúng như trước.
func datafeedNextToken(data []byte) string {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		return ""
	}
	return firstString(env, "next_page_token", "page_token", "nextPageToken", "cursor")
}

// normalizeRate chuẩn hoá tỷ lệ hoa hồng về 0..1: API trả 8.5 (= 8.5%)
// hay 0.085 đều về 0.085. Giá trị vô lý (>100%) → 0 (không bịa).
func normalizeRate(v float64) float64 {
	switch {
	case v <= 0:
		return 0
	case v <= 1:
		return v
	case v <= 100:
		return v / 100
	default:
		return 0
	}
}

// firstFloat lấy field số đầu tiên tồn tại (chấp nhận cả string số).
func firstFloat(m map[string]json.RawMessage, keys ...string) float64 {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		var f float64
		if err := json.Unmarshal(raw, &f); err == nil {
			return f
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

// stringList lấy field mảng string (chấp nhận string đơn lẻ).
func stringList(m map[string]json.RawMessage, keys ...string) []string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		var arr []string
		if err := json.Unmarshal(raw, &arr); err == nil {
			return arr
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && s != "" {
			return []string{s}
		}
	}
	return nil
}

// PriceLabel hiển thị giá trung thực (VND làm tròn, ngoại tệ 2 số lẻ).
// Đợt Y: math.Round thay vì cắt ngọn.
func (p DatafeedProduct) PriceLabel() string {
	cur := strings.ToUpper(strings.TrimSpace(p.Currency))
	if cur == "" || cur == "VND" || cur == "Đ" || cur == "₫" {
		return fmt.Sprintf("%s₫", Thousands(int64(math.Round(p.Price))))
	}
	return fmt.Sprintf("%s %.2f", cur, p.Price)
}

// Thousands định dạng số với dấu phân cách nghìn (1.234.567).
func Thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
}

// ListDatafeeds gọi GET /v1/datafeeds, parse danh sách sản phẩm.
// API không có sort theo hoa hồng → sort HIGH_COMMISSION_RATE phía client
// (commission cao nhất trước). Không gọi khi chưa có key (fail-closed).
func (c *Client) ListDatafeeds(ctx context.Context, f DatafeedFilter) ([]DatafeedProduct, error) {
	q := url.Values{}
	if f.Domain != "" {
		q.Set("domain", f.Domain)
	}
	if f.MinPrice > 0 {
		q.Set("min_price", strconv.FormatFloat(f.MinPrice, 'f', -1, 64))
	}
	if f.MaxPrice > 0 {
		q.Set("max_price", strconv.FormatFloat(f.MaxPrice, 'f', -1, 64))
	}
	limit := f.Limit
	if limit <= 0 || limit > maxDatafeedLimit {
		limit = maxDatafeedLimit
	}
	q.Set("limit", strconv.Itoa(limit))
	path := "/v1/datafeeds?" + q.Encode()

	data, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	raws, err := decodeList(data, "data", "datafeeds", "products", "results", "items")
	if err != nil {
		return nil, err
	}
	out := make([]DatafeedProduct, 0, len(raws))
	for _, raw := range raws {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		p := DatafeedProduct{
			SKU:        firstString(m, "sku", "product_id", "id", "item_id"),
			Name:       firstString(m, "name", "product_name", "title"),
			Price:      firstFloat(m, "price", "sale_price", "discounted_price"),
			OldPrice:   firstFloat(m, "old_price", "original_price", "price_before_discount", "list_price"),
			Currency:   firstString(m, "currency"),
			AffLink:    firstString(m, "aff_link", "affiliate_link", "deeplink", "url", "link"),
			Category:   firstString(m, "category", "category_name", "cat"),
			ShopName:   firstString(m, "shop_name", "advertiser", "merchant", "domain"),
			CampaignID: firstString(m, "campaign_id", "campaignId"),
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
	// Sort client-side: hoa hồng cao nhất trước (ổn định theo tên).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CommissionRate != out[j].CommissionRate {
			return out[i].CommissionRate > out[j].CommissionRate
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
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
func (p DatafeedProduct) PriceLabel() string {
	cur := strings.ToUpper(strings.TrimSpace(p.Currency))
	if cur == "" || cur == "VND" || cur == "Đ" || cur == "₫" {
		return fmt.Sprintf("%s₫", Thousands(int64(p.Price)))
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

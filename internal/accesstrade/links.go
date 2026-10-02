package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LinkRequest tham số tạo tracking link.
type LinkRequest struct {
	// URL link gốc của sản phẩm (bắt buộc).
	URL string
	// CampaignID id chiến dịch (nếu endpoint yêu cầu).
	CampaignID string
	// UTM các tham số utm (tuỳ chọn).
	UTM map[string]string
	// Sub1..Sub4 sub-id theo dõi per kênh (tuỳ chọn).
	Sub1, Sub2, Sub3, Sub4 string
}

// CreatedLink kết quả tạo link.
type CreatedLink struct {
	// TrackingLink link affiliate đầy đủ.
	TrackingLink string
	// ShortLink link rút gọn (nếu API trả).
	ShortLink string
}

// CreateProductLink gọi POST /v1/product_link/create để tạo tracking link.
// Link được lưu vào store bởi handler (không lưu ở đây để package client
// không phụ thuộc DB).
func (c *Client) CreateProductLink(ctx context.Context, req LinkRequest) (*CreatedLink, error) {
	u := strings.TrimSpace(req.URL)
	if u == "" {
		return nil, fmt.Errorf("accesstrade: thiếu URL sản phẩm")
	}
	body := map[string]any{"url": u}
	if req.CampaignID != "" {
		body["campaign_id"] = req.CampaignID
	}
	for k, v := range req.UTM {
		if v = strings.TrimSpace(v); v != "" {
			body["utm_"+k] = v
		}
	}
	for i, v := range []string{req.Sub1, req.Sub2, req.Sub3, req.Sub4} {
		if v = strings.TrimSpace(v); v != "" {
			body[fmt.Sprintf("sub%d", i+1)] = v
		}
	}
	data, err := c.do(ctx, "POST", "/v1/product_link/create", body)
	if err != nil {
		return nil, err
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("accesstrade: response tạo link không phải JSON hợp lệ")
	}
	// API có thể bọc trong "data" hoặc trả trực tiếp.
	m := env
	if raw, ok := env["data"]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err == nil {
			m = inner
		}
	}
	out := &CreatedLink{
		TrackingLink: firstString(m, "url", "tracking_url", "aff_link", "affiliate_url", "link"),
		ShortLink:    firstString(m, "short_url", "shorten_url", "short_link"),
	}
	if out.TrackingLink == "" {
		return nil, fmt.Errorf("accesstrade: API không trả về tracking link")
	}
	return out, nil
}

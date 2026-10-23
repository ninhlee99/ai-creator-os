package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LinkRequest tham số tạo tracking link.
type LinkRequest struct {
	// URL link gốc của sản phẩm (tuỳ chọn; API dùng URL campaign nếu trống).
	URL string
	// CampaignID id chiến dịch (bắt buộc theo API).
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
	campaignID := strings.TrimSpace(req.CampaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("accesstrade: thiếu campaign_id")
	}
	body := map[string]any{"campaign_id": campaignID}
	if u != "" {
		body["urls"] = []string{u}
	}
	for k, v := range req.UTM {
		key := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(k)), "utm_")
		if v = strings.TrimSpace(v); v != "" {
			switch key {
			case "source", "medium", "campaign", "content":
				body["utm_"+key] = v
			}
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
	if raw, ok := env["success"]; ok {
		var succeeded bool
		if json.Unmarshal(raw, &succeeded) == nil && !succeeded {
			return nil, fmt.Errorf("accesstrade: API từ chối tạo link")
		}
	}
	// API có thể bọc trong "data" hoặc trả trực tiếp.
	m := env
	if raw, ok := env["data"]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err == nil {
			m = inner
		}
	}
	if raw, ok := m["success_link"]; ok {
		var links []json.RawMessage
		if err := json.Unmarshal(raw, &links); err == nil && len(links) > 0 {
			var first map[string]json.RawMessage
			if json.Unmarshal(links[0], &first) == nil {
				m = first
			}
		}
	}
	out := &CreatedLink{
		TrackingLink: firstString(m, "aff_link", "url", "tracking_url", "affiliate_url", "link"),
		ShortLink:    firstString(m, "short_link", "short_url", "shorten_url"),
	}
	if out.TrackingLink == "" {
		return nil, fmt.Errorf("accesstrade: API không trả về tracking link")
	}
	return out, nil
}

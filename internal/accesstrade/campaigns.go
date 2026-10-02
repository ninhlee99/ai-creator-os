package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Campaign là một chiến dịch affiliate trên Accesstrade.
// Các field API trả về được giữ nguyên tên ý nghĩa; field nào API không
// trả thì để trống (zero value) — UI hiển thị "—" thay vì bịa số.
type Campaign struct {
	// ID của chiến dịch (campaign_id).
	ID string
	// Name tên chiến dịch.
	Name string
	// Approval trạng thái duyệt: "successful" = đã duyệt, còn lại
	// (pending/unregistered/...) = chưa được tạo link.
	Approval string
	// CommissionPolicy mô tả chính sách hoa hồng (text từ API, nếu có).
	CommissionPolicy string
	// CommissionRate tỷ lệ hoa hồng (số, nếu API trả; 0 = không rõ).
	CommissionRate float64
	// CookieDuration thời gian lưu cookie (text từ API, nếu có).
	CookieDuration string
}

// ApprovedOnly lọc chỉ chiến dịch đã duyệt (đủ điều kiện tạo link).
func ApprovedOnly(cs []Campaign) []Campaign {
	out := cs[:0:0]
	for _, c := range cs {
		if c.Approval == "successful" {
			out = append(out, c)
		}
	}
	return out
}

// CampaignFilter tham số lọc cho ListCampaigns.
type CampaignFilter struct {
	// Approval ví dụ "successful" — để trống = lấy tất cả.
	Approval string
	// Limit số lượng tối đa (0 = mặc định API).
	Limit int
}

// ListCampaigns gọi GET /v1/campaigns, parse danh sách chiến dịch.
// Không gọi khi chưa có key (client.do fail-closed).
func (c *Client) ListCampaigns(ctx context.Context, f CampaignFilter) ([]Campaign, error) {
	q := url.Values{}
	if f.Approval != "" {
		q.Set("approval", f.Approval)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	path := "/v1/campaigns"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	raws, err := decodeList(data, "data", "campaigns", "results", "items")
	if err != nil {
		return nil, err
	}
	out := make([]Campaign, 0, len(raws))
	for _, raw := range raws {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		cp := Campaign{
			ID:               firstString(m, "campaign_id", "id", "campaignId"),
			Name:             firstString(m, "name", "campaign_name", "title"),
			Approval:         firstString(m, "approval", "approval_status", "status"),
			CommissionPolicy: firstString(m, "commission_policy", "commission", "policy"),
			CookieDuration:   firstString(m, "cookie_duration", "cookie", "cookieDuration"),
		}
		if rs := firstString(m, "commission_rate", "commissionRate", "rate"); rs != "" {
			if f, err := strconv.ParseFloat(rs, 64); err == nil {
				cp.CommissionRate = f
			}
		}
		if cp.ID == "" && cp.Name == "" {
			continue
		}
		out = append(out, cp)
	}
	return out, nil
}

// ApprovalLabel trả về nhãn tiếng Việt ngắn cho trạng thái duyệt.
func (c Campaign) ApprovalLabel() string {
	switch c.Approval {
	case "successful":
		return "Đã duyệt"
	case "pending":
		return "Chờ duyệt"
	case "unregistered", "":
		return "Chưa đăng ký"
	default:
		return c.Approval
	}
}

// CommissionLabel hiển thị hoa hồng trung thực: có số thì hiện số,
// không có thì "—" (không bịa).
func (c Campaign) CommissionLabel() string {
	if c.CommissionRate > 0 {
		return fmt.Sprintf("%g%%", c.CommissionRate)
	}
	if c.CommissionPolicy != "" {
		return c.CommissionPolicy
	}
	return "—"
}

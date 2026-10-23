package accesstrade

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/url"
	"strconv"
	"strings"
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
	// Merchant mã merchant (vd "shopee") — khớp trường merchant của datafeed.
	Merchant string
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

// maxCampaignPages giới hạn số trang đọc mỗi lần (mỗi trang ≤50 chiến
// dịch, trang đầy đủ ~9MB vì mô tả dài).
const maxCampaignPages = 20

// ListCampaigns gọi GET /v1/campaigns, parse danh sách chiến dịch, đọc
// tiếp các trang theo "page"/"total_page" (đo thật 12/10/2026: API trả
// tối đa 50/trang, ~207 trang công khai; chỉ filter approval=successful có
// tác dụng — giá trị khác bị bỏ qua). Không gọi khi chưa có key.
func (c *Client) ListCampaigns(ctx context.Context, f CampaignFilter) ([]Campaign, error) {
	var out []Campaign
	for page := 1; page <= maxCampaignPages; page++ {
		q := url.Values{}
		if f.Approval != "" {
			q.Set("approval", f.Approval)
		}
		if f.Limit > 0 {
			q.Set("limit", strconv.Itoa(f.Limit))
		}
		if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}
		path := "/v1/campaigns"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		data, err := c.do(ctx, "GET", path, nil)
		if err != nil {
			return out, err
		}
		raws, err := decodeList(data, "data", "campaigns", "results", "items")
		if err != nil {
			return out, err
		}
		out = append(out, parseCampaigns(raws)...)
		if f.Limit > 0 && len(out) >= f.Limit {
			return out[:f.Limit], nil
		}
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(data, &meta)
		totalPages := 0
		var totalPage json.Number
		if raw, ok := meta["total_page"]; ok && json.Unmarshal(raw, &totalPage) == nil {
			if n, err := totalPage.Float64(); err == nil && n > 0 {
				totalPages = int(n)
			}
		}
		pageSize := f.Limit
		if pageSize <= 0 {
			pageSize = 50 // documented API default observed in current publisher response
		}
		total := int(firstFloat(meta, "total"))
		if totalPages == 0 && total > 0 {
			totalPages = (total + pageSize - 1) / pageSize
		}
		if len(raws) == 0 || (totalPages > 0 && page >= totalPages) ||
			(totalPages == 0 && len(raws) < pageSize) {
			break
		}
		// Đợt AM: chạm trần 20 trang mà vẫn còn → log warn thay vì cắt im lặng.
		if page == maxCampaignPages && totalPages > page {
			log.Printf("accesstrade: chạm trần %d trang (còn %d trang) — một số chiến dịch bị bỏ qua", maxCampaignPages, totalPages-page)
		}
	}
	return out, nil
}

func parseCampaigns(raws []json.RawMessage) []Campaign {
	out := make([]Campaign, 0, len(raws))
	for _, raw := range raws {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		var description map[string]json.RawMessage
		if rawDescription, ok := m["description"]; ok {
			_ = json.Unmarshal(rawDescription, &description)
		}
		commissionPolicy := firstString(m, "commission_policy", "commission", "policy", "max_com")
		if commissionPolicy == "" {
			commissionPolicy = firstString(description, "commission_policy", "conversion_policy")
		}
		commissionPolicy = plainText(commissionPolicy)
		cp := Campaign{
			ID:               firstString(m, "campaign_id", "id", "campaignId"),
			Name:             firstString(m, "name", "campaign_name", "title"),
			Approval:         firstString(m, "approval", "approval_status", "status"),
			CommissionPolicy: commissionPolicy,
			CookieDuration:   firstString(m, "cookie_duration", "cookie", "cookieDuration"),
			Merchant:         firstString(m, "merchant"),
		}
		if rs := firstString(m, "commission_rate", "commissionRate", "rate"); rs != "" {
			if f, err := strconv.ParseFloat(rs, 64); err == nil {
				// Đợt Y: chuẩn hoá về 0..1 như datafeed (API có thể trả
				// 8.5 hoặc 0.085) — trước đây 0.085 hiện thành "0.085%".
				cp.CommissionRate = normalizeRate(f)
			}
		}
		if cp.ID == "" && cp.Name == "" {
			continue
		}
		out = append(out, cp)
	}
	return out
}

func plainText(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(out.String())), " ")
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
// không có thì "—" (không bịa). CommissionRate đã chuẩn hoá 0..1
// (normalizeRate) nên nhân 100 khi hiển thị %; giá trị cache cũ (>1,
// lưu trước Đợt Y) được giữ nguyên để không hiện sai trong lúc chờ
// làm mới cache.
func (c Campaign) CommissionLabel() string {
	if c.CommissionRate > 0 {
		r := c.CommissionRate
		if r <= 1 {
			r *= 100
		}
		return fmt.Sprintf("%g%%", r)
	}
	if c.CommissionPolicy != "" {
		return c.CommissionPolicy
	}
	return "—"
}

package accesstrade

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// HunterSource là nguồn ghi trong products.Store cho sản phẩm săn từ AT.
const HunterSource = "accesstrade"

// HunterFilter tham số một lần quét datafeed.
type HunterFilter struct {
	// Domain lọc advertiser ("" = tất cả).
	Domain string
	// MinPrice/MaxPrice lọc giá (0 = không lọc).
	MinPrice, MaxPrice float64
	// Limit số sản phẩm lấy từ API mỗi lần quét (0 = 200).
	Limit int
	// MinCommissionRate ngưỡng hoa hồng 0..1 (0 = không lọc).
	MinCommissionRate float64
	// ThemeFallback theme gán khi không map được từ category ("" = để trống).
	ThemeFallback string
	// ApprovedOnly: chỉ săn sản phẩm của merchant thuộc chiến dịch bạn đã
	// được duyệt (link chiến dịch chưa duyệt không ra hoa hồng).
	ApprovedOnly bool
	// MaxAgeDays bỏ sản phẩm datafeed cũ hơn N ngày (ảnh/link hay đã chết;
	// đo thật: feed 30shine cập nhật 2023 → 100% ảnh 404). 0 = không lọc.
	MaxAgeDays int
}

// HuntResult kết quả một lần săn.
type HuntResult struct {
	// New số sản phẩm mới thêm vào kho.
	New int
	// Updated số sản phẩm đã có được làm mới.
	Updated int
	// Skipped số bỏ qua (thiếu aff_link/giá).
	Skipped int
	// Products sản phẩm MỚI, sort theo hoa hồng kỳ vọng (giá × tỷ lệ)
	// giảm dần — video được làm trước cho sản phẩm "ngon" nhất.
	Products []products.Product
	// Note giải thích cho người dùng khi không săn được gì (vd chưa có
	// chiến dịch sản phẩm nào được duyệt).
	Note string
}

// Hunt quét datafeed Accesstrade → lọc (có aff_link, giá > 0) → lưu vào
// products.Store (Source="accesstrade", dedupe theo aff_link/SKU).
// Không gọi khi chưa có key (fail-closed từ client.do).
func (c *Client) Hunt(ctx context.Context, ps *products.Store, f HunterFilter) (*HuntResult, error) {
	if ps == nil {
		return nil, fmt.Errorf("accesstrade: kho sản phẩm chưa sẵn sàng")
	}
	res := &HuntResult{}
	var feed []DatafeedProduct
	if f.ApprovedOnly {
		cs, err := c.ListCampaigns(ctx, CampaignFilter{Approval: "successful"})
		if err != nil {
			return nil, err
		}
		var names []string
		seen := map[string]bool{}
		var noMerchant []string
		for _, cp := range cs {
			names = append(names, cp.Name)
			if cp.Merchant == "" {
				// Đợt AM: ghi nhận riêng thay vì bỏ qua im lặng.
				noMerchant = append(noMerchant, cp.Name)
				continue
			}
			// Đợt AM: dedupe merchant — 2 campaign cùng shop chỉ gọi API 1 lần.
			if seen[cp.Merchant] {
				continue
			}
			seen[cp.Merchant] = true
			part, err := c.ListDatafeeds(ctx, DatafeedFilter{Merchant: cp.Merchant,
				Domain: f.Domain, MinPrice: f.MinPrice, MaxPrice: f.MaxPrice, Limit: f.Limit})
			if err != nil {
				return nil, err
			}
			feed = append(feed, part...)
		}
		if len(noMerchant) > 0 {
			note := fmt.Sprintf("Bỏ qua %d chiến dịch không rõ merchant: %s", len(noMerchant), strings.Join(noMerchant, ", "))
			if res.Note != "" {
				res.Note += "; " + note
			} else {
				res.Note = note
			}
		}
		switch {
		case len(cs) == 0:
			res.Note = "Chưa có chiến dịch Accesstrade nào được duyệt — đăng ký chiến dịch (Shopee, Lazada, Tiki…) trên pub.accesstrade.vn rồi chạy lại."
			return res, nil
		case len(feed) == 0:
			res.Note = "Chiến dịch đã duyệt (" + strings.Join(names, ", ") + ") không có datafeed sản phẩm — đăng ký thêm chiến dịch bán hàng (Shopee, Lazada, Tiki…) trên pub.accesstrade.vn."
			return res, nil
		}
	} else {
		var err error
		feed, err = c.ListDatafeeds(ctx, DatafeedFilter{
			Domain: f.Domain, MinPrice: f.MinPrice, MaxPrice: f.MaxPrice, Limit: f.Limit,
		})
		if err != nil {
			return nil, err
		}
	}
	stale := 0
	for _, dp := range feed {
		if f.MaxAgeDays > 0 && !dp.UpdatedAt.IsZero() &&
			time.Since(dp.UpdatedAt) > time.Duration(f.MaxAgeDays)*24*time.Hour {
			res.Skipped++
			stale++
			continue
		}
		if strings.TrimSpace(dp.AffLink) == "" || dp.Price <= 0 {
			res.Skipped++
			continue
		}
		if f.MinCommissionRate > 0 && dp.CommissionRate < f.MinCommissionRate {
			res.Skipped++
			continue
		}
		p := mapDatafeed(dp, f.ThemeFallback)
		_, gerr := ps.GetBySource(p.Source, p.SourceID)
		isNew := gerr != nil
		id, serr := ps.Save(p)
		if serr != nil {
			res.Skipped++
			continue
		}
		p.ID = id
		if isNew {
			res.New++
			res.Products = append(res.Products, p)
		} else {
			res.Updated++
		}
	}
	if stale > 0 && res.New == 0 && res.Updated == 0 {
		res.Note = fmt.Sprintf("Bỏ %d sản phẩm datafeed quá cũ (>%d ngày, ảnh/link thường đã chết).", stale, f.MaxAgeDays)
	}
	// Sort thật theo hoa hồng kỳ vọng (giá × tỷ lệ) giảm dần — video
	// affiliate được làm trước cho sản phẩm "ngon" nhất.
	sort.Slice(res.Products, func(i, j int) bool {
		return res.Products[i].Price*res.Products[i].CommissionRate >
			res.Products[j].Price*res.Products[j].CommissionRate
	})
	return res, nil
}

// mapDatafeed chuyển DatafeedProduct → products.Product.
func mapDatafeed(dp DatafeedProduct, themeFallback string) products.Product {
	sourceID := strings.TrimSpace(dp.SKU)
	if sourceID == "" {
		// Dedupe theo aff_link khi không có SKU.
		sum := sha256.Sum256([]byte(dp.AffLink))
		sourceID = fmt.Sprintf("link-%x", sum[:8])
	}
	theme := themeFor(dp.Category, dp.Name)
	if theme == "" {
		theme = themeFallback
	}
	cur := strings.ToUpper(strings.TrimSpace(dp.Currency))
	if cur == "" {
		cur = "VND" // datafeed Accesstrade Vietnam
	}
	return products.Product{
		Source:         HunterSource,
		SourceID:       sourceID,
		Title:          dp.Name,
		ImageURLs:      dp.Images,
		Price:          dp.Price,
		Currency:       cur,
		CommissionRate: dp.CommissionRate,
		ShopName:       dp.ShopName,
		Category:       dp.Category,
		ProductURL:     dp.AffLink,
		Theme:          theme,
	}
}

// themeFor map category datafeed → theme slug của app (keyword đơn giản;
// không chắc thì trả "" để sản phẩm nằm ở kho discovery, không gán sai).
func themeFor(category, name string) string {
	hay := strings.ToLower(category + " " + name)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(hay, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("thời trang", "fashion", "quần áo", "váy", "đầm", "giày", "túi xách", "phụ kiện", "đồng hồ", "kính"):
		return "thoi-trang-nu"
	case has("mỹ phẩm", "cosmetic", "skincare", "makeup", "son ", "kem ", "serum", "nước hoa", "beauty"):
		return "my-pham"
	case has("gia dụng", "nhà bếp", "nồi", "chảo", "bếp", "nội thất", "điện gia dụng", "home"):
		return "gia-dung"
	case has("mẹ và bé", "em bé", "sữa", "bỉm", "đồ chơi", "baby", "kids"):
		return "me-va-be"
	case has("công nghệ", "điện tử", "tai nghe", "điện thoại", "laptop", "tech", "gadget"):
		return "cong-nghe"
	case has("thể thao", "gym", "yoga", "sport", "fitness"):
		return "the-thao"
	case has("sách", "văn phòng", "book", "stationery"):
		return "sach-van-phong"
	default:
		return ""
	}
}

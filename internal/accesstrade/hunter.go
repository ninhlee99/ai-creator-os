package accesstrade

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

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
}

// Hunt quét datafeed Accesstrade → lọc (có aff_link, giá > 0) → lưu vào
// products.Store (Source="accesstrade", dedupe theo aff_link/SKU).
// Không gọi khi chưa có key (fail-closed từ client.do).
func (c *Client) Hunt(ctx context.Context, ps *products.Store, f HunterFilter) (*HuntResult, error) {
	if ps == nil {
		return nil, fmt.Errorf("accesstrade: kho sản phẩm chưa sẵn sàng")
	}
	feed, err := c.ListDatafeeds(ctx, DatafeedFilter{
		Domain: f.Domain, MinPrice: f.MinPrice, MaxPrice: f.MaxPrice, Limit: f.Limit,
	})
	if err != nil {
		return nil, err
	}
	res := &HuntResult{}
	for _, dp := range feed {
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

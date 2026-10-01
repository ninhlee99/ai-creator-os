package products

import (
	"context"
	"fmt"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// Provider finds affiliate products for a theme.
type Provider interface {
	// Name is the stable source tag stored on products.
	Name() string
	// Search returns candidates for q. It must honor q.Limit when > 0.
	Search(ctx context.Context, q Query) ([]Product, error)
}

// ---------------------------------------------------------------------------
// TikTok Shop provider
// ---------------------------------------------------------------------------

// TikTokShopProvider searches the Affiliate Product Marketplace (open
// collaboration) through internal/tiktok.ShopClient.
//
// HONEST LIMIT: the ShopClient endpoint paths are still unverified in the
// Partner Center sandbox and production needs an approved app + creator
// OAuth (human steps). Until then Search fails closed with a clear message
// instead of guessing URLs. This is deliberate — see shop.go.
type TikTokShopProvider struct {
	Client *tiktok.ShopClient
}

func (p *TikTokShopProvider) Name() string { return "tiktok_shop" }

// Configured reports whether the provider can actually fire requests.
func (p *TikTokShopProvider) Configured() bool {
	return p.Client != nil && p.Client.AppKey != "" &&
		p.Client.AppSecret != "" && p.Client.AccessToken != "" &&
		tiktok.Endpoints["affiliate_open_collab_search"] != ""
}

func (p *TikTokShopProvider) Search(ctx context.Context, q Query) ([]Product, error) {
	if !p.Configured() {
		return nil, fmt.Errorf("tiktok_shop: chưa cấu hình — cần (1) verify endpoint " +
			"affiliate_open_collab_search trong Partner Center sandbox, (2) app đã duyệt, " +
			"(3) creator OAuth access token. Đây là bước con người; app không đoán URL.")
	}
	keyword := strings.Join(q.Keywords, " ")
	if keyword == "" {
		keyword = q.Theme
	}
	limit := q.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	minComm := int(q.MinCommission * 100)
	data, err := p.Client.SearchOpenCollabProducts(keyword, "", minComm, 1, limit)
	if err != nil {
		return nil, fmt.Errorf("tiktok_shop search: %w", err)
	}
	return parseShopProducts(data, q.Theme), nil
}

// parseShopProducts converts the Shop API payload into Products. Unknown
// shapes are skipped rather than misread.
func parseShopProducts(data map[string]any, theme string) []Product {
	var out []Product
	raw, _ := data["products"].([]any)
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		p := Product{Source: "tiktok_shop", Theme: theme}
		p.SourceID = strOf(m["product_id"], m["id"])
		p.Title = strOf(m["product_name"], m["title"], m["name"])
		if p.Title == "" || p.SourceID == "" {
			continue
		}
		p.Price = fOf(m["price"])
		p.Currency = strOf(m["currency"])
		if cr := fOf(m["commission_rate"]); cr > 1 {
			p.CommissionRate = cr / 100 // API may return percent
		} else {
			p.CommissionRate = cr
		}
		p.ShopName = strOf(m["shop_name"], m["seller_name"])
		p.Rating = fOf(m["rating"], m["product_rating"])
		p.SoldCount = iOf(m["sold_count"], m["sales"])
		p.Category = strOf(m["category_name"], m["category"])
		p.ProductURL = strOf(m["product_url"], m["share_url"])
		for _, k := range []string{"image_urls", "images", "main_images"} {
			if arr, ok := m[k].([]any); ok {
				for _, u := range arr {
					if s, ok := u.(string); ok && s != "" {
						p.ImageURLs = append(p.ImageURLs, s)
					}
				}
				if len(p.ImageURLs) > 0 {
					break
				}
			}
		}
		if u, ok := m["main_image"].(string); ok && u != "" && len(p.ImageURLs) == 0 {
			p.ImageURLs = []string{u}
		}
		out = append(out, p)
	}
	return out
}

func strOf(vs ...any) string {
	for _, v := range vs {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func fOf(vs ...any) float64 {
	for _, v := range vs {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}

func iOf(vs ...any) int64 {
	for _, v := range vs {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int:
			return int64(n)
		case int64:
			return n
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// aggregation
// ---------------------------------------------------------------------------

// Aggregate queries every provider, merges, filters, ranks. A provider that
// errors is skipped but reported — one dead source never kills discovery.
func Aggregate(ctx context.Context, providers []Provider, q Query) ([]Product, []error) {
	var all []Product
	var errs []error
	seen := map[string]bool{}
	for _, pv := range providers {
		ps, err := pv.Search(ctx, q)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pv.Name(), err))
			continue
		}
		for _, p := range ps {
			if p.Source == "" {
				p.Source = pv.Name()
			}
			key := p.Source + "\x00" + p.SourceID
			if p.SourceID == "" || seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, p)
		}
	}
	return Rank(Filter(all, q)), errs
}

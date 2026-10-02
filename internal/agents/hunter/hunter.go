// Package hunter is the Go port of agents/hunter/agent.py.
//
// Hunter discovers high-commission affiliate products. Scheduled daily.
// It pulls candidates from the TikTok Shop affiliate marketplace, scores
// them by EXPECTED EARNINGS (not commission % alone), passes governance,
// and writes the shelf to the ledger.
//
// TikTok Shop API client details: docs/RESEARCH.md §4.
// Endpoint paths must be verified in the Partner Center sandbox first —
// until then the underlying client fails closed with a clear error, and
// Run returns {"ok": False, "reason": ...} just like the Python agent.
package hunter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// Candidate is one affiliate-marketplace product under consideration.
type Candidate struct {
	PlatformPID    string
	Title          string
	Category       string
	Price          float64 // VND
	CommissionRate float64 // 0..1
	SellerRating   *float64
	ConversionRate float64 // estimated, 0..1
	Competition    float64 // 0..n, higher = more crowded
}

// BlockedCategories mirrors Python's BLOCKED_CATEGORIES.
var BlockedCategories = []string{"thuốc", "thực phẩm chức năng không rõ nguồn gốc"}

// Keywords are the discovery queries. TODO: keyword list from config /
// past winners.
var Keywords = []string{"gia dụng", "làm đẹp", "thời trang"}

// ProductSource abstracts the affiliate marketplace search so tests can
// inject a fake without touching the network.
type ProductSource interface {
	SearchProducts(ctx context.Context, keyword string) ([]map[string]any, error)
}

// TikTokShopAffiliateClient is a thin wrapper over the TikTok Shop Open
// Platform affiliate endpoints (HMAC-signed via tiktok.ShopClient).
type TikTokShopAffiliateClient struct {
	cfg    config.Config
	client *tiktok.ShopClient
}

// NewTikTokShopAffiliateClient builds the client. It errors when the shop
// credentials are missing, like Python's RuntimeError in _get().
func NewTikTokShopAffiliateClient(cfg config.Config) (*TikTokShopAffiliateClient, error) {
	if cfg.TikTokShopAppKey == "" || cfg.TikTokShopAccessToken == "" {
		return nil, fmt.Errorf("missing TIKTOK_SHOP_APP_KEY / TIKTOK_SHOP_ACCESS_TOKEN " +
			"(see docs/USER_GUIDE.md for the setup checklist)")
	}
	return &TikTokShopAffiliateClient{cfg: cfg, client: &tiktok.ShopClient{
		AppKey:      cfg.TikTokShopAppKey,
		AppSecret:   cfg.TikTokShopAppSecret,
		AccessToken: cfg.TikTokShopAccessToken,
		ShopCipher:  cfg.TikTokShopCipher,
	}}, nil
}

// SearchProducts queries the affiliate marketplace's open-collab products.
func (c *TikTokShopAffiliateClient) SearchProducts(ctx context.Context, keyword string) ([]map[string]any, error) {
	data, err := c.client.SearchOpenCollabProducts(keyword, "", 0, 1, 20)
	if err != nil {
		return nil, err
	}
	return productList(data), nil
}

// CommissionReport is a placeholder: via AffiliateOrders() once the
// endpoint path is verified.
func (c *TikTokShopAffiliateClient) CommissionReport(_ context.Context) ([]map[string]any, error) {
	return nil, nil
}

// productList extracts the product list from a shop response envelope.
// Python: data.get("products", data) when data is a dict.
func productList(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	raw, ok := data["products"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func asFloat(m map[string]any, key string, def float64) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
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
	return def
}

func asFloatPtr(m map[string]any, key string) *float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	f := asFloat(m, key, 0)
	return &f
}

func toCandidate(r map[string]any) Candidate {
	return Candidate{
		PlatformPID:    asString(r, "id"),
		Title:          asString(r, "title"),
		Category:       asString(r, "category"),
		Price:          asFloat(r, "price", 0),
		CommissionRate: asFloat(r, "commission_rate", 0),
		SellerRating:   asFloatPtr(r, "seller_rating"),
		ConversionRate: asFloat(r, "conversion_rate", 0.02),
		Competition:    asFloat(r, "competition", 1.0),
	}
}

// Run discovers candidates, gates them through governance, and shelves
// the winners. Failures from the shop client (unverified endpoint paths,
// missing credentials) surface as {"ok": False, "reason": ...}.
func Run(ctx context.Context, cfg config.Config, l *ledger.Ledger, client ProductSource) map[string]any {
	v := governance.CheckKillSwitch(cfg)
	if !v.Allowed {
		return map[string]any{"ok": false, "reason": v.Reason}
	}
	if client == nil {
		c, err := NewTikTokShopAffiliateClient(cfg)
		if err != nil {
			return map[string]any{"ok": false, "reason": err.Error()}
		}
		client = c
	}

	added, rejected := 0, 0
	for _, keyword := range Keywords {
		raw, err := client.SearchProducts(ctx, keyword)
		if err != nil {
			return map[string]any{"ok": false, "reason": err.Error()}
		}
		for _, r := range raw {
			cand := toCandidate(r)
			ev := governance.ProductEligible(cfg, cand.Price, cand.CommissionRate,
				cand.SellerRating, cand.Category, BlockedCategories)
			if !ev.Allowed {
				rejected++
				continue
			}
			score := governance.HunterScore(cand.Price, cand.CommissionRate,
				cand.ConversionRate, cand.Competition)
			var rating any
			if cand.SellerRating != nil {
				rating = *cand.SellerRating
			}
			pid, err := l.UpsertProduct(cand.PlatformPID, map[string]any{
				"title":            cand.Title,
				"category":         cand.Category,
				"price":            cand.Price,
				"commission_rate":  cand.CommissionRate,
				"commission_value": cand.Price * cand.CommissionRate,
				"seller_rating":    rating,
				"score":            score,
				"status":           "shelf",
			})
			if err != nil {
				return map[string]any{"ok": false, "reason": err.Error()}
			}
			target := strconv.FormatInt(pid, 10)
			if err := l.Decide("hunter", "shelf_product", &target,
				fmt.Sprintf("score=%.0f", score),
				map[string]any{"keyword": keyword}); err != nil {
				return map[string]any{"ok": false, "reason": err.Error()}
			}
			added++
		}
	}
	return map[string]any{"ok": true, "added": added, "rejected": rejected}
}

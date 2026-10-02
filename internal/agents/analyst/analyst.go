// Package analyst is the Go port of agents/analyst/agent.py.
//
// Analyst: money. Runs after each live session + daily rollup.
//
//   - Pulls orders/commissions from the TikTok Shop API (PROVIDER EVIDENCE
//     ONLY).
//   - Applies kill/scale rules via governance. Analyst proposes;
//     governance disposes.
//   - Never invents numbers. Never spends.
//
// Semantic difference from Python: the original review_products counted
// "sessions featured" with a raw SQL query (live_events payload LIKE
// %title[:20]%). The Go ledger now exposes SessionsFeatured (exact
// product_id match on product_moment events, title-prefix fallback for
// legacy rows), so both kill triggers apply again.
package analyst

import (
	"context"
	"strconv"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// OrdersClient is the shop-client surface the analyst needs. The real
// *tiktok.ShopClient satisfies it; tests inject a fake.
type OrdersClient interface {
	AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error)
}

// ReconcileOrders fetches affiliate orders from the TikTok Shop API and
// inserts them idempotently (duplicate platform_oid -> (0, nil)).
func ReconcileOrders(ctx context.Context, cfg config.Config, l *ledger.Ledger, shopClient OrdersClient) (int, error) {
	if v := governance.CheckKillSwitch(cfg); !v.Allowed {
		return 0, nil
	}
	now := time.Now().Unix()
	data, err := shopClient.AffiliateOrders(now-86400, now, 1, 50)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range orderList(data) {
		rid, err := l.RecordOrder(asString(o, "order_id"), map[string]any{
			"product_id": asInt64OrNil(o, "product_id"),
			"session_id": asInt64OrNil(o, "session_id"),
			"amount":     asFloat(o, "amount", 0),
			"commission": asFloat(o, "commission", 0),
			"ordered_at": asString(o, "ordered_at"),
		})
		if err != nil {
			return n, err
		}
		if rid != 0 {
			n++
		}
	}
	return n, nil
}

// ReviewProducts applies kill/scale rules to every shelf/scaled product
// and records the decisions in the ledger.
func ReviewProducts(cfg config.Config, l *ledger.Ledger) (map[string][]string, error) {
	review := map[string][]string{"killed": {}, "scaled": {}, "kept": {}}
	products, err := l.GetProducts()
	if err != nil {
		return nil, err
	}
	for _, p := range products {
		if p.Status != "shelf" && p.Status != "scaled" {
			continue
		}
		stats, err := l.ProductStats(p.ID)
		if err != nil {
			return nil, err
		}
		sessionsFeatured, err := l.SessionsFeatured(p.ID)
		if err != nil {
			return nil, err
		}
		kv := governance.ShouldKillProduct(cfg, stats, sessionsFeatured)
		target := strconv.FormatInt(p.ID, 10)
		inputs := map[string]any{
			"orders": stats.Orders, "revenue": stats.Revenue,
			"commission": stats.Commission, "views": stats.Views,
			"sessions_featured": sessionsFeatured,
		}
		if kv.Allowed {
			if err := l.SetProductStatus(p.ID, "killed"); err != nil {
				return nil, err
			}
			if err := l.Decide("analyst", "kill_product", &target, kv.Reason, inputs); err != nil {
				return nil, err
			}
			review["killed"] = append(review["killed"], p.PlatformPID)
			continue
		}
		sv := governance.ShouldScaleProduct(stats, 5, 2.0)
		if sv.Allowed {
			if err := l.SetProductStatus(p.ID, "scaled"); err != nil {
				return nil, err
			}
			if err := l.Decide("analyst", "scale_product", &target, sv.Reason, inputs); err != nil {
				return nil, err
			}
			review["scaled"] = append(review["scaled"], p.PlatformPID)
		} else {
			review["kept"] = append(review["kept"], p.PlatformPID)
		}
	}
	return review, nil
}

// Run reconciles orders (when a shop client is given) and reviews the
// shelf. Unverified shop endpoints fail closed and surface as
// {"ok": False, "reason": ...}.
func Run(ctx context.Context, cfg config.Config, l *ledger.Ledger, shopClient OrdersClient) map[string]any {
	if v := governance.CheckKillSwitch(cfg); !v.Allowed {
		return map[string]any{"ok": false, "reason": v.Reason}
	}
	reconciled := 0
	if shopClient != nil {
		n, err := ReconcileOrders(ctx, cfg, l, shopClient)
		if err != nil {
			return map[string]any{"ok": false, "reason": err.Error()}
		}
		reconciled = n
	}
	review, err := ReviewProducts(cfg, l)
	if err != nil {
		return map[string]any{"ok": false, "reason": err.Error()}
	}
	return map[string]any{
		"ok": true, "orders_reconciled": reconciled,
		"killed": review["killed"], "scaled": review["scaled"], "kept": review["kept"],
	}
}

// ---- response extraction helpers ----

func orderList(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	items, ok := data["orders"].([]any)
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

func asInt64OrNil(m map[string]any, key string) any {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return nil
}

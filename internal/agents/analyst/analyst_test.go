//go:build parked

package analyst

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func testCfg() governance.Config {
	return governance.Config{
		DryRun:              false,
		KillViewsNoOrder:    10_000,
		KillSessionsNoOrder: 3,
	}
}

func testLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	l, err := ledger.New(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func shelfProduct(t *testing.T, l *ledger.Ledger, pid string) int64 {
	t.Helper()
	id, err := l.UpsertProduct(pid, map[string]any{
		"title": pid + " title", "price": 100000.0,
		"commission_rate": 0.1, "commission_value": 10000.0,
		"status": "shelf",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// fakeOrders returns 2 orders per call.
type fakeOrders struct{}

func (fakeOrders) AffiliateOrders(_, _ int64, _, _ int) (map[string]any, error) {
	return map[string]any{"orders": []any{
		map[string]any{"order_id": "o1", "amount": 100000.0, "commission": 10000.0, "ordered_at": "2026-10-01"},
		map[string]any{"order_id": "o2", "amount": 200000.0, "commission": 20000.0, "ordered_at": "2026-10-01"},
	}}, nil
}

func TestReconcileIdempotent(t *testing.T) {
	l := testLedger(t)
	n, err := ReconcileOrders(context.Background(), testCfg(), l, fakeOrders{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("first reconcile = %d, want 2", n)
	}
	// Duplicate platform_oid: ignored, like Python.
	n, err = ReconcileOrders(context.Background(), testCfg(), l, fakeOrders{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second reconcile = %d, want 0", n)
	}
}

func TestReconcileKillSwitch(t *testing.T) {
	l := testLedger(t)
	cfg := testCfg()
	cfg.KillSwitch = true
	n, err := ReconcileOrders(context.Background(), cfg, l, fakeOrders{})
	if err != nil || n != 0 {
		t.Errorf("kill switch: n=%d err=%v", n, err)
	}
}

type errOrders struct{}

func (errOrders) AffiliateOrders(_, _ int64, _, _ int) (map[string]any, error) {
	return nil, errors.New("endpoint \"affiliate_orders_search\" has no verified path yet")
}

func TestRunShopErrorFailsClosed(t *testing.T) {
	l := testLedger(t)
	res := Run(context.Background(), testCfg(), l, errOrders{})
	if res["ok"] != false {
		t.Fatalf("expected ok=false, got %v", res)
	}
	if _, ok := res["reason"].(string); !ok {
		t.Errorf("missing reason: %v", res)
	}
}

func TestReviewKillRule(t *testing.T) {
	l := testLedger(t)
	cfg := testCfg()
	cfg.KillViewsNoOrder = 0 // 0 views >= 0 with 0 orders -> kill trigger
	id := shelfProduct(t, l, "doomed")
	review, err := ReviewProducts(cfg, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(review["killed"]) != 1 || review["killed"][0] != "doomed" {
		t.Errorf("killed = %v", review["killed"])
	}
	prods, _ := l.GetProducts()
	for _, p := range prods {
		if p.ID == id && p.Status != "killed" {
			t.Errorf("status = %q, want killed", p.Status)
		}
	}
}

func TestReviewScaleRule(t *testing.T) {
	l := testLedger(t)
	id := shelfProduct(t, l, "winner")
	for i := 0; i < 5; i++ {
		if _, err := l.RecordOrder("so"+string(rune('0'+i)), map[string]any{
			"product_id": id, "amount": 100000.0,
			"commission": 10000.0, "ordered_at": "2026-10-01",
		}); err != nil {
			t.Fatal(err)
		}
	}
	review, err := ReviewProducts(testCfg(), l)
	if err != nil {
		t.Fatal(err)
	}
	if len(review["scaled"]) != 1 || review["scaled"][0] != "winner" {
		t.Errorf("scaled = %v", review["scaled"])
	}
}

func TestReviewKept(t *testing.T) {
	l := testLedger(t)
	shelfProduct(t, l, "steady")
	review, err := ReviewProducts(testCfg(), l)
	if err != nil {
		t.Fatal(err)
	}
	if len(review["kept"]) != 1 || review["kept"][0] != "steady" {
		t.Errorf("kept = %v (killed=%v scaled=%v)", review["kept"], review["killed"], review["scaled"])
	}
}

func TestRunFull(t *testing.T) {
	l := testLedger(t)
	shelfProduct(t, l, "steady")
	res := Run(context.Background(), testCfg(), l, fakeOrders{})
	if res["ok"] != true {
		t.Fatalf("ok = %v (%v)", res["ok"], res["reason"])
	}
	if res["orders_reconciled"] != 2 {
		t.Errorf("orders_reconciled = %v, want 2", res["orders_reconciled"])
	}
}

package hunter

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func testCfg() config.Config {
	return config.Config{
		DryRun:          false,
		MinSellerRating: 4.0,
		MaxPrice:        1_000_000,
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

// fakeSource returns 2 products per keyword: one eligible, one blocked.
type fakeSource struct{}

func (fakeSource) SearchProducts(_ context.Context, _ string) ([]map[string]any, error) {
	return []map[string]any{
		{
			"id": "p1", "title": "Máy xay sinh tố", "category": "gia dụng",
			"price": 450000.0, "commission_rate": 0.12,
			"seller_rating": 4.6, "conversion_rate": 0.03, "competition": 1.0,
		},
		{
			"id": "p2", "title": "Thuốc giảm cân X", "category": "thuốc",
			"price": 300000.0, "commission_rate": 0.20,
			"seller_rating": 4.8, "conversion_rate": 0.05, "competition": 0.5,
		},
	}, nil
}

func TestRunAddsOneRejectsOne(t *testing.T) {
	l := testLedger(t)
	res := Run(context.Background(), testCfg(), l, fakeSource{})
	if res["ok"] != true {
		t.Fatalf("ok = %v (%v)", res["ok"], res["reason"])
	}
	if res["added"] != 3 || res["rejected"] != 3 {
		t.Errorf("added/rejected = %v/%v, want 3/3 (2 products x %d keywords)",
			res["added"], res["rejected"], len(Keywords))
	}
	prods, err := l.GetProducts()
	if err != nil {
		t.Fatal(err)
	}
	if len(prods) != 1 || prods[0].PlatformPID != "p1" {
		t.Errorf("shelf = %+v, want only p1", prods)
	}
	if prods[0].Status != "shelf" {
		t.Errorf("status = %q, want shelf", prods[0].Status)
	}
	// score = 450000*0.12*0.03 / 2 = 810
	if prods[0].Score < 809 || prods[0].Score > 811 {
		t.Errorf("score = %v, want ~810", prods[0].Score)
	}
}

func TestRunKillSwitch(t *testing.T) {
	l := testLedger(t)
	cfg := testCfg()
	cfg.KillSwitch = true
	res := Run(context.Background(), cfg, l, fakeSource{})
	if res["ok"] != false {
		t.Errorf("expected ok=false, got %v", res)
	}
}

type errSource struct{}

func (errSource) SearchProducts(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, errors.New("endpoint \"affiliate_open_collab_search\" has no verified path yet")
}

func TestRunClientError(t *testing.T) {
	l := testLedger(t)
	res := Run(context.Background(), testCfg(), l, errSource{})
	if res["ok"] != false {
		t.Fatalf("expected ok=false, got %v", res)
	}
	if _, ok := res["reason"].(string); !ok {
		t.Errorf("missing reason: %v", res)
	}
}

func TestNewClientMissingCreds(t *testing.T) {
	if _, err := NewTikTokShopAffiliateClient(config.Config{}); err == nil {
		t.Error("expected error for missing credentials")
	}
	cfg := config.Config{TikTokShopAppKey: "k", TikTokShopAccessToken: "tok"}
	if _, err := NewTikTokShopAffiliateClient(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestToCandidateDefaults(t *testing.T) {
	c := toCandidate(map[string]any{"id": "x", "title": "T"})
	if c.ConversionRate != 0.02 || c.Competition != 1.0 {
		t.Errorf("defaults wrong: %+v", c)
	}
	if c.SellerRating != nil {
		t.Errorf("nil rating expected, got %v", *c.SellerRating)
	}
}

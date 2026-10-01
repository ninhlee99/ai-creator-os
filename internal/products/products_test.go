package products

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestScoreCommissionDominates(t *testing.T) {
	high := Product{CommissionRate: 0.30, SoldCount: 100, Rating: 4.0}
	low := Product{CommissionRate: 0.05, SoldCount: 100000, Rating: 5.0}
	if Score(high) <= Score(low) {
		t.Fatalf("commission should dominate: high=%v low=%v", Score(high), Score(low))
	}
}

func TestRankOrder(t *testing.T) {
	ps := []Product{
		{SourceID: "c", CommissionRate: 0.10},
		{SourceID: "a", CommissionRate: 0.25},
		{SourceID: "b", CommissionRate: 0.15},
	}
	r := Rank(ps)
	if r[0].SourceID != "a" || r[1].SourceID != "b" || r[2].SourceID != "c" {
		t.Fatalf("bad rank order: %v %v %v", r[0].SourceID, r[1].SourceID, r[2].SourceID)
	}
}

func TestFilter(t *testing.T) {
	ps := []Product{
		{CommissionRate: 0.20, SoldCount: 500, Price: 100},
		{CommissionRate: 0.05, SoldCount: 500, Price: 100},
		{CommissionRate: 0.20, SoldCount: 5, Price: 100},
		{CommissionRate: 0.20, SoldCount: 500, Price: 99999},
	}
	f := Filter(ps, Query{MinCommission: 0.10, MinSold: 100, MaxPrice: 1000})
	if len(f) != 1 || f[0].CommissionRate != 0.20 || f[0].SoldCount != 500 {
		t.Fatalf("filter kept %d, want 1", len(f))
	}
}

type stubProvider struct {
	name string
	ps   []Product
	err  error
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Search(ctx context.Context, q Query) ([]Product, error) {
	return s.ps, s.err
}

func TestAggregateSkipsFailingProvider(t *testing.T) {
	good := &stubProvider{name: "good", ps: []Product{{SourceID: "1", CommissionRate: 0.2}}}
	bad := &stubProvider{name: "bad", err: errors.New("boom")}
	ps, errs := Aggregate(context.Background(),
		[]Provider{bad, good}, Query{MinCommission: 0.1})
	if len(ps) != 1 || len(errs) != 1 {
		t.Fatalf("want 1 product + 1 error, got %d + %d", len(ps), len(errs))
	}
}

func TestAggregateDedupes(t *testing.T) {
	a := &stubProvider{name: "s", ps: []Product{{SourceID: "1", CommissionRate: 0.2}}}
	b := &stubProvider{name: "s", ps: []Product{{SourceID: "1", CommissionRate: 0.2}}}
	ps, _ := Aggregate(context.Background(), []Provider{a, b}, Query{})
	if len(ps) != 1 {
		t.Fatalf("dedupe failed: %d", len(ps))
	}
}

func TestTikTokShopProviderFailClosed(t *testing.T) {
	p := &TikTokShopProvider{}
	if p.Configured() {
		t.Fatal("unconfigured provider must not report configured")
	}
	_, err := p.Search(context.Background(), Query{Theme: "thời trang"})
	if err == nil {
		t.Fatal("expected fail-closed error")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, err := s.Save(Product{
		Source: "tiktok_shop", SourceID: "p1", Title: "Túi kem",
		ImageURLs: []string{"https://x/y.jpg"}, Price: 299000,
		CommissionRate: 0.22, ShopName: "Shop A", Rating: 4.8,
		SoldCount: 1200, Theme: "thời trang nữ",
	})
	if err != nil || id == 0 {
		t.Fatalf("save: %v id=%d", err, id)
	}
	// save again = upsert, same id
	id2, err := s.Save(Product{Source: "tiktok_shop", SourceID: "p1",
		Title: "Túi kem", CommissionRate: 0.25, Theme: "thời trang nữ"})
	if err != nil || id2 != id {
		t.Fatalf("upsert: %v id2=%d want %d", err, id2, id)
	}
	top, err := s.TopByTheme("thời trang nữ", 0.10, 7, 10)
	if err != nil || len(top) != 1 || top[0].CommissionRate != 0.25 {
		t.Fatalf("top: %v %+v", err, top)
	}
	if err := s.RecordUse(id, 7, "job1"); err != nil {
		t.Fatal(err)
	}
	top, err = s.TopByTheme("thời trang nữ", 0.10, 7, 10)
	if err != nil || len(top) != 0 {
		t.Fatalf("used product should be excluded: %v %+v", err, top)
	}
	// other accounts still see it
	top, err = s.TopByTheme("thời trang nữ", 0.10, 8, 10)
	if err != nil || len(top) != 1 {
		t.Fatalf("other account should still see product: %v", len(top))
	}
}

func TestParseShopProducts(t *testing.T) {
	data := map[string]any{
		"products": []any{
			map[string]any{
				"product_id": "42", "product_name": "Son lì",
				"price": 159000.0, "commission_rate": 18.0,
				"shop_name": "BeautyVN", "rating": 4.9,
				"sold_count": 5300.0, "main_image": "https://x/son.jpg",
			},
			map[string]any{"product_id": "43"}, // no title -> skipped
		},
	}
	ps := parseShopProducts(data, "mỹ phẩm")
	if len(ps) != 1 {
		t.Fatalf("want 1 parsed, got %d", len(ps))
	}
	p := ps[0]
	if p.CommissionRate != 0.18 {
		t.Fatalf("commission percent->rate: %v", p.CommissionRate)
	}
	if len(p.ImageURLs) != 1 || p.SoldCount != 5300 {
		t.Fatalf("bad fields: %+v", p)
	}
}

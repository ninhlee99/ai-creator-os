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

func TestAggregateEmptyProviders(t *testing.T) {
	ps, errs := Aggregate(context.Background(), nil, Query{Theme: "thời trang"})
	if len(ps) != 0 || len(errs) != 0 {
		t.Fatalf("empty providers should yield nothing, got %d products %d errs", len(ps), len(errs))
	}
	// Đợt H1: nguồn sản phẩm duy nhất là Accesstrade → products.Store;
	// providers là fallback (hiện để trống).
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

// TestShelfLifecycle — trạng thái kệ sống cùng kho: thêm vào kệ → hiện
// trong Shelf; gỡ khỏi kệ → không còn; hạ cấp dữ liệu cũ (DB không có cột
// kệ) được migration tự thêm cột khi mở.
func TestShelfLifecycle(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	id, err := s.Save(Product{Source: "manual", SourceID: "P1", Title: "Tai nghe",
		Theme: "cong-nghe", CommissionRate: 0.2})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if shelf, _ := s.Shelf(50); len(shelf) != 0 {
		t.Fatalf("shelf = %d, want 0 before SetShelf", len(shelf))
	}
	if err := s.SetShelf(id, "shelf", 0); err != nil {
		t.Fatalf("SetShelf: %v", err)
	}
	shelf, err := s.Shelf(50)
	if err != nil || len(shelf) != 1 || shelf[0].ShelfStatus != "shelf" {
		t.Fatalf("Shelf = (%v, %v), want 1 with status shelf", shelf, err)
	}
	if err := s.SetShelf(id, "", 0); err != nil {
		t.Fatalf("SetShelf off: %v", err)
	}
	if shelf, _ := s.Shelf(50); len(shelf) != 0 {
		t.Fatalf("shelf = %d after removal, want 0", len(shelf))
	}

	// Old DB without shelf columns: opening adds them (idempotent).
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	raw, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := raw.db.Exec(`ALTER TABLE products DROP COLUMN shelf_status`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if _, err := raw.db.Exec(`ALTER TABLE products DROP COLUMN shelf_score`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	raw.Close()
	reopened, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("reopen legacy db: %v", err)
	}
	defer reopened.Close()
	id2, err := reopened.Save(Product{Source: "manual", SourceID: "P2", Title: "Loa"})
	if err != nil {
		t.Fatalf("Save after migration: %v", err)
	}
	if err := reopened.SetShelf(id2, "shelf", 0); err != nil {
		t.Fatalf("SetShelf after migration: %v", err)
	}
	if shelf, _ := reopened.Shelf(50); len(shelf) != 1 {
		t.Fatalf("shelf = %d after migration, want 1", len(shelf))
	}
}

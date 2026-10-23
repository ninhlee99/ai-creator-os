package products

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecordBioEventPrunesOldEventsOncePerUTCDate(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	old := at.AddDate(0, 0, -401).Format(time.RFC3339)
	if _, err := store.db.Exec(`INSERT INTO bio_events(account_id,product_id,event_type,local_day,created_at)
		VALUES(1,0,'profile_view','2025-08-01',?)`, old); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBioEvent(1, 0, "profile_view", at); err != nil {
		t.Fatal(err)
	}
	var oldCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM bio_events WHERE created_at<?`, at.AddDate(0, 0, -400).Format(time.RFC3339)).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("old events after daily prune = %d, want 0", oldCount)
	}

	if _, err := store.db.Exec(`INSERT INTO bio_events(account_id,product_id,event_type,local_day,created_at)
		VALUES(1,0,'profile_view','2025-08-01',?)`, old); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordBioEvent(1, 0, "profile_view", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM bio_events WHERE created_at=?`, old).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 1 {
		t.Fatalf("same-day event request repeated retention scan: stale rows=%d, want 1", oldCount)
	}
}

func TestRecordBioEventOnceSurvivesRestartAndExpires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products.db")
	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	key := strings.Repeat("a", 64)
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := store.RecordBioEventOnce(key, 1, 0, "profile_view", at, 30*time.Minute)
	if err != nil || !recorded {
		t.Fatalf("first record = %v, err=%v; want recorded", recorded, err)
	}
	recorded, err = store.RecordBioEventOnce(key, 1, 0, "profile_view", at.Add(29*time.Minute), 30*time.Minute)
	if err != nil || recorded {
		t.Fatalf("duplicate before expiry = %v, err=%v; want deduped", recorded, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recorded, err = store.RecordBioEventOnce(key, 1, 0, "profile_view", at.Add(29*time.Minute), 30*time.Minute)
	if err != nil || recorded {
		t.Fatalf("duplicate after reopen = %v, err=%v; want deduped", recorded, err)
	}
	removed, err := store.CleanupExpiredBioEventDedupe(at.Add(31 * time.Minute))
	if err != nil || removed != 1 {
		t.Fatalf("expired dedupe cleanup = %d, err=%v; want 1", removed, err)
	}
	recorded, err = store.RecordBioEventOnce(key, 1, 0, "profile_view", at.Add(31*time.Minute), 30*time.Minute)
	if err != nil || !recorded {
		t.Fatalf("record after expiry = %v, err=%v; want new event", recorded, err)
	}
	daily, err := store.BioDailyMetrics(1, "2026-10-23")
	if err != nil || len(daily) != 1 || daily[0].ProfileViews != 2 {
		t.Fatalf("daily metrics after dedupe expiry = %+v, err=%v; want 2 views", daily, err)
	}
}

func TestRecordBioEventOnceConcurrentRequestsCountOnce(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const concurrent = 24
	key := strings.Repeat("b", 64)
	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	results := make(chan bool, concurrent)
	errors := make(chan error, concurrent)
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorded, err := store.RecordBioEventOnce(key, 1, 0, "profile_view", at, 30*time.Minute)
			results <- recorded
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	accepted := 0
	for recorded := range results {
		if recorded {
			accepted++
		}
	}
	for err := range errors {
		if err != nil {
			t.Errorf("RecordBioEventOnce: %v", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d concurrent duplicate events, want 1", accepted)
	}
	daily, err := store.BioDailyMetrics(1, "2026-10-23")
	if err != nil || len(daily) != 1 || daily[0].ProfileViews != 1 {
		t.Fatalf("daily metrics = %+v, err=%v; want one view", daily, err)
	}
}

func TestBioListingStoresProductDetailsPerChannel(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetBioProfile(1, true, "", true); err != nil {
		t.Fatal(err)
	}
	productID, err := store.Save(Product{Source: "manual", SourceID: "cardigan", Title: "Áo cardigan"})
	if err != nil {
		t.Fatal(err)
	}
	details := "Chất liệu: len cotton\nKích thước: S, M, L"
	if err := store.SetBioListingWithProductDetails(1, productID, true, 0, nil, "", 0, "", "", details); err != nil {
		t.Fatal(err)
	}
	listing, err := store.BioListing(1, productID, true)
	if err != nil || listing.ProductDetails != details {
		t.Fatalf("product details = %q, err=%v", listing.ProductDetails, err)
	}
	if err := store.SetBioListingWithImages(1, productID, true, 1, nil, "", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	listing, err = store.BioListing(1, productID, true)
	if err != nil || listing.ProductDetails != details {
		t.Fatalf("image-only update cleared product details: %q, err=%v", listing.ProductDetails, err)
	}
	if err := store.SetBioListingWithProductDetails(1, productID, true, 0, nil, "", 0, "", "", strings.Repeat("x", 1501)); err == nil {
		t.Fatal("oversized product details were accepted")
	}
}

func TestBioMetricsAggregateTryOnEventsByDayAndProduct(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetBioProfile(1, true, "", true); err != nil {
		t.Fatal(err)
	}
	productID, err := store.Save(Product{Source: "manual", SourceID: "metrics-shirt", Title: "Áo thử"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioListing(1, productID, true, 0, "SAVE", 10000, "", ""); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 10, 22, 18, 30, 0, 0, time.UTC) // 2026-10-23 in Vietnam.
	for _, event := range []struct {
		name      string
		productID int64
		count     int
	}{
		{name: "profile_view", productID: 0, count: 1},
		{name: "product_view", productID: productID, count: 2},
		{name: "outbound_click", productID: productID, count: 3},
		{name: "voucher_copy", productID: productID, count: 1},
		{name: "tryon_start", productID: productID, count: 2},
		{name: "tryon_success", productID: productID, count: 1},
		{name: "tryon_failure", productID: productID, count: 1},
		{name: "tryon_quota_denied", productID: productID, count: 4},
	} {
		for i := 0; i < event.count; i++ {
			if err := store.RecordBioEvent(1, event.productID, event.name, at.Add(time.Duration(i)*time.Second)); err != nil {
				t.Fatalf("record %s: %v", event.name, err)
			}
		}
	}

	daily, err := store.BioDailyMetrics(1, "2026-10-23")
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 1 {
		t.Fatalf("daily metrics rows = %d, want 1", len(daily))
	}
	wantDaily := (BioDailyMetric{
		Day: "2026-10-23", ProfileViews: 1, ProductViews: 2, OutboundClicks: 3,
		VoucherCopies: 1, TryOnStarts: 2, TryOnSuccesses: 1, TryOnFailures: 1, QuotaDenials: 4,
	})
	if daily[0] != wantDaily {
		t.Fatalf("daily metrics = %+v, want %+v", daily[0], wantDaily)
	}

	byProduct, err := store.BioProductMetrics(1, "2026-10-23", 20)
	if err != nil {
		t.Fatal(err)
	}
	wantProduct := (BioProductMetric{
		ProductID: productID, Title: "Áo thử", Views: 2, OutboundClicks: 3,
		VoucherCopies: 1, TryOnStarts: 2, TryOnSuccesses: 1, TryOnFailures: 1, QuotaDenials: 4,
	})
	if len(byProduct) != 1 || byProduct[0] != wantProduct {
		t.Fatalf("product metrics = %+v, want [%+v]", byProduct, wantProduct)
	}
}

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestHandleProductsImportLink(t *testing.T) {
	mockAffiliateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Bộ Dụng Cụ Nấu Ăn Silicon Chịu Nhiệt Cao Cấp | Shopee Việt Nam">
	<meta property="og:image" content="https://example.com/bep-silicon.jpg">
	<meta property="product:price:amount" content="289000">
	<meta property="og:site_name" content="Shopee">
</head>
<body>
	<div>Giá: 289.000₫</div>
</body>
</html>`))
	}))
	defer mockAffiliateServer.Close()

	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	form := url.Values{
		"url": {mockAffiliateServer.URL + "/product/i.123.456789"},
	}
	rec := postForm(t, s, "/products/import-link", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /products/import-link code = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("expected success redirect with ok=..., got %q", loc)
	}

	shelf, err := s.Products.Shelf(10)
	if err != nil {
		t.Fatalf("Products.Shelf failed: %v", err)
	}
	if len(shelf) == 0 {
		t.Fatal("expected at least 1 shelf product")
	}

	var found *products.Product
	for _, p := range shelf {
		if strings.Contains(p.Title, "Bộ Dụng Cụ Nấu Ăn Silicon") {
			found = &p
			break
		}
	}
	if found == nil {
		t.Fatalf("imported product not found in shelf, got shelf: %+v", shelf)
	}
	if found.Price != 289000 {
		t.Errorf("Price mismatch: got %f, want 289000", found.Price)
	}
	if found.Theme != "home" {
		t.Errorf("Theme mismatch: got %q, want home", found.Theme)
	}
}

func TestHandleProductsCreateVideo(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st

	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	prodID, err := s.Products.SaveImportedProduct(products.ExtractedProduct{
		PlatformPID:    "TEST-PID-99",
		Title:          "Áo Thun Unisex Cotton 100%",
		Price:          150000,
		Currency:       "VND",
		CommissionRate: 0.12,
		Theme:          "fashion",
		Category:       "Thời trang",
		ProductURL:     "https://example.com/ao-thun",
	})
	if err != nil {
		t.Fatalf("SaveImportedProduct failed: %v", err)
	}

	form := url.Values{
		"product_id": {strconv.FormatInt(prodID, 10)},
	}
	rec := postForm(t, s, "/products/create-video", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /products/create-video code = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/studio/jobs") {
		t.Fatalf("expected redirect to /studio/jobs, got %q", loc)
	}

	// Verify job was created in Studio
	jobs := s.Studio.ListJobs(10)
	foundJob := false
	for _, j := range jobs {
		if strings.Contains(j.Title, "Áo Thun Unisex Cotton") {
			foundJob = true
			break
		}
	}
	if !foundJob {
		t.Errorf("created video job not found in studio jobs list: %+v", jobs)
	}
}

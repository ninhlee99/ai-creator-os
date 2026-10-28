package products

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestParseProductHTML(t *testing.T) {
	htmlShopee := `<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Tai nghe Bluetooth True Wireless X10 chống ồn | Shopee Việt Nam">
	<meta property="og:image" content="https://down-vn.img.susercontent.com/file/vn-11134207-7r98o-xyz123.jpg">
	<meta property="product:price:amount" content="350000">
	<meta property="og:site_name" content="Shopee">
</head>
<body>
	<h1>Tai nghe Bluetooth X10</h1>
</body>
</html>`

	rawURL := "https://s.shopee.vn/abcdef123"
	finalURL := "https://shopee.vn/product/12345/67890?sp_atk=xyz"

	prod := ParseProductHTML(rawURL, finalURL, htmlShopee)
	if prod == nil {
		t.Fatal("ParseProductHTML returned nil")
	}
	if prod.Title != "Tai nghe Bluetooth True Wireless X10 chống ồn" {
		t.Errorf("title mismatch: got %q", prod.Title)
	}
	if len(prod.ImageURLs) != 1 || prod.ImageURLs[0] != "https://down-vn.img.susercontent.com/file/vn-11134207-7r98o-xyz123.jpg" {
		t.Errorf("image mismatch: got %v", prod.ImageURLs)
	}
	if prod.Price != 350000 {
		t.Errorf("price mismatch: got %f, want 350000", prod.Price)
	}
	if prod.Theme != "tech" {
		t.Errorf("theme mismatch: got %q, want tech", prod.Theme)
	}
	if prod.PlatformPID != "SP-67890" {
		t.Errorf("PID mismatch: got %q, want SP-67890", prod.PlatformPID)
	}
}

func TestExtractFromAffiliateLinkServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Váy hoa nhí dáng xòe vintage thanh lịch - TikTok Shop">
	<meta property="og:image" content="https://p16-oec-va.ibyteimg.com/tos-maliva-i-o3syd03w52-us/image.jpeg">
	<meta property="og:price:amount" content="220000">
</head>
<body>
	<div>220.000₫</div>
</body>
</html>`))
	}))
	defer server.Close()

	ctx := context.Background()
	prod, err := ExtractFromAffiliateLink(ctx, server.URL+"/product/99887766")
	if err != nil {
		t.Fatalf("ExtractFromAffiliateLink failed: %v", err)
	}
	if prod.Title != "Váy hoa nhí dáng xòe vintage thanh lịch" {
		t.Errorf("Title: %q", prod.Title)
	}
	if prod.Theme != "fashion" {
		t.Errorf("Theme: %q, want fashion", prod.Theme)
	}
	if prod.Price != 220000 {
		t.Errorf("Price: %f, want 220000", prod.Price)
	}
	if prod.PlatformPID != "TTS-99887766" {
		t.Errorf("PID: %q, want TTS-99887766", prod.PlatformPID)
	}
}

func TestSaveImportedProduct(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "products.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	p := ExtractedProduct{
		PlatformPID:    "SP-123456",
		Title:          "Son kem lì mịn môi lâu trôi",
		Price:          189000,
		Currency:       "VND",
		CommissionRate: 0.15,
		ImageURLs:      []string{"https://example.com/son.jpg"},
		Theme:          "beauty",
		Category:       "Làm đẹp",
		ProductURL:     "https://s.shopee.vn/son123",
		ProductPageURL: "https://shopee.vn/son-kem-li-i.111.123456",
	}

	id, err := store.SaveImportedProduct(p)
	if err != nil {
		t.Fatalf("SaveImportedProduct failed: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	shelf, err := store.Shelf(10)
	if err != nil {
		t.Fatalf("store.Shelf failed: %v", err)
	}
	if len(shelf) != 1 {
		t.Fatalf("expected 1 shelf product, got %d", len(shelf))
	}
	if shelf[0].Title != p.Title {
		t.Errorf("shelf product title mismatch: %q", shelf[0].Title)
	}
	if shelf[0].ReviewStatus != ReviewApproved {
		t.Errorf("shelf product review status: %q, want approved", shelf[0].ReviewStatus)
	}
}

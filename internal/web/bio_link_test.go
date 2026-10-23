package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func TestBioProductViewShowsVoucherWithoutCode(t *testing.T) {
	listing := products.BioListing{
		Product:      products.Product{ID: 1, Title: "Áo khoác", Price: 250000},
		VoucherValue: 30000,
		VoucherTerms: "Áp dụng tại shop",
	}
	view := (&Server{}).bioProductView(listing, "creator", time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC))
	if !view.VoucherAvailable || view.VoucherCode != "" || view.VoucherValue != 30000 {
		t.Fatalf("value-only voucher should display without copy action: %+v", view)
	}
}

func TestBioProductViewShowsChannelSpecificProductDetails(t *testing.T) {
	details := "Chất liệu: len cotton\nKích thước: S, M, L"
	listing := products.BioListing{Product: products.Product{ID: 12, Title: "Áo"}, ProductDetails: details}
	view := (&Server{}).bioProductView(listing, "creator", time.Now())
	if view.ProductDetails != details {
		t.Fatalf("product details = %q, want %q", view.ProductDetails, details)
	}
}

func TestRoutesExposePublishedBioAndBlockDashboardRemotely(t *testing.T) {
	s := newTestServer(t)
	productsStore, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = productsStore.Close() })
	s.Products = productsStore

	accountID, err := s.Ledger.AddAccount("bio_route_test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := productsStore.SetBioProfile(accountID, true, "Kênh thời trang", true); err != nil {
		t.Fatal(err)
	}
	productID, err := productsStore.Save(products.Product{
		Source: "manual", SourceID: "bio-route-test", Title: "Áo len kiểm thử",
		Price: 250000, Currency: "VND", ProductURL: "https://shop.example/item",
		ImageURLs: []string{"https://images.example/item.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := productsStore.SetBioListingWithProductDetails(accountID, productID, true, 0, nil,
		"SAVE", 30000, "", "Áp dụng cho đơn từ 200.000đ", "Chất liệu: len cotton"); err != nil {
		t.Fatal(err)
	}
	s.RemoteNeedsPassword = true
	handler := s.Routes()
	request := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = "203.0.113.8:9000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	profile := request(http.MethodGet, "/bio/bio_route_test")
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), "Áo len kiểm thử") ||
		!strings.Contains(profile.Body.String(), formatBioMoney(250000, "VND")) {
		t.Fatalf("published Bio route = %d, body contains product=%v price=%v", profile.Code,
			strings.Contains(profile.Body.String(), "Áo len kiểm thử"),
			strings.Contains(profile.Body.String(), formatBioMoney(250000, "VND")))
	}
	product := request(http.MethodGet, fmt.Sprintf("/bio/bio_route_test/product/%d", productID))
	productHTML := product.Body.String()
	if product.Code != http.StatusOK || !strings.Contains(productHTML, "Xem giá và mua tại shop") ||
		!strings.Contains(productHTML, "SAVE") || !strings.Contains(productHTML, "Chất liệu: len cotton") ||
		!strings.Contains(productHTML, "bio_route_test") || !strings.Contains(productHTML, "voucher-copy") ||
		!strings.Contains(productHTML, "return response.json();") {
		t.Fatalf("published product route = %d, body has CTA=%v voucher=%v details=%v copy-url=%v json=%v", product.Code,
			strings.Contains(productHTML, "Xem giá và mua tại shop"),
			strings.Contains(productHTML, "SAVE"),
			strings.Contains(productHTML, "Chất liệu: len cotton"),
			strings.Contains(productHTML, "bio_route_test") && strings.Contains(productHTML, "voucher-copy"),
			strings.Contains(productHTML, "response.json()"))
	}
	for _, path := range []string{"/", "/products", "/bio-admin", "/bio-admin/analytics", "/settings/system"} {
		if response := request(http.MethodGet, path); response.Code != http.StatusForbidden {
			t.Errorf("remote private route %s = %d, want 403", path, response.Code)
		}
	}
}

func TestBioVoucherCopyValidatesCurrentVoucher(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.Products = store

	accountID, err := s.Ledger.AddAccount("bio_voucher_validation", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioProfile(accountID, true, "", true); err != nil {
		t.Fatal(err)
	}
	productID, err := store.Save(products.Product{Source: "manual", SourceID: "voucher-validation", Title: "Áo voucher",
		ProductURL: "https://shop.example/voucher-validation", ImageURLs: []string{"https://images.example/voucher.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioListing(accountID, productID, true, 0, "CURRENT", 10000, "2026-12-31", ""); err != nil {
		t.Fatal(err)
	}
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/bio/bio_voucher_validation/products/%d/voucher-copy", productID), nil)
		r.RemoteAddr = "127.0.0.1:4321"
		response := httptest.NewRecorder()
		s.Routes().ServeHTTP(response, r)
		return response
	}
	active := request()
	if active.Code != http.StatusOK || !strings.Contains(active.Body.String(), `"code":"CURRENT"`) {
		t.Fatalf("active voucher response = %d %s", active.Code, active.Body.String())
	}
	if err := store.SetBioListing(accountID, productID, true, 0, "EXPIRED", 10000, "2020-01-01", ""); err != nil {
		t.Fatal(err)
	}
	expired := request()
	if expired.Code != http.StatusNotFound || strings.Contains(expired.Body.String(), "EXPIRED") {
		t.Fatalf("expired voucher response = %d %s, want 404 without code", expired.Code, expired.Body.String())
	}
}

func TestBioAdminDoesNotRenderUnknownVNDPriceAsZero(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.Products = store

	accountID, err := s.Ledger.AddAccount("bio_unknown_price", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(products.Product{Source: "manual", SourceID: "unknown-price", Title: "Áo chưa có giá", Currency: "VND"}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bio-admin?account_id=%d", accountID), nil)
	response := httptest.NewRecorder()
	s.handleBioAdmin(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "Chưa có giá được xác nhận") || strings.Contains(body, "0 ₫") {
		t.Fatalf("admin missing-price display status=%d unknown-label=%v zero-price=%v", response.Code,
			strings.Contains(body, "Chưa có giá được xác nhận"), strings.Contains(body, "0 ₫"))
	}
}

func TestActiveBioVoucherExpiresAtVietnamEndOfDay(t *testing.T) {
	listing := products.BioListing{VoucherCode: "SAVE", VoucherEndsAt: "2026-10-23"}
	beforeExpiry := time.Date(2026, 10, 23, 16, 59, 59, 0, time.UTC)
	afterExpiry := time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC)
	if code, _, _, _ := activeBioVoucher(listing, beforeExpiry); code != "SAVE" {
		t.Fatalf("voucher should be active before Vietnam day ends, got %q", code)
	}
	if code, _, _, _ := activeBioVoucher(listing, afterExpiry); code != "" {
		t.Fatalf("voucher should expire at Vietnam end of day, got %q", code)
	}
}

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

func TestBioAnalyticsRendersTryOnMetrics(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.Products = store

	accountID, err := s.Ledger.AddAccount("bio_analytics_test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioProfile(accountID, true, "", true); err != nil {
		t.Fatal(err)
	}
	productID, err := store.Save(products.Product{Source: "manual", SourceID: "analytics-shirt", Title: "Áo analytics"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioListing(accountID, productID, true, 0, "", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	for _, event := range []struct {
		name  string
		count int
	}{{"tryon_start", 2}, {"tryon_success", 1}, {"tryon_failure", 1}, {"tryon_quota_denied", 3}} {
		for i := 0; i < event.count; i++ {
			if err := store.RecordBioEvent(accountID, productID, event.name, at.Add(time.Duration(i)*time.Second)); err != nil {
				t.Fatalf("record %s: %v", event.name, err)
			}
		}
	}

	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bio-admin/analytics?account_id=%d", accountID), nil)
	response := httptest.NewRecorder()
	s.handleBioAnalytics(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "Hiệu quả thử đồ ảo") ||
		!strings.Contains(body, "2 · 1 · 1 · 3") || !strings.Contains(body, "Lượt bị chặn quota") {
		t.Fatalf("analytics page status=%d try-on chart present=%v totals present=%v", response.Code,
			strings.Contains(body, "Hiệu quả thử đồ ảo"), strings.Contains(body, "2 · 1 · 1 · 3"))
	}
}

func TestBioAnalyticsDoesNotPresentLockedTryOnAsZeroActivity(t *testing.T) {
	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.Products = store

	accountID, err := s.Ledger.AddAccount("bio_locked_analytics", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioProfile(accountID, true, "", true); err != nil {
		t.Fatal(err)
	}
	productID, err := store.Save(products.Product{Source: "manual", SourceID: "locked-analytics-shirt", Title: "Áo đang khóa"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBioListing(accountID, productID, true, 0, "", 0, "", ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bio-admin/analytics?account_id=%d", accountID), nil)
	response := httptest.NewRecorder()
	s.handleBioAnalytics(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "Tính năng đang khóa") ||
		strings.Contains(body, "Lượt bắt đầu thử đồ") || strings.Contains(body, "<th>Chặn quota</th>") ||
		strings.Contains(body, `class="bio-bar `) {
		t.Fatalf("locked try-on analytics status=%d locked notice=%v zero KPI hidden=%v quota column hidden=%v", response.Code,
			strings.Contains(body, "Tính năng đang khóa"), !strings.Contains(body, "Lượt bắt đầu thử đồ"), !strings.Contains(body, "<th>Chặn quota</th>"))
	}
}

package web

// Accesstrade Đợt C: hunter + order sync + công tắc tick — test fail-closed
// (chưa key) và render trang Affiliate có card mới.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// postJSON đã có trong accesstrade_test.go — dùng chung.

func TestATHuntNoKey(t *testing.T) {
	s := newTestServer(t) // chưa nhập key
	rec := postJSON(t, s, "/at/hunt", "{}")
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("thiếu key phải 412, được %d", rec.Code)
	}
	var d map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("response phải là JSON: %v", err)
	}
	if _, ok := d["error"]; !ok {
		t.Errorf("thiếu field error: %v", d)
	}
}

func TestATOrderSyncNoKey(t *testing.T) {
	s := newTestServer(t)
	rec := postJSON(t, s, "/at/orders/sync", "{}")
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("thiếu key phải 412, được %d", rec.Code)
	}
}

func TestATSettingsSave(t *testing.T) {
	s := newTestServer(t)
	rec := postJSON(t, s, "/at/settings",
		`{"hunter_on":false,"hunter_videos":5,"order_sync_on":true,"campaign_on":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lưu settings phải 200, được %d", rec.Code)
	}
	if v := s.atSettingOn("at.hunter_enabled", true); v {
		t.Errorf("hunter_on=false phải lưu thành tắt")
	}
	if v := s.atSettingInt("at.hunter_videos", 3); v != 5 {
		t.Errorf("hunter_videos phải = 5, được %d", v)
	}
	// JSON sai → 400.
	rec = postJSON(t, s, "/at/settings", "{invalid")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("JSON sai phải 400, được %d", rec.Code)
	}
	// hunter_videos vượt trần → cắt về 10.
	rec = postJSON(t, s, "/at/settings", `{"hunter_videos":99}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if v := s.atSettingInt("at.hunter_videos", 3); v != 10 {
		t.Errorf("hunter_videos phải cắt về 10, được %d", v)
	}
}

func TestProductsPageRendersNewCards(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products phải 200, được %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Săn sản phẩm", "Tự động Accesstrade", "Đối soát hoa hồng",
		"/at/hunt", "/at/orders/sync", "/at/settings",
		"Chờ duyệt", // nhãn trung thực cho pending
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /products thiếu %q", want)
		}
	}
}

func TestATPageDataNoStore(t *testing.T) {
	s := newTestServer(t)
	s.AT = nil // kho chưa mở
	v := s.atPageData()
	if v.StoreOK || v.KeySet {
		t.Errorf("chưa có store/key phải StoreOK=false KeySet=false")
	}
	if v.Hunted != nil || v.Orders != nil {
		t.Errorf("chưa có store phải không có dữ liệu: %+v", v)
	}
}

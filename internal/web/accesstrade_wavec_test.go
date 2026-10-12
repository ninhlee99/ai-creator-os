package web

// Accesstrade Đợt C: hunter + order sync + công tắc tick — test fail-closed
// (chưa key) và render trang Affiliate có card mới.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
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

// Đợt AK: công tắc tick AT gộp về form Cài đặt · Accesstrade (bỏ JSON
// /at/settings ở trang Affiliate). Checkbox trống = tắt; số video bị cắt.
func TestATSettingsSave(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/settings/accesstrade/automation", url.Values{
		"toggles": {"1"}, "ordersync_on": {"1"}, "hunter_videos": {"5"},
		"hunter_hours": {"24"}, "ordersync_mins": {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("lưu phải redirect, được %d", rec.Code)
	}
	if s.atSettingOn("at.hunter_enabled", true) {
		t.Errorf("hunter_on trống phải lưu thành tắt")
	}
	if !s.atSettingOn(automation.KeyATOrderSyncEnabled, false) {
		t.Errorf("ordersync_on=1 phải bật")
	}
	if v := s.atSettingInt("at.hunter_videos", 3); v != 5 {
		t.Errorf("hunter_videos phải = 5, được %d", v)
	}
	postForm(t, s, "/settings/accesstrade/automation", url.Values{"toggles": {"1"}, "ordersync_on": {"1"}, "hunter_videos": {"99"}})
	if v := s.atSettingInt("at.hunter_videos", 3); v != 10 {
		t.Errorf("hunter_videos phải cắt về 10, được %d", v)
	}
	// Không có cờ toggles (chỉ gửi chu kỳ) → công tắc giữ nguyên.
	postForm(t, s, "/settings/accesstrade/automation", url.Values{"hunter_hours": {"12"}})
	if !s.atSettingOn(automation.KeyATOrderSyncEnabled, false) {
		t.Errorf("form chỉ có chu kỳ không được tắt công tắc")
	}
	if body := get(t, s, "/settings/accesstrade").Body.String(); !strings.Contains(body, `name="hunter_on"`) {
		t.Errorf("Cài đặt · Accesstrade thiếu công tắc săn sản phẩm")
	}
}

func TestProductsPageRendersNewCards(t *testing.T) {
	s := newTestServer(t)
	// Chưa có key (Đợt AK): đúng 1 thẻ kết nối, không lặp 3 khối rỗng.
	body := get(t, s, "/products").Body.String()
	if !strings.Contains(body, "Nhập mã truy cập") || strings.Contains(body, "Đối soát hoa hồng") {
		t.Errorf("chưa có key phải chỉ hiện thẻ kết nối Accesstrade")
	}
	// Có key: đủ các khối săn / đối soát.
	if err := s.Ledger.SetSetting(accesstrade.KeySetting, "test-key"); err != nil {
		t.Fatal(err)
	}
	rec := get(t, s, "/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products phải 200, được %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{
		"Săn sản phẩm", "Đối soát hoa hồng",
		"/at/hunt", "/at/orders/sync",
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

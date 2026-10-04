package web

// Test Accesstrade UI (Đợt B): tab Cài đặt, card Affiliate, fail-closed
// khi chưa có key/store.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
)

func postJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestSettingsAccesstradePage(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/settings/accesstrade")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/accesstrade = %d, muốn 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Cài đặt · Accesstrade", "Chưa có key", "access_key", `href="/settings/accesstrade"`} {
		if !strings.Contains(body, want) {
			t.Errorf("trang thiếu %q", want)
		}
	}
	if strings.Contains(body, "Kiểm tra kết nối") {
		t.Error("chưa có key thì không được hiện nút kiểm tra kết nối")
	}
}

func TestATFailClosedWithoutKey(t *testing.T) {
	s := newTestServer(t) // chưa có key
	// Nối store thật để test đúng nhánh "thiếu key" (thay vì "thiếu store").
	atStore, err := accesstrade.NewStore(filepath.Join(t.TempDir(), "accesstrade.db"))
	if err != nil {
		t.Fatalf("accesstrade.NewStore: %v", err)
	}
	t.Cleanup(func() { atStore.Close() })
	s.AT = atStore

	// Sync campaign → redirect về /products kèm ?err=
	rec := postForm(t, s, "/at/campaigns/sync", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("sync không key = %d, muốn 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/products?err=") {
		t.Errorf("redirect sai: %q", loc)
	}

	// Tạo link → 412 JSON, không gọi API
	rec = postJSON(t, s, "/at/links/create", `{"product_url":"https://x.vn","campaign_id":"c1"}`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("create link không key = %d, muốn 412", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Cài đặt · Accesstrade") {
		t.Errorf("lỗi phải trỏ về trang cài đặt: %s", rec.Body.String())
	}

	// Test kết nối → 412
	rec = postForm(t, s, "/settings/accesstrade/test", url.Values{})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("test không key = %d, muốn 412", rec.Code)
	}
}

func TestATKeySaveAndMasked(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/settings/accesstrade/key", url.Values{"access_key": {"SECRETKEY9999"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("lưu key = %d, muốn 303", rec.Code)
	}
	body := get(t, s, "/settings/accesstrade").Body.String()
	if !strings.Contains(body, "Đã kết nối") {
		t.Error("sau khi lưu key phải hiện Đã kết nối")
	}
	if !strings.Contains(body, "••••9999") {
		t.Error("key phải hiện dạng mask ••••9999")
	}
	if strings.Contains(body, "SECRETKEY9999") {
		t.Error("key thô bị lộ trên trang")
	}

	// Lưu key trống → từ chối, không ghi đè
	rec = postForm(t, s, "/settings/accesstrade/key", url.Values{"access_key": {""}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Errorf("key trống phải báo lỗi: %q", loc)
	}
}

func TestProductsATCard(t *testing.T) {
	s := newTestServer(t)
	body := get(t, s, "/products").Body.String()
	for _, want := range []string{"Chiến dịch Accesstrade", "Chưa có key", "/settings/accesstrade"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang Affiliate thiếu %q", want)
		}
	}
}

// Đợt G: chu kỳ tick AT có UI ở tab Cài đặt · Accesstrade (interval),
// công tắc on/off ở trang Affiliate (POST /at/settings) — đúng nguyên tắc
// "UI cho mọi khả năng".
func TestATAutomationSave(t *testing.T) {
	s := newTestServer(t)
	// Trang hiện form chu kỳ.
	body := get(t, s, "/settings/accesstrade").Body.String()
	for _, want := range []string{"Tự động", "hunter_hours", "ordersync_mins"} {
		if !strings.Contains(body, want) {
			t.Fatalf("tab accesstrade thiếu %q", want)
		}
	}
	// Lưu + clamp.
	rec := postForm(t, s, "/settings/accesstrade/automation", url.Values{
		"hunter_hours":   {"48"},
		"ordersync_mins": {"15"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("lưu phải redirect, được %d", rec.Code)
	}
	if v := s.atSettingInt(automation.KeyATHunterIntervalHrs, 24); v != 48 {
		t.Errorf("hunter_hours phải 48, được %d", v)
	}
	if v := s.atSettingInt(automation.KeyATOrderSyncIntervalM, 30); v != 15 {
		t.Errorf("ordersync_mins phải 15, được %d", v)
	}
	// Giá trị vô lý → clamp.
	rec = postForm(t, s, "/settings/accesstrade/automation", url.Values{
		"hunter_hours":   {"9999"},
		"ordersync_mins": {"0"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("lưu phải redirect, được %d", rec.Code)
	}
	if v := s.atSettingInt(automation.KeyATHunterIntervalHrs, 24); v != 168 {
		t.Errorf("hunter_hours phải clamp 168, được %d", v)
	}
	if v := s.atSettingInt(automation.KeyATOrderSyncIntervalM, 30); v != 5 {
		t.Errorf("ordersync_mins phải clamp 5, được %d", v)
	}
}

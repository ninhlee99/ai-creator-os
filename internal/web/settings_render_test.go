package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Renders the real settings pages against the real handlers (no chains
// wired) to catch template field mismatches. PIVOT 2026-10-02: the avatar
// pages/cards are parked — the remaining pages must render without any
// avatar remnant.
func TestSettingsTemplateRenders(t *testing.T) {
	s := newTestServer(t)
	for _, path := range []string{
		"/settings/system", "/settings/providers",
		"/settings/safety",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		serveLocalTestRoute(s, rec, req)
		if rec.Code != 200 {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
	}
	// Retired local-model settings route must not resolve.
	recLocal := get(t, s, "/settings/model-local")
	if recLocal.Code != http.StatusNotFound {
		t.Fatalf("GET /settings/model-local = %d, want 404", recLocal.Code)
	}
	// Parked routes must 404, not 500. Đợt H2: /team đã định nghĩa lại
	// quanh 3 pipeline (không còn park) nên ra khỏi danh sách này.
	for _, path := range []string{"/settings/nhan-vat", "/schedule"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		serveLocalTestRoute(s, rec, req)
		if rec.Code == 200 {
			t.Errorf("GET %s = 200, want non-200 (parked)", path)
		}
	}
	body := get(t, s, "/settings/providers").Body.String()
	for _, want := range []string{"Thứ tự nhà cung cấp giọng đọc (TTS)", "Thứ tự nhà cung cấp viết nội dung (LLM)"} {
		if !strings.Contains(body, want) {
			t.Errorf("providers page missing %q", want)
		}
	}
	for _, gone := range []string{"Chuỗi provider Avatar", "provider-avatar"} {
		if strings.Contains(body, gone) {
			t.Errorf("providers page still has avatar remnant %q", gone)
		}
	}
	// Sidebar: no live schedule; /team đã định nghĩa lại ở Đợt H2
	// (Đội ngũ — 3 pipeline) nên được phép xuất hiện.
	body = get(t, s, "/").Body.String()
	for _, gone := range []string{"/schedule", "Lịch live", "Agent Team"} {
		if strings.Contains(body, gone) {
			t.Errorf("sidebar still references %q", gone)
		}
	}
	if !strings.Contains(body, "/growth") {
		t.Error("sidebar missing /growth")
	}
	for _, want := range []string{">Kênh<", ">Affiliate<"} {
		if !strings.Contains(body, want) {
			t.Errorf("sidebar missing relabel %q", want)
		}
	}
}

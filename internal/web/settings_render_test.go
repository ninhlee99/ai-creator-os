package web

import (
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
		"/settings/he-thong", "/settings/nha-cung-cap",
		"/settings/model-local", "/settings/an-toan",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
	}
	// Parked routes must 404, not 500. Đợt H2: /team đã định nghĩa lại
	// quanh 3 pipeline (không còn park) nên ra khỏi danh sách này.
	for _, path := range []string{"/settings/nhan-vat", "/schedule"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		if rec.Code == 200 {
			t.Errorf("GET %s = 200, want non-200 (parked)", path)
		}
	}
	body := get(t, s, "/settings/nha-cung-cap").Body.String()
	for _, want := range []string{"Chuỗi provider TTS", "Chuỗi provider LLM"} {
		if !strings.Contains(body, want) {
			t.Errorf("nha-cung-cap page missing %q", want)
		}
	}
	for _, gone := range []string{"Chuỗi provider Avatar", "provider-avatar"} {
		if strings.Contains(body, gone) {
			t.Errorf("nha-cung-cap page still has avatar remnant %q", gone)
		}
	}
	body = get(t, s, "/settings/model-local").Body.String()
	if !strings.Contains(body, "Khả năng AI") {
		t.Error("model-local page missing Khả năng AI table")
	}
	for _, gone := range []string{"avatar-sidecar-badge", "Hình đại diện chạy trên máy"} {
		if strings.Contains(body, gone) {
			t.Errorf("model-local page still has avatar remnant %q", gone)
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
	if !strings.Contains(body, "/team") {
		t.Error("sidebar missing /team (Đội ngũ — 3 pipeline, Đợt H2)")
	}
	for _, want := range []string{">Kênh<", ">Affiliate<"} {
		if !strings.Contains(body, want) {
			t.Errorf("sidebar missing relabel %q", want)
		}
	}
}

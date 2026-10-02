package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Renders the real settings pages against the real handlers (no chains
// wired) to catch template field mismatches (R2-W3: 5 sub-pages).
func TestSettingsTemplateRendersWithAvatar(t *testing.T) {
	s := newTestServer(t)
	for _, path := range []string{"/settings/nhan-vat", "/settings/nha-cung-cap", "/settings/model-local"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
	}
	body := get(t, s, "/settings/nhan-vat").Body.String()
	for _, want := range []string{
		"Nhân vật AI", "char-add-form",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("nhan-vat page missing %q", want)
		}
	}
	body = get(t, s, "/settings/nha-cung-cap").Body.String()
	for _, want := range []string{"Chuỗi provider Avatar", "local"} {
		if !strings.Contains(body, want) {
			t.Errorf("nha-cung-cap page missing %q", want)
		}
	}
	body = get(t, s, "/settings/model-local").Body.String()
	for _, want := range []string{
		"Hình đại diện chạy trên máy (Avatar)", "avatar-sidecar-badge",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("model-local page missing %q", want)
		}
	}
}

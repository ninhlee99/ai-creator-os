package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Renders the real settings page against the real handlers (no chains
// wired) to catch template field mismatches.
func TestSettingsTemplateRendersWithAvatar(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/settings", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET /settings = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Nhân vật AI", "Chuỗi provider Avatar", "Hình đại diện chạy trên máy (Avatar)",
		"avatar-sidecar-badge", "char-add-form", "local",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
}

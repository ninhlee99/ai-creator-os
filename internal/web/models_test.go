package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsPageRenders(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/models")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /models = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Thư Viện Nhân Vật &amp; Face Lock AI",
		"Face Swap Engine",
		"Ảnh Nhân Vật Đã Khóa",
		"Tải Lên &amp; Khóa Khuôn Mặt Nhân Vật Mới",
		"Danh Sách Khuôn Mặt Đã Khóa Trong Hệ Thống",
		"Nguyên Lý Hoạt Động Của Dây Chuyền Face Swap Cục Bộ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /models thiếu nội dung %q", want)
		}
	}
}

func TestAffiliateRedirect(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/affiliate", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /affiliate = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/studio" {
		t.Fatalf("GET /affiliate redirects to %q, want /studio", loc)
	}
}

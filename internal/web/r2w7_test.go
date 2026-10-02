package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestAPIGateDefaultOff — /api/* mặc định TẮT (404); bật qua UI → 200;
// tắt lại → 404 (nghiệm thu R2-W7, R2-09).
func TestAPIGateDefaultOff(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/api/stats", "/api/products", "/api/decisions"} {
		if rec := get(t, s, p); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404 (API mặc định TẮT)", p, rec.Code)
		}
	}
	rec := postForm(t, s, "/settings/api", url.Values{"value": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/api code = %d, want 303", rec.Code)
	}
	if rec := get(t, s, "/api/stats"); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/stats sau khi bật = %d, want 200", rec.Code)
	}
	rec = postForm(t, s, "/settings/api", url.Values{"value": {"0"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/api tắt code = %d, want 303", rec.Code)
	}
	if rec := get(t, s, "/api/stats"); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/stats sau khi tắt = %d, want 404", rec.Code)
	}
}

// TestOnboardGate — thư mục dữ liệu trống → mọi route 303 về /onboard;
// hoàn tất wizard (1 POST) → vào app bình thường (nghiệm thu R2-W7).
func TestOnboardGate(t *testing.T) {
	s := newTestServer(t)
	s.Onboarding = true

	rec := get(t, s, "/")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/onboard" {
		t.Fatalf("GET / lúc onboarding = %d -> %q, want 303 -> /onboard",
			rec.Code, rec.Header().Get("Location"))
	}
	rec = get(t, s, "/settings/he-thong")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /settings/he-thong lúc onboarding = %d, want 303", rec.Code)
	}
	rec = get(t, s, "/onboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /onboard = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Thiết lập lần đầu") {
		t.Fatal("trang /onboard thiếu nội dung wizard")
	}

	rec = postForm(t, s, "/onboard", url.Values{"dryrun": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /onboard = %d, want 303", rec.Code)
	}
	if s.Onboarding {
		t.Fatal("wizard chưa tắt sau POST /onboard")
	}
	if !s.Cfg.DryRun() {
		t.Fatal("dryrun=1 phải bật dry-run")
	}
	rec = get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / sau wizard = %d, want 200", rec.Code)
	}
}

// TestSettingsHeThongR2W7 — trang Hệ thống render đủ các khối mới của
// R2-W7: phiên bản, công tắc /api/*, sao lưu & khôi phục.
func TestSettingsHeThongR2W7(t *testing.T) {
	s := newTestServer(t)
	s.Cfg.Version = "v9.9-test"
	rec := get(t, s, "/settings/he-thong")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/he-thong = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"v9.9-test",                // phiên bản app
		"/settings/api",            // công tắc /api/*
		"Sao lưu",                  // khối sao lưu
		"/settings/backup",         // nút tạo bản sao lưu
		"/settings/backup/restore", // form khôi phục
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang Hệ thống thiếu %q", want)
		}
	}
}

// TestSettingsModelLocalRuntimeTools — trang Model local có bảng công cụ
// hệ thống từ probe thật (không tuyên bố "sẵn sàng" khi thiếu).
func TestSettingsModelLocalRuntimeTools(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/settings/model-local")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/model-local = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Công cụ hệ thống", "FFmpeg", "llama-server", "uv"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang Model local thiếu %q", want)
		}
	}
}

// TestOnboardPageRendersAlone — /onboard render được ngay cả khi không
// qua gate (không phụ thuộc dữ liệu nào khác).
func TestOnboardPageRendersAlone(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/onboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /onboard = %d, want 200", rec.Code)
	}
}

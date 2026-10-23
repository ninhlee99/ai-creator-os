package web

// Tests cho đợt AA (settings + kênh): YouTube OAuth trong app, badge
// token chết, middleware chống CSRF, validate biến cấu hình.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// YouTube authorize từ chối khi chưa có Client ID/Secret.
func TestYouTubeAuthorizeNoClient(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("yt_no_client", "", ""); err != nil {
		t.Fatal(err)
	}
	rec := get(t, s, "/publishers/youtube/authorize?username=yt_no_client")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/publishers?") || !strings.Contains(loc, "err=") {
		t.Fatalf("redirect = %q, want /publishers?err=...", loc)
	}
	if !strings.Contains(loc, "YOUTUBE_CLIENT_ID") {
		t.Fatalf("thiếu hướng dẫn nhập client: %q", loc)
	}
}

// YouTube authorize từ chối username không tồn tại.
func TestYouTubeAuthorizeUnknownAccount(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/publishers/youtube/authorize?username=khong_co")
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("redirect = %q, want err", loc)
	}
}

// YouTube authorize redirect tới Google khi đủ client + host loopback.
func TestYouTubeAuthorizeRedirectsToGoogle(t *testing.T) {
	s := newTestServer(t)
	t.Setenv("YOUTUBE_CLIENT_ID", "test-id")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "test-secret")
	if _, err := s.Mgr.Add("yt_ok", "", ""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/publishers/youtube/authorize?username=yt_ok", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("redirect = %q, want Google auth URL", loc)
	}
	if !strings.Contains(loc, "access_type%3Doffline") && !strings.Contains(loc, "access_type=offline") {
		t.Fatalf("thiếu access_type=offline (không có refresh_token): %q", loc)
	}
}

// youtubeRedirectURI từ chối host không phải loopback.
func TestYouTubeRedirectURINonLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example.com"
	if _, err := youtubeRedirectURI(req); err == nil {
		t.Fatal("want error for non-loopback host")
	}
	req.Host = "127.0.0.1:8080"
	u, err := youtubeRedirectURI(req)
	if err != nil {
		t.Fatal(err)
	}
	if u != "http://127.0.0.1:8080/publishers/youtube/callback" {
		t.Fatalf("redirect URI = %q", u)
	}
}

// sameOrigin: bảng kiểm tra.
func TestSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		referer string
		want    bool
	}{
		{"không header (curl)", "", "", true},
		{"origin khớp", "http://127.0.0.1:8080", "", true},
		{"origin khác host", "https://evil.com", "", false},
		{"origin khác cổng", "http://127.0.0.1:9999", "", false},
		{"referer khớp khi không có origin", "", "http://127.0.0.1:8080/settings", true},
		{"referer khác", "", "https://evil.com/x", false},
		{"origin null (sandbox)", "null", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/kill", nil)
			req.Host = "127.0.0.1:8080"
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				req.Header.Set("Referer", c.referer)
			}
			if got := sameOrigin(req); got != c.want {
				t.Fatalf("sameOrigin = %v, want %v", got, c.want)
			}
		})
	}
}

// originGate: POST từ origin lạ bị 403, POST nội bộ vẫn qua.
func TestOriginGateBlocksCrossSitePost(t *testing.T) {
	s := newTestServer(t)
	evil := httptest.NewRequest(http.MethodPost, "/kill", nil)
	evil.Host = "127.0.0.1:8080"
	evil.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, evil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST = %d, want 403", rec.Code)
	}
	// POST nội bộ (không Origin, như curl) vẫn qua middleware.
	local := httptest.NewRequest(http.MethodPost, "/kill", strings.NewReader(""))
	local.Host = "127.0.0.1:8080"
	local.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	serveLocalTestRoute(s, rec2, local)
	if rec2.Code == http.StatusForbidden {
		t.Fatal("POST nội bộ bị chặn oan")
	}
}

// Badge token chết: setting token_dead → trang publishers hiện đỏ.
func TestPublishersDeadTokenBadge(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("yt_dead", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Ledger.SetSetting(tokenDeadKey("yt_dead"), "2026-10-12T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rec := get(t, s, "/publishers")
	body := rec.Body.String()
	if !strings.Contains(body, "Hết hạn — cần nối lại") {
		t.Fatal("thiếu badge đỏ token chết")
	}
	if strings.Contains(body, "yt_dead</strong></td>\n    <td><span class=\"badge badge-ok\">Có</span>") {
		t.Fatal("vẫn hiện badge xanh dù token chết")
	}
}

// Validate YOUTUBE_DEFAULT_PRIVACY: giá trị sai bị từ chối.
func TestEnvPrivacyValidation(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/settings/env", url.Values{
		"name":  {"YOUTUBE_DEFAULT_PRIVACY"},
		"value": {"publick"},
	})
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "enverr=") {
		t.Fatalf("redirect = %q, want enverr", loc)
	}
	// Giá trị hợp lệ được lưu.
	rec = postForm(t, s, "/settings/env", url.Values{
		"name":  {"YOUTUBE_DEFAULT_PRIVACY"},
		"value": {"unlisted"},
	})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "envsaved=") {
		t.Fatalf("redirect = %q, want envsaved", loc)
	}
}

// Thêm kênh trùng username → báo lỗi rõ, không nhận vơ.
func TestAccountCreateDuplicate(t *testing.T) {
	s := newTestServer(t)
	postForm(t, s, "/accounts", url.Values{"username": {"trung_test"}})
	rec := postForm(t, s, "/accounts", url.Values{"username": {"trung_test"}})
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/accounts/new?") || !strings.Contains(loc, "err=") {
		t.Fatalf("redirect = %q, want /accounts/new?err=", loc)
	}
}

// Username rỗng → báo lỗi rõ.
func TestAccountCreateEmptyUsername(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/accounts", url.Values{"username": {"   "}})
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=") {
		t.Fatalf("redirect = %q, want err", loc)
	}
}

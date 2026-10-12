package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Hash + kiểm tra mật khẩu (PBKDF2, salt ngẫu nhiên mỗi lần).
func TestAuthPasswordHashVerify(t *testing.T) {
	h1, err := hashAuthPassword("mat-khau-123")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hashAuthPassword("mat-khau-123")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("hai lần hash giống nhau — salt không ngẫu nhiên?")
	}
	if !strings.HasPrefix(h1, "v1$") {
		t.Fatalf("định dạng hash sai: %q", h1[:8])
	}
	if !checkAuthPassword("mat-khau-123", h1) {
		t.Fatal("mật khẩu đúng bị từ chối")
	}
	if checkAuthPassword("sai-mat-khau", h1) {
		t.Fatal("mật khẩu sai được chấp nhận")
	}
	if checkAuthPassword("mat-khau-123", "v1$deadbeef$00") {
		t.Fatal("hash hỏng được chấp nhận")
	}
	if _, err := hashAuthPassword("abc"); err == nil {
		t.Fatal("mật khẩu 3 ký tự phải bị từ chối")
	}
}

// Session: tạo → hợp lệ → hủy → hết hạn.
func TestAuthSessionLifecycle(t *testing.T) {
	s := &Server{}
	tok, err := s.newAuthSession()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: authSessionCookie, Value: tok})
	if !s.validAuthSession(req) {
		t.Fatal("session vừa tạo bị coi là không hợp lệ")
	}
	rr := httptest.NewRecorder()
	s.destroyAuthSession(rr, req)
	if s.validAuthSession(req) {
		t.Fatal("session đã hủy vẫn hợp lệ")
	}
	// Cookie không có → không hợp lệ.
	if s.validAuthSession(httptest.NewRequest("GET", "/", nil)) {
		t.Fatal("request không cookie được coi là đã đăng nhập")
	}
}

// authGate: chưa đặt mật khẩu → đi qua hết.
func TestAuthGateDisabled(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	s := &Server{} // Ledger nil → authEnabled() false
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rr := httptest.NewRecorder()
	s.authGate(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusTeapot {
		t.Fatalf("chưa đặt mật khẩu mà bị chặn: %d", rr.Code)
	}
}

// authGate với mật khẩu đã đặt: chặn route thường, cho /login + /healthz
// qua, đăng nhập đúng thì vào được.
func TestAuthGateEnabledFullFlow(t *testing.T) {
	s := newTestServer(t)
	if err := s.setAuthPassword("bi-mat-123"); err != nil {
		t.Fatal(err)
	}
	if !s.authEnabled() {
		t.Fatal("đã đặt mật khẩu mà authEnabled() false")
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	gated := s.authGate(next)

	// Chưa đăng nhập → /dashboard bị đẩy về /login.
	rr := httptest.NewRecorder()
	gated.ServeHTTP(rr, httptest.NewRequest("GET", "/dashboard", nil))
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "/login") {
		t.Fatalf("chưa đăng nhập phải redirect /login, got %d %q", rr.Code, rr.Header().Get("Location"))
	}
	// /healthz và /login luôn qua (watchdog + trang đăng nhập).
	for _, p := range []string{"/healthz", "/login", "/static/app.js"} {
		rr := httptest.NewRecorder()
		gated.ServeHTTP(rr, httptest.NewRequest("GET", p, nil))
		if rr.Code != http.StatusTeapot {
			t.Fatalf("%s phải đi qua gate, got %d", p, rr.Code)
		}
	}

	// Đăng nhập sai → ở lại trang login với thông báo lỗi.
	form := strings.NewReader("password=sai&next=%2F")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Sai mật khẩu") {
		t.Fatalf("sai mật khẩu phải ở lại login + báo lỗi, got %d", rr.Code)
	}

	// Đăng nhập đúng → có cookie session → vào được /dashboard.
	form = strings.NewReader("password=bi-mat-123&next=%2Fdashboard")
	req = httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("đăng nhập đúng phải redirect, got %d", rr.Code)
	}
	var sess *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == authSessionCookie {
			sess = c
		}
	}
	if sess == nil || sess.HttpOnly != true {
		t.Fatal("thiếu cookie session HttpOnly sau đăng nhập")
	}
	req2 := httptest.NewRequest("GET", "/dashboard", nil)
	req2.AddCookie(sess)
	rr2 := httptest.NewRecorder()
	gated.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTeapot {
		t.Fatalf("đã đăng nhập phải vào được, got %d", rr2.Code)
	}

	// Đăng xuất → session chết.
	req3 := httptest.NewRequest("POST", "/logout", nil)
	req3.AddCookie(sess)
	rr3 := httptest.NewRecorder()
	s.handleLogout(rr3, req3)
	if s.validAuthSession(req3) {
		t.Fatal("đăng xuất rồi mà session vẫn sống")
	}
}

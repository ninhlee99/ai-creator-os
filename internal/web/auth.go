package web

// Mật khẩu truy cập dashboard (Đợt T) — điều kiện tiên quyết để Ninh mở app
// ra LAN/VPS ("chạy ở bất kỳ đâu") mà không phơi control plane.
//
// Thiết kế đơn giản, đủ cho app cá nhân single-user:
//   - Chưa đặt mật khẩu → app chạy như cũ (mặc định bind localhost).
//   - Đã đặt → mọi route riêng tư yêu cầu đăng nhập, trừ /login, /logout,
//     /static/* và /healthz trên loopback. Từ xa chỉ mở allowlist Bio.
//   - Hash: PBKDF2-HMAC-SHA256 100k vòng, salt 16 byte (stdlib only),
//     lưu "v1$salthex$hashhex" trong ledger settings (auth.password_hash).
//   - Session: token ngẫu nhiên 32 byte, cookie HttpOnly + SameSite=Lax,
//     hết hạn 30 ngày, lưu trong bộ nhớ (restart là đăng nhập lại).
// Đây KHÔNG phải SSO/MFA — đủ chống truy cập vô tình và quét port cơ bản.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// KeyAuthPasswordHash là key ledger settings lưu hash mật khẩu.
	KeyAuthPasswordHash = "auth.password_hash"
	authSessionCookie   = "aicos_session"
	authSessionTTL      = 30 * 24 * time.Hour
	authPBKDF2Iter      = 100_000
	authMinLen          = 4
)

// pbkdf2SHA256 là PBKDF2-HMAC-SHA256 viết tay bằng stdlib (tránh thêm
// dependency x/crypto chỉ cho một hàm băm mật khẩu).
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := func(key, data []byte) []byte {
		m := hmac.New(func() hash.Hash { return sha256.New() }, key)
		m.Write(data)
		return m.Sum(nil)
	}
	var out []byte
	var block [4]byte
	for i := 1; len(out) < keyLen; i++ {
		block[0] = byte(i >> 24)
		block[1] = byte(i >> 16)
		block[2] = byte(i >> 8)
		block[3] = byte(i)
		u := prf(password, append(salt, block[:]...))
		t := make([]byte, len(u))
		copy(t, u)
		for n := 1; n < iter; n++ {
			u = prf(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func hashAuthPassword(password string) (string, error) {
	if len([]rune(password)) < authMinLen {
		return "", fmt.Errorf("mật khẩu quá ngắn (tối thiểu %d ký tự)", authMinLen)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("tạo salt: %w", err)
	}
	sum := pbkdf2SHA256([]byte(password), salt, authPBKDF2Iter, 32)
	return "v1$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(sum), nil
}

func checkAuthPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != "v1" {
		return false
	}
	salt, err1 := hex.DecodeString(parts[1])
	want, err2 := hex.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(want) != 32 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, authPBKDF2Iter, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// authEnabled báo đã đặt mật khẩu chưa.
func (s *Server) authEnabled() bool {
	if s.Ledger == nil {
		return false
	}
	v, ok, err := s.Ledger.GetSetting(KeyAuthPasswordHash)
	return err == nil && ok && v != ""
}

// setAuthPassword đặt/đổi mật khẩu. pw rỗng = tắt (dùng clearAuthPassword).
func (s *Server) setAuthPassword(pw string) error {
	if s.Ledger == nil {
		return fmt.Errorf("ledger chưa sẵn sàng")
	}
	h, err := hashAuthPassword(pw)
	if err != nil {
		return err
	}
	return s.Ledger.SetSetting(KeyAuthPasswordHash, h)
}

func (s *Server) clearAuthPassword() error {
	if s.Ledger == nil {
		return fmt.Errorf("ledger chưa sẵn sàng")
	}
	return s.Ledger.SetSetting(KeyAuthPasswordHash, "")
}

// newAuthSession tạo session mới, trả về token cho cookie.
func (s *Server) newAuthSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	s.authMu.Lock()
	if s.authSessions == nil {
		s.authSessions = make(map[string]time.Time)
	}
	// Dọn session hết hạn ké (tránh map phình trên máy 24/7).
	now := time.Now()
	for t, exp := range s.authSessions {
		if exp.Before(now) {
			delete(s.authSessions, t)
		}
	}
	s.authSessions[tok] = now.Add(authSessionTTL)
	s.authMu.Unlock()
	return tok, nil
}

func (s *Server) validAuthSession(r *http.Request) bool {
	c, err := r.Cookie(authSessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	s.authMu.Lock()
	exp, ok := s.authSessions[c.Value]
	s.authMu.Unlock()
	return ok && time.Now().Before(exp)
}

func (s *Server) destroyAuthSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(authSessionCookie); err == nil && c.Value != "" {
		s.authMu.Lock()
		delete(s.authSessions, c.Value)
		s.authMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// authGate chặn route riêng tư khi chưa đăng nhập. Remote chỉ được vào
// allowlist Bio; /healthz chỉ mở từ loopback cho watchdog.
func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remote := isRemoteRequest(r)
		if isPublicBioRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.authEnabled() {
			// Remote visitors get only the explicit public bio allowlist.
			// Private pages fail closed even when no dashboard password exists.
			if remote {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(remoteBlockedHTML))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/login" || p == "/logout" || (!remote && p == "/healthz") ||
			p == "/favicon.ico" || strings.HasPrefix(p, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if s.validAuthSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
	})
}

// isRemoteRequest: request không đến từ chính máy chủ — địa chỉ không phải
// loopback, hoặc đi qua proxy/tunnel (cloudflared, Tailscale serve, nginx…)
// vốn kết nối từ 127.0.0.1 nhưng gắn header chuyển tiếp.
func isRemoteRequest(r *http.Request) bool {
	for _, h := range []string{"X-Forwarded-For", "Forwarded", "X-Real-Ip", "Cf-Connecting-Ip", "Tailscale-User-Login"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

const remoteBlockedHTML = `<!doctype html><html lang="vi"><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Cần đặt mật khẩu — MiuNova</title>
<body style="font-family:system-ui,sans-serif;max-width:32rem;margin:15vh auto;padding:0 16px;line-height:1.5;color:#1F2328">
<h1 style="font-size:22px">Cần đặt mật khẩu trước</h1>
<p>App chưa có mật khẩu nên chỉ mở được trên chính máy đang chạy app.</p>
<p>Trên máy đó, mở <b>http://127.0.0.1:8080/settings/system</b> → mục <b>Mật khẩu truy cập</b> → đặt mật khẩu. Sau đó vào lại trang này.</p>
</body></html>`

// handleLogin: GET → trang đăng nhập; POST → kiểm tra mật khẩu.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			s.fail(w, err, "parse login")
			return
		}
		pw := r.PostFormValue("password")
		stored, ok, _ := s.Ledger.GetSetting(KeyAuthPasswordHash)
		if !ok || !checkAuthPassword(pw, stored) {
			// Chung chung cố ý — không tiết lộ "chưa đặt mật khẩu".
			s.renderLogin(w, "Sai mật khẩu.", r.URL.Query().Get("next"))
			return
		}
		tok, err := s.newAuthSession()
		if err != nil {
			s.fail(w, err, "new session")
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     authSessionCookie,
			Value:    tok,
			Path:     "/",
			MaxAge:   int(authSessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   isHTTPSRequest(r),
			SameSite: http.SameSiteLaxMode,
		})
		next := r.PostFormValue("next")
		// Đợt AF: chặn open redirect — "//evil.com" cũng có prefix "/" nên
		// phải loại riêng (protocol-relative URL).
		if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
			next = "/"
		}
		_ = s.Ledger.Decide("human", "login", nil, "Đăng nhập dashboard.", nil)
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	if s.validAuthSession(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, r.URL.Query().Get("err"), r.URL.Query().Get("next"))
}

func (s *Server) renderLogin(w http.ResponseWriter, errMsg, next string) {
	t, ok := s.templates["login"]
	if !ok {
		http.Error(w, "login template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Err": errMsg, "Next": next}
	if err := t.ExecuteTemplate(w, "login", data); err != nil {
		log.Printf("web: render login: %v", err)
	}
}

// handleLogout: POST → hủy session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.destroyAuthSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

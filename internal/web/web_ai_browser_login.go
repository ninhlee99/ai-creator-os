package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"strings"
	"time"
)

func rejectMobileBrowserLogin(w http.ResponseWriter, r *http.Request) bool {
	clientHint := strings.TrimSpace(r.Header.Get("Sec-CH-UA-Mobile"))
	userAgent := strings.ToLower(r.UserAgent())
	mobile := clientHint == "?1"
	for _, marker := range []string{"android", "iphone", "ipad", "ipod", "mobile", "windows phone", "iemobile", "opera mini"} {
		if strings.Contains(userAgent, marker) {
			mobile = true
			break
		}
	}
	if !mobile {
		return false
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Đăng nhập Web AI chỉ hỗ trợ trên máy tính."})
	return true
}

// handleWebAIBrowserLoginStart starts an automated Chromium CDP login session for a platform.
func (s *Server) handleWebAIBrowserLoginStart(w http.ResponseWriter, r *http.Request) {
	if rejectMobileBrowserLogin(w, r) {
		return
	}
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa được khởi tạo"})
		return
	}

	var request struct {
		Platform string `json:"platform"`
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") && r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Yêu cầu đăng nhập không hợp lệ"})
			return
		}
	} else if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Yêu cầu đăng nhập không hợp lệ"})
		return
	}
	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = strings.TrimSpace(request.Platform)
	}
	if platform == "" {
		platform = strings.TrimSpace(r.FormValue("platform"))
	}
	if platform == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu thông tin nền tảng (platform)"})
		return
	}
	targets := webai.SupportedWebLoginTargets()
	target, ok := targets[platform]
	if !ok {
		s.writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("Nền tảng %s chưa hỗ trợ đăng nhập trình duyệt tự động", platform)})
		return
	}

	mgr := webai.GetLoginManager()
	const loginTimeout = 10 * time.Minute
	session, err := mgr.StartSession(platform, loginTimeout)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Launch CDP capture in background goroutine
	go func(sess *webai.ActiveLoginSession, tgt webai.WebLoginTarget) {
		auth, captureErr := webai.RunCaptureViaCDPWithBrowserReady(sess.Context(), tgt, loginTimeout, func(status, msg string) {
			mgr.UpdateSession(sess.ID, status, msg, "", "")
		}, func(port int) {
			mgr.SetBrowserPort(sess.ID, port)
		})

		if captureErr != nil {
			current, _ := mgr.GetSession(sess.ID)
			if (current == nil || current.Status != "canceled") && !errors.Is(captureErr, context.Canceled) {
				mgr.UpdateSession(sess.ID, "failed", "Đăng nhập thất bại: "+captureErr.Error(), "", captureErr.Error())
			} else {
				mgr.UpdateSession(sess.ID, "canceled", "Đã hủy phiên đăng nhập.", "", "")
			}
			return
		}

		if auth != nil && auth.SessionValue != "" {
			if tgt.Platform == webai.PlatformMuseAI && strings.TrimSpace(auth.CookieHeader) == "" {
				msg := "Muse đã đăng nhập nhưng trình duyệt không trả cookie phiên để lưu. Hãy tải lại Muse rồi thử lại."
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			email := strings.TrimSpace(auth.Email)
			if email == "" && tgt.Platform != webai.PlatformMuseAI {
				msg := "Đã xác thực phiên nhưng chưa xác minh được email thật; chưa lưu tài khoản."
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			accountName := tgt.Name
			if email != "" {
				accountName = fmt.Sprintf("%s (%s)", tgt.Name, email)
			} else if tgt.Platform == webai.PlatformMuseAI {
				accountName = fmt.Sprintf("%s (phiên %s)", tgt.Name, time.Now().Format("02/01 15:04:05"))
			}
			id, err := s.WebAI.UpsertAccountSessionWithCookieJar(tgt.Platform, auth.SessionValue, auth.CookieHeader, auth.CookieJar, accountName, email, "unknown")
			if err != nil {
				msg := "Lưu tài khoản thất bại: " + err.Error()
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			saved, verifyErr := s.WebAI.GetAccount(id)
			if verifyErr != nil || saved == nil || saved.SessionKey != auth.SessionValue || saved.Cookie != auth.CookieHeader || len(saved.CookieJar) == 0 {
				msg := "Phiên đã gửi vào kho nhưng xác nhận đọc lại không khớp; chưa báo thành công."
				if verifyErr != nil {
					msg += " " + verifyErr.Error()
				}
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}

			successLabel := email
			if successLabel == "" {
				successLabel = tgt.Name
			}
			mgr.UpdateSession(sess.ID, "success", fmt.Sprintf("Đã lưu phiên %s.", successLabel), email, "")
			if s.Ledger != nil {
				_ = s.Ledger.Decide("web_ai", "cdp_login_success", nil, fmt.Sprintf("Đăng nhập CDP thành công %s id=%s email=%s", tgt.Platform, id, email), nil)
			}
		}
	}(session, target)

	initial, _ := mgr.GetSession(session.ID)
	s.writeJSON(w, map[string]any{
		"ok":         true,
		"session_id": session.ID,
		"platform":   platform,
		"status":     initial.Status,
		"message":    initial.Message,
	})
}

// handleWebAIBrowserLoginScreen returns a no-store screenshot from the active
// loopback browser. The route remains behind dashboard authentication.
func (s *Server) handleWebAIBrowserLoginScreen(w http.ResponseWriter, r *http.Request) {
	if rejectMobileBrowserLogin(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get("X-Login-Session"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		http.Error(w, "thiếu phiên đăng nhập", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	imageData, width, height, err := webai.GetLoginManager().BrowserScreenshot(ctx, sessionID)
	if err != nil {
		http.Error(w, "trình duyệt đăng nhập chưa sẵn sàng", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Viewport-Width", fmt.Sprint(width))
	w.Header().Set("X-Viewport-Height", fmt.Sprint(height))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(imageData)
}

// handleWebAIBrowserLoginInput forwards one bounded same-origin interaction.
func (s *Server) handleWebAIBrowserLoginInput(w http.ResponseWriter, r *http.Request) {
	if rejectMobileBrowserLogin(w, r) {
		return
	}
	var input webai.LoginBrowserInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu thao tác không hợp lệ"})
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get("X-Login-Session"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu phiên đăng nhập"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := webai.GetLoginManager().BrowserInput(ctx, sessionID, input); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không gửi được thao tác tới trình duyệt"})
		return
	}
	s.writeJSON(w, map[string]any{"ok": true})
}

// handleWebAIBrowserLoginStatus checks the current state of a CDP login attempt.
func (s *Server) handleWebAIBrowserLoginStatus(w http.ResponseWriter, r *http.Request) {
	if rejectMobileBrowserLogin(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get("X-Login-Session"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.FormValue("session_id"))
	}
	if sessionID == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu session_id"})
		return
	}

	mgr := webai.GetLoginManager()
	session, ok := mgr.GetSession(sessionID)
	if !ok {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy phiên đăng nhập"})
		return
	}

	s.writeJSON(w, map[string]any{
		"ok":         true,
		"session_id": session.ID,
		"platform":   session.Platform,
		"status":     session.Status,
		"message":    session.Message,
		"email":      session.Email,
		"error":      session.Error,
	})
}

// handleWebAIBrowserLoginCancel cancels an active login attempt.
func (s *Server) handleWebAIBrowserLoginCancel(w http.ResponseWriter, r *http.Request) {
	if rejectMobileBrowserLogin(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get("X-Login-Session"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.FormValue("session_id"))
	}
	if sessionID == "" && r.Body != nil {
		var req struct {
			SessionID string `json:"session_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		sessionID = strings.TrimSpace(req.SessionID)
	}
	if sessionID != "" {
		webai.GetLoginManager().CancelSession(sessionID)
	}
	s.writeJSON(w, map[string]any{"ok": true, "message": "Đã hủy"})
}

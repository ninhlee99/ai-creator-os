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

// handleWebAICaptureSession is retained as a compatibility response only.
func (s *Server) handleWebAICaptureSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusGone)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": "Đăng nhập thủ công đã tắt. Hãy dùng nút Đăng nhập để xác minh phiên.",
	})
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
				email = fmt.Sprintf("%s_user_%d@web.ai", tgt.Platform, time.Now().Unix()%100000)
			}
			accountName := tgt.Name
			if email != "" {
				accountName = fmt.Sprintf("%s (%s)", tgt.Name, email)
			} else if tgt.Platform == webai.PlatformMuseAI {
				accountName = fmt.Sprintf("%s (phiên %s)", tgt.Name, time.Now().Format("02/01 15:04:05"))
			}
			id, err := s.WebAI.UpsertAccountSession(tgt.Platform, auth.SessionValue, auth.CookieHeader, accountName, email, "unknown")
			if err != nil {
				msg := "Lưu tài khoản thất bại: " + err.Error()
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			if err := s.WebAI.ConfirmFreeAccount(id); err != nil {
				msg := "Phiên đã lưu nhưng chưa xác nhận được gói miễn phí: " + err.Error()
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			saved, verifyErr := s.WebAI.GetAccount(id)
			if verifyErr != nil || saved == nil || !saved.IsConfirmedFree() || saved.SessionKey != auth.SessionValue || (auth.CookieHeader != "" && saved.Cookie != auth.CookieHeader) {
				msg := "Phiên đã gửi vào kho nhưng xác nhận đọc lại không khớp; chưa báo thành công."
				if verifyErr != nil {
					msg += " " + verifyErr.Error()
				}
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}

			mgr.UpdateSession(sess.ID, "success", fmt.Sprintf("✓ Đã lưu phiên %s vào nhóm tài khoản miễn phí.", email), email, "")
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

// handleWebAISyncGoogleAccount syncs a selected, verified Google session to Flow.
func (s *Server) handleWebAISyncGoogleAccount(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa được khởi tạo"})
		return
	}

	var request struct {
		AccountID   string `json:"account_id"`
		SyncAll     bool   `json:"sync_all"`
		ConfirmFree bool   `json:"confirm_free"`
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") && r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Yêu cầu đồng bộ không hợp lệ"})
			return
		}
	} else if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Yêu cầu đồng bộ không hợp lệ"})
		return
	}
	if request.AccountID == "" {
		request.AccountID = strings.TrimSpace(r.URL.Query().Get("account_id"))
	}
	if request.AccountID == "" {
		request.AccountID = strings.TrimSpace(r.FormValue("account_id"))
	}
	if !request.ConfirmFree && r.FormValue("confirm_free") != "1" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Hãy xác nhận chỉ đồng bộ tài khoản miễn phí đã được đánh dấu Free."})
		return
	}

	accounts, err := s.WebAI.GetAccounts(webai.PlatformGemini)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được tài khoản Gemini: " + err.Error()})
		return
	}
	if len(accounts) == 0 {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Chưa có tài khoản Google (Gemini) nào được kết nối trong hệ thống"})
		return
	}
	var selected []*webai.Account
	if request.AccountID != "" {
		for i := range accounts {
			if accounts[i].ID == request.AccountID {
				selected = append(selected, &accounts[i])
				break
			}
		}
		if len(selected) == 0 {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy tài khoản Gemini đã chọn"})
			return
		}
		if !selected[0].IsConfirmedFree() {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Tài khoản này chưa được xác nhận gói miễn phí; đăng nhập lại và xác nhận gói trước."})
			return
		}
	} else if request.SyncAll {
		for i := range accounts {
			acc := &accounts[i]
			if acc.IsActive && acc.IsConfirmedFree() && (acc.Status == "ok" || acc.Status == "connected") {
				selected = append(selected, acc)
			}
		}
	} else {
		for i := range accounts {
			acc := &accounts[i]
			if acc.IsActive && acc.IsConfirmedFree() && (acc.Status == "ok" || acc.Status == "connected") {
				selected = append(selected, acc)
			}
		}
		if len(selected) > 1 {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Có nhiều tài khoản Gemini miễn phí; chọn tài khoản cần đồng bộ."})
			return
		}
	}
	if len(selected) == 0 {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không có tài khoản Gemini miễn phí đã xác nhận và sẵn sàng để đồng bộ."})
		return
	}

	var syncedIDs, syncedEmails, failures []string
	for _, geminiAcc := range selected {
		cookieHdr := strings.TrimSpace(geminiAcc.Cookie)
		sessionKey := strings.TrimSpace(geminiAcc.SessionKey)
		if cookieHdr == "" {
			cookieHdr = sessionKey
		}
		email := strings.TrimSpace(geminiAcc.Email)
		if cookieHdr == "" || email == "" {
			failures = append(failures, fmt.Sprintf("%s: thiếu cookie hoặc email đã xác minh", emailOrLabel(email, geminiAcc.AccountName)))
			continue
		}
		if err := webai.ValidateWebAccountSession(r.Context(), webai.PlatformGoogleFlow, sessionKey, cookieHdr, email); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", email, err))
			continue
		}
		accountName := fmt.Sprintf("Google Flow (%s)", email)
		id, err := s.WebAI.UpsertAccountSession(webai.PlatformGoogleFlow, sessionKey, cookieHdr, accountName, email, "unknown")
		if err == nil {
			err = s.WebAI.ConfirmFreeAccount(id)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: không lưu được Flow: %v", email, err))
			continue
		}
		syncedIDs = append(syncedIDs, id)
		syncedEmails = append(syncedEmails, email)
	}
	if len(syncedIDs) == 0 {
		message := "Không đồng bộ được tài khoản miễn phí nào sang Flow."
		if len(failures) > 0 {
			message += " " + strings.Join(failures, "; ")
		}
		s.writeJSON(w, map[string]any{"ok": false, "error": message})
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("web_ai", "sync_google_flow", nil, fmt.Sprintf("Đồng bộ %d tài khoản Google miễn phí sang Flow", len(syncedIDs)), nil)
	}

	message := fmt.Sprintf("Đã đồng bộ %d tài khoản Google miễn phí sang Flow: %s.", len(syncedIDs), strings.Join(syncedEmails, ", "))
	if len(failures) > 0 {
		message += fmt.Sprintf(" Bỏ qua %d tài khoản lỗi.", len(failures))
	}
	s.writeJSON(w, map[string]any{
		"ok":       true,
		"ids":      syncedIDs,
		"emails":   syncedEmails,
		"message":  message,
		"failures": failures,
	})
}

func emailOrLabel(email, label string) string {
	if strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if strings.TrimSpace(label) != "" {
		return strings.TrimSpace(label)
	}
	return "Gemini"
}

package web

import (
	"context"
	"errors"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type accountProbeOutcome string

const (
	accountProbeAlive   accountProbeOutcome = "alive"
	accountProbeExpired accountProbeOutcome = "expired"
	accountProbeUnknown accountProbeOutcome = "unknown"
)

func probeAccountSessionOutcome(ctx context.Context, acc *webai.Account) (accountProbeOutcome, string) {
	if acc == nil {
		return accountProbeExpired, "Tài khoản không tồn tại"
	}
	platform := strings.ToLower(strings.TrimSpace(acc.Platform))
	switch platform {
	case "museai", "muse_ai", "muse":
		return probeMuseAccountSession(ctx, acc)
	case "googleflow":
		return probeGoogleFlowSession(ctx, acc)
	}
	if platform == webai.PlatformGemini && strings.HasPrefix(strings.TrimSpace(acc.SessionKey), "AIzaSy") {
		return probeGeminiAPIKey(ctx, strings.TrimSpace(acc.SessionKey))
	}
	if strings.TrimSpace(acc.Cookie) == "" && strings.TrimSpace(acc.SessionKey) == "" {
		return accountProbeUnknown, "Tài khoản chưa có thông tin phiên để xác minh"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := webai.ValidateWebAccountSession(ctx, platform, acc.SessionKey, acc.Cookie, acc.Email); err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "http 401") || strings.Contains(lower, "http 403") || strings.Contains(lower, "unauthorized") {
			return accountProbeExpired, "Phiên đăng nhập đã hết hạn: " + err.Error()
		}
		return accountProbeUnknown, "Chưa xác minh được phiên: " + err.Error()
	}
	return accountProbeAlive, "Phiên đăng nhập hợp lệ"
}

func probeMuseAccountSession(ctx context.Context, acc *webai.Account) (accountProbeOutcome, string) {
	if acc == nil || strings.TrimSpace(acc.Cookie) == "" {
		return accountProbeUnknown, "Muse Chat chưa có cookie phiên"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := webai.ProbeMuseSession(ctx, acc.Cookie); err != nil {
		if strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "HTTP 403") && strings.Contains(err.Error(), "auth/check") {
			return accountProbeExpired, "Muse từ chối phiên đăng nhập"
		}
		return accountProbeUnknown, "Chưa xác minh được Muse: " + err.Error()
	}
	return accountProbeAlive, "Phiên Muse Chat hợp lệ; gateway đã phản hồi"
}

func probeGeminiAPIKey(ctx context.Context, key string) (accountProbeOutcome, string) {
	if strings.TrimSpace(key) == "" {
		return accountProbeUnknown, "Gemini API key chưa được cấu hình"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://generativelanguage.googleapis.com/v1beta/models?key="+url.QueryEscape(key), nil)
	if err != nil {
		return accountProbeUnknown, "Không tạo được yêu cầu kiểm tra Gemini API key: " + err.Error()
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return accountProbeUnknown, "Không kết nối được Gemini API: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return accountProbeAlive, "Gemini API key hợp lệ"
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return accountProbeExpired, fmt.Sprintf("Gemini API key bị từ chối (HTTP %d)", resp.StatusCode)
	}
	return accountProbeUnknown, fmt.Sprintf("Gemini API chưa xác minh được key (HTTP %d)", resp.StatusCode)
}

func (s *Server) persistWebAICookieRefresh(ctx context.Context, platform, email, cookie string, verify bool) error {
	if s.WebAI == nil {
		return errors.New("kho Web AI chưa được khởi tạo")
	}
	email = strings.TrimSpace(email)
	cookie = strings.TrimSpace(cookie)
	if email == "" || cookie == "" {
		return errors.New("thiếu email hoặc cookie phiên mới")
	}
	accounts, err := s.WebAI.GetAccounts(platform)
	if err != nil {
		return fmt.Errorf("đọc tài khoản %s: %w", platform, err)
	}
	for i := range accounts {
		if !strings.EqualFold(strings.TrimSpace(accounts[i].Email), email) {
			continue
		}
		if verify {
			if err := webai.ValidateWebAccountSession(ctx, platform, accounts[i].SessionKey, cookie, email); err != nil {
				return fmt.Errorf("xác minh cookie mới: %w", err)
			}
		}
		accounts[i].Cookie = cookie
		accounts[i].LastChecked = time.Now().UTC()
		return s.WebAI.UpdateAccount(accounts[i])
	}
	return fmt.Errorf("không tìm thấy tài khoản %s cho %s", email, platform)
}

func probeGoogleFlowSession(ctx context.Context, acc *webai.Account) (accountProbeOutcome, string) {
	if acc == nil {
		return accountProbeExpired, "Tài khoản không tồn tại"
	}
	cookie := strings.TrimSpace(acc.Cookie)
	if cookie == "" {
		cookie = strings.TrimSpace(acc.SessionKey)
	}
	if cookie == "" {
		return accountProbeUnknown, "Tài khoản Google Flow chưa có cookie phiên để xác minh"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := webai.ProbeGoogleFlowSession(ctx, cookie); err != nil {
		if strings.Contains(err.Error(), "Flow HTTP 401") {
			return accountProbeExpired, "Google Flow từ chối phiên đăng nhập"
		}
		if strings.Contains(err.Error(), "Flow HTTP 403") {
			return accountProbeUnknown, "Google Flow từ chối truy cập; kiểm tra quyền tài khoản và khu vực"
		}
		return accountProbeUnknown, "Chưa xác minh được Flow: " + err.Error()
	}
	return accountProbeAlive, "Phiên Google Flow hợp lệ; lịch sử đã phản hồi"
}

// handleWebAIAccountTest validates account session credentials with a live probe request.
func (s *Server) handleWebAIAccountTest(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa sẵn sàng"})
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))

	if id != "" {
		acc, err := s.WebAI.GetAccount(id)
		if err != nil || acc == nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy tài khoản"})
			return
		}
		outcome, msg := probeAccountSessionOutcome(r.Context(), acc)
		if outcome == accountProbeAlive {
			acc.Status = "ok"
		} else if outcome == accountProbeExpired {
			acc.Status = "expired"
		} else {
			acc.Status = webai.StatusUnverified
		}
		_ = s.WebAI.UpdateAccount(*acc)
		s.writeJSON(w, map[string]any{
			"ok":      outcome == accountProbeAlive,
			"outcome": outcome,
			"status":  acc.Status,
			"message": msg,
			"id":      acc.ID,
		})
		return
	}

	// Test all accounts concurrently
	allAccsMap, err := s.WebAI.GetAllAccounts()
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Lỗi lấy danh sách tài khoản: " + err.Error()})
		return
	}

	var allList []webai.Account
	for _, list := range allAccsMap {
		allList = append(allList, list...)
	}

	if len(allList) == 0 {
		s.writeJSON(w, map[string]any{"ok": true, "tested_count": 0, "message": "Không có tài khoản nào để kiểm tra"})
		return
	}

	type testResult struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Platform string `json:"platform"`
		Status   string `json:"status"`
		Message  string `json:"message"`
		Alive    bool   `json:"alive"`
		Outcome  string `json:"outcome"`
	}

	var results []testResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, a := range allList {
		wg.Add(1)
		go func(acc webai.Account) {
			defer wg.Done()
			outcome, msg := probeAccountSessionOutcome(r.Context(), &acc)
			if outcome == accountProbeAlive {
				acc.Status = "ok"
			} else if outcome == accountProbeExpired {
				acc.Status = "expired"
			} else {
				acc.Status = webai.StatusUnverified
			}
			_ = s.WebAI.UpdateAccount(acc)

			mu.Lock()
			results = append(results, testResult{
				ID:       acc.ID,
				Email:    acc.Email,
				Platform: acc.Platform,
				Status:   acc.Status,
				Message:  msg,
				Alive:    outcome == accountProbeAlive,
				Outcome:  string(outcome),
			})
			mu.Unlock()
		}(a)
	}

	wg.Wait()
	aliveCount := 0
	expiredCount := 0
	unknownCount := 0
	for _, res := range results {
		if res.Alive {
			aliveCount++
		} else if res.Outcome == string(accountProbeExpired) {
			expiredCount++
		} else {
			unknownCount++
		}
	}

	s.writeJSON(w, map[string]any{
		"ok":           true,
		"tested_count": len(results),
		"alive_count":  aliveCount,
		"results":      results,
		"message":      fmt.Sprintf("Đã kiểm tra %d tài khoản: %d còn sống ✓, %d hết hạn ⚠, %d chưa xác minh", len(results), aliveCount, expiredCount, unknownCount),
	})
}

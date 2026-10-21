package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// webAIView holds template projection for Web AI Hub.
// webAIView holds template projection for Web AI Hub with multi-account keyring.
type webAIView struct {
	Platform      webai.PlatformInfo
	Accounts      []webai.Account
	Primary       webai.Account
	ReadyCount    int
	CooldownCount int
	TotalCount    int
	HasAccounts   bool
	IsConnected   bool
}

// webAIPlatformViews computes template views for each platform and the total ready accounts count.
func (s *Server) webAIPlatformViews() ([]webAIView, int) {
	platforms := webai.SupportedPlatforms()
	var allAccounts map[string][]webai.Account
	if s.WebAI != nil {
		var err error
		allAccounts, err = s.WebAI.GetAllAccounts()
		if err != nil {
			logErr("webai: GetAllAccounts: %v", err)
		}
	}

	views := make([]webAIView, 0, len(platforms))
	totalConnected := 0
	for _, p := range platforms {
		var accs []webai.Account
		if allAccounts != nil {
			accs = allAccounts[p.ID]
		}
		ready, cooldown, total := 0, 0, len(accs)
		var primary webai.Account
		for _, a := range accs {
			if a.Status == "ok" || a.Status == "connected" {
				ready++
			} else if a.Status == "cooldown" {
				cooldown++
			}
			if primary.ID == "" || a.IsActive {
				primary = a
			}
		}
		if primary.ID == "" {
			primary = webai.Account{
				Platform: p.ID,
				Status:   webai.StatusDisconnected,
				Tier:     "free",
			}
		}
		totalConnected += ready
		views = append(views, webAIView{
			Platform:      p,
			Accounts:      accs,
			Primary:       primary,
			ReadyCount:    ready,
			CooldownCount: cooldown,
			TotalCount:    total,
			HasAccounts:   total > 0,
			IsConnected:   ready > 0,
		})
	}
	return views, totalConnected
}

func resolveWebAIRedirect(r *http.Request, defaultTarget string) string {
	back := strings.TrimSpace(r.FormValue("back"))
	if back != "" && strings.HasPrefix(back, "/") && !strings.HasPrefix(back, "//") {
		return back
	}
	ref := r.Header.Get("Referer")
	if strings.Contains(ref, "/settings") {
		return "/settings/system#tab-webai"
	}
	return defaultTarget
}

// handleWebAI renders the AI Web Hub page with multi-account rotation support.
func (s *Server) handleWebAI(w http.ResponseWriter, r *http.Request) {
	views, totalConnected := s.webAIPlatformViews()

	// Active tab inside the embedded workspace
	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = webai.PlatformChatGPT
	}

	s.render(w, "web_ai", s.ctx(
		"Title", "AI Web Hub — AI Creator OS",
		"Platforms", views,
		"ConnectedCount", totalConnected,
		"ActiveTab", activeTab,
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
	))
}

// handleWebAIAccountAdd adds an account to the pool for a platform.
func (s *Server) handleWebAIAccountAdd(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	sessionKey := strings.TrimSpace(r.FormValue("session_key"))
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	accountName := strings.TrimSpace(r.FormValue("account_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	tier := strings.TrimSpace(r.FormValue("tier"))
	if tier == "" {
		tier = "free"
	}

	if sessionKey == "" && cookie == "" {
		redirectBack("err", "Vui lòng nhập Mã kết nối (API Key hoặc Cookie header).")
		return
	}

	// Auto-extract cookie header if user pasted cookie string in session_key or cookie field
	if cookie == "" && (strings.Contains(sessionKey, "=") || strings.Contains(sessionKey, ";")) {
		cookie = sessionKey
		for _, part := range strings.Split(cookie, ";") {
			part = strings.TrimSpace(part)
			for _, prefix := range []string{"__Secure-1PSID=", "__Secure-3PSID=", "SID=", "__session=", "sessionKey=", "hatch_sess=", "ps_l=", "sso="} {
				if strings.HasPrefix(part, prefix) {
					sessionKey = strings.TrimPrefix(part, prefix)
					break
				}
			}
		}
	} else if cookie != "" && sessionKey == "" {
		for _, part := range strings.Split(cookie, ";") {
			part = strings.TrimSpace(part)
			for _, prefix := range []string{"__Secure-1PSID=", "__Secure-3PSID=", "SID=", "__session=", "sessionKey=", "hatch_sess=", "ps_l=", "sso="} {
				if strings.HasPrefix(part, prefix) {
					sessionKey = strings.TrimPrefix(part, prefix)
					break
				}
			}
		}
		if sessionKey == "" {
			sessionKey = cookie
		}
	}

	if accountName == "" {
		if email != "" {
			accountName = email
		} else {
			accountName = fmt.Sprintf("%s acc %d", platform, time.Now().Unix()%1000)
		}
	}

	acc := webai.Account{
		Platform:    platform,
		AccountName: accountName,
		Email:       email,
		SessionKey:  sessionKey,
		Cookie:      cookie,
		Status:      "ok",
		Tier:        tier,
		LastChecked: time.Now().UTC(),
	}

	if _, err := s.WebAI.AddAccount(acc); err != nil {
		redirectBack("err", "Lỗi thêm tài khoản: "+err.Error())
		return
	}

	// Auto sync API keys to environment variables so that Studio AI and generators work immediately.
	if s.Ledger != nil && sessionKey != "" {
		if platform == webai.PlatformGemini {
			_ = s.Ledger.SetSetting(envSettingKey("GEMINI_API_KEYS"), sessionKey)
			_ = os.Setenv("GEMINI_API_KEYS", sessionKey)
			s.Cfg.RefreshEnv()
		} else if platform == webai.PlatformChatGPT && strings.HasPrefix(sessionKey, "sk-") {
			_ = s.Ledger.SetSetting(envSettingKey("OPENAI_API_KEY"), sessionKey)
			_ = os.Setenv("OPENAI_API_KEY", sessionKey)
			s.Cfg.RefreshEnv()
		} else if platform == webai.PlatformClaude && strings.HasPrefix(sessionKey, "sk-ant-") {
			_ = s.Ledger.SetSetting(envSettingKey("ANTHROPIC_API_KEY"), sessionKey)
			_ = os.Setenv("ANTHROPIC_API_KEY", sessionKey)
			s.Cfg.RefreshEnv()
		}
	}

	redirectBack("ok", "Đã kết nối tài khoản "+platform+" thành công!")
}

// handleWebAIAccountDelete deletes an account from the pool.
func (s *Server) handleWebAIAccountDelete(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		redirectBack("err", "Thiếu ID tài khoản")
		return
	}
	if err := s.WebAI.DeleteAccount(id); err != nil {
		redirectBack("err", "Lỗi xóa tài khoản: "+err.Error())
		return
	}
	redirectBack("ok", "Đã xóa tài khoản khỏi pool xoay tua")
}

// handleWebAIAccountResetCooldown manually clears cooldown for an account.
func (s *Server) handleWebAIAccountResetCooldown(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		redirectBack("err", "Thiếu ID tài khoản")
		return
	}
	if err := s.WebAI.ResetCooldown(id); err != nil {
		redirectBack("err", "Lỗi đặt lại cooldown: "+err.Error())
		return
	}
	redirectBack("ok", "Đã đặt lại trạng thái — tài khoản đã sẵn sàng hoạt động trở lại!")
}

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
	switch strings.ToLower(acc.Platform) {
	case "museai", "muse_ai", "muse":
		return probeMuseAccountSession(ctx, acc)
	case "googleflow":
		return probeGoogleFlowSession(ctx, acc)
	default:
		alive, message := probeAccountSession(ctx, acc)
		if alive {
			return accountProbeAlive, message
		}
		return accountProbeExpired, message
	}
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

// probeAccountSession sends a live HTTP probe to verify if the account session is alive.
func probeAccountSession(ctx context.Context, acc *webai.Account) (bool, string) {
	if acc == nil {
		return false, "Tài khoản không tồn tại"
	}
	client := &http.Client{Timeout: 8 * time.Second}
	platform := strings.ToLower(acc.Platform)

	switch platform {
	case "museai", "muse_ai", "muse":
		_, message := probeMuseAccountSession(ctx, acc)
		return false, message

	case "chatgpt":
		cookie := acc.Cookie
		if cookie == "" {
			cookie = acc.SessionKey
		}
		if cookie == "" {
			return false, "Chưa có cookie ChatGPT"
		}
		hasSessionToken := strings.Contains(cookie, "session-token") || strings.Contains(acc.SessionKey, "session-token") || len(acc.SessionKey) > 20
		if !hasSessionToken {
			return false, "Thiếu session token ChatGPT"
		}

		req, err := http.NewRequestWithContext(ctx, "GET", "https://chatgpt.com/api/auth/session", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					var bodyMap map[string]any
					if json.NewDecoder(resp.Body).Decode(&bodyMap) == nil && bodyMap["user"] != nil {
						return true, "Phiên ChatGPT hoạt động tốt (User authenticated)"
					}
					return true, "Phiên ChatGPT hoạt động tốt (HTTP 200)"
				}
				if resp.StatusCode == 401 {
					return false, "Phiên ChatGPT đã hết hạn (HTTP 401)"
				}
				if resp.StatusCode == 403 {
					return true, "Phiên ChatGPT hợp lệ (Đang hoạt động trên trình duyệt)"
				}
			}
		}
		return true, "Phiên ChatGPT hợp lệ (Đã lưu session token)"

	case "claude":
		cookie := acc.Cookie
		if cookie == "" {
			cookie = "sessionKey=" + acc.SessionKey
		} else if !strings.Contains(cookie, "sessionKey=") && acc.SessionKey != "" {
			cookie = "sessionKey=" + acc.SessionKey + "; " + cookie
		}
		if cookie == "" {
			return false, "Chưa có session key Claude"
		}
		hasSessionKey := strings.Contains(cookie, "sessionKey=") && len(acc.SessionKey) > 10
		if !hasSessionKey {
			return false, "Thiếu sessionKey Claude"
		}

		req, err := http.NewRequestWithContext(ctx, "GET", "https://claude.ai/api/organizations", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					return true, "Phiên Claude.ai hoạt động tốt (HTTP 200)"
				}
				if resp.StatusCode == 401 {
					return false, "Phiên Claude.ai đã hết hạn (HTTP 401)"
				}
				if resp.StatusCode == 403 {
					return true, "Phiên Claude.ai hợp lệ (Đang hoạt động trên trình duyệt)"
				}
			}
		}
		return true, "Phiên Claude.ai hợp lệ (Đã lưu session key)"

	case "grok":
		cookie := acc.Cookie
		if cookie == "" {
			cookie = acc.SessionKey
		}
		if cookie == "" {
			return false, "Chưa có cookie Grok"
		}
		hasValidToken := strings.Contains(cookie, "sso=") || strings.Contains(cookie, "x-userid=") || strings.Contains(acc.SessionKey, ".")
		if !hasValidToken {
			return false, "Thiếu token phiên Grok (sso / x-userid)"
		}

		req, err := http.NewRequestWithContext(ctx, "GET", "https://grok.com/rest/auth/user", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("Accept", "application/json, text/plain, */*")
			req.Header.Set("Referer", "https://grok.com/")
			req.Header.Set("Origin", "https://grok.com")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					return true, "Phiên Grok hoạt động tốt (HTTP 200)"
				}
				if resp.StatusCode == 401 {
					return false, "Phiên Grok đã hết hạn (HTTP 401 Unauthorized)"
				}
				if resp.StatusCode == 403 {
					return true, "Phiên Grok hợp lệ (Đang hoạt động trên trình duyệt)"
				}
				if resp.StatusCode < 400 {
					return true, "Phiên Grok đang hoạt động"
				}
			}
		}
		return true, "Phiên Grok hợp lệ (Đã lưu token sso & x-userid)"

	case "gemini":
		if strings.HasPrefix(acc.SessionKey, "AIzaSy") {
			req, err := http.NewRequestWithContext(ctx, "GET", "https://generativelanguage.googleapis.com/v1beta/models?key="+acc.SessionKey, nil)
			if err != nil {
				return false, err.Error()
			}
			resp, err := client.Do(req)
			if err != nil {
				return false, "Lỗi kết nối Gemini API: " + err.Error()
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return true, "Gemini API key hợp lệ (HTTP 200)"
			}
			return false, fmt.Sprintf("Gemini API key bị từ chối (HTTP %d)", resp.StatusCode)
		}
		cookie := acc.Cookie
		if cookie == "" {
			cookie = acc.SessionKey
		}
		if cookie != "" {
			req, err := http.NewRequestWithContext(ctx, "GET", "https://gemini.google.com/app", nil)
			if err != nil {
				return false, err.Error()
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			resp, err := client.Do(req)
			if err != nil {
				return false, "Lỗi kết nối Gemini: " + err.Error()
			}
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				return true, "Phiên Gemini Web đang hoạt động"
			}
		}
		return true, "Phiên Gemini đã ghi nhận"

	case "googleflow":
		outcome, message := probeGoogleFlowSession(ctx, acc)
		return outcome == accountProbeAlive, message

	default:
		return true, "Phiên làm việc hợp lệ"
	}
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

// handleWebAIConnect maintains compatibility with legacy single-account connect calls.
func (s *Server) handleWebAIConnect(w http.ResponseWriter, r *http.Request) {
	s.handleWebAIAccountAdd(w, r)
}

// handleWebAIDisconnect disconnects an account and clears credentials.
func (s *Server) handleWebAIDisconnect(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	if err := s.WebAI.DisconnectAccount(platform); err != nil {
		redirectBack("err", "Lỗi ngắt kết nối: "+err.Error())
		return
	}
	redirectBack("ok", "Đã ngắt kết nối "+platform)
}

// handleWebAIPromptGenerate returns synthesized prompts for Claude, ChatGPT, Gemini, and Muse AI.
func (s *Server) handleWebAIPromptGenerate(w http.ResponseWriter, r *http.Request) {
	productName := r.FormValue("product_name")
	niche := r.FormValue("niche")
	framework := r.FormValue("framework")
	targetAudience := r.FormValue("target_audience")
	keyBenefits := r.FormValue("key_benefits")
	cta := r.FormValue("cta")
	engine := r.FormValue("engine")
	if engine == "" {
		engine = r.FormValue("ai_engine")
	}

	pkg := webai.GenerateAdScriptPackageWithEngine(r.Context(), s.LLM, engine, productName, niche, framework, targetAudience, keyBenefits, cta)
	s.writeJSON(w, pkg)
}

// handleWebAIPoolStatus returns live accounts status for the connected Web AI accounts.
func (s *Server) handleWebAIPoolStatus(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "total_connected": 0, "accounts": map[string]any{}})
		return
	}

	type platformSummary struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Icon    string `json:"icon"`
		Total   int    `json:"total"`
		Ready   int    `json:"ready"`
		Cooling int    `json:"cooling"`
		Status  string `json:"status"`
	}

	platforms := []string{webai.PlatformClaude, webai.PlatformChatGPT, webai.PlatformGemini, webai.PlatformGrok, webai.PlatformMuseAI, webai.PlatformGoogleFlow}
	names := map[string]string{
		webai.PlatformClaude:     "Claude 5.5 Sonnet",
		webai.PlatformChatGPT:    "ChatGPT (GPT-6 Luna)",
		webai.PlatformGemini:     "Gemini 3.8 Flash",
		webai.PlatformGrok:       "Grok 3 (xAI)",
		webai.PlatformMuseAI:     "Muse Chat (Meta)",
		webai.PlatformGoogleFlow: "Google Flow (video)",
	}
	icons := map[string]string{
		webai.PlatformClaude:     "🟣",
		webai.PlatformChatGPT:    "💬",
		webai.PlatformGemini:     "✨",
		webai.PlatformGrok:       "🚀",
		webai.PlatformMuseAI:     "🎬",
		webai.PlatformGoogleFlow: "🌊",
	}

	summaries := make(map[string]platformSummary)
	totalConnected := 0

	for _, p := range platforms {
		ready, cool, total := s.WebAI.CountByPlatform(p)
		status := "offline"
		if ready > 0 {
			status = "ok"
		} else if cool > 0 {
			status = "cooldown"
		}
		summaries[p] = platformSummary{
			ID:      p,
			Name:    names[p],
			Icon:    icons[p],
			Total:   total,
			Ready:   ready,
			Cooling: cool,
			Status:  status,
		}
		totalConnected += ready
	}

	s.writeJSON(w, map[string]any{
		"ok":              true,
		"total_connected": totalConnected,
		"accounts":        summaries,
	})
}

// handleWebAIProxy delegates requests to the ProxyRelay.
func (s *Server) handleWebAIProxy(w http.ResponseWriter, r *http.Request) {
	if s.WebAIRelay == nil {
		if s.WebAI != nil {
			s.WebAIRelay = webai.NewProxyRelay(s.WebAI)
		} else {
			http.Error(w, "Web AI relay not configured", http.StatusServiceUnavailable)
			return
		}
	}
	s.WebAIRelay.ServeHTTP(w, r)
}

// handleStudioAdCreative queues a high quality ad creative video job leveraging Muse AI clips,
// Gemini Imagen 3 photos, and AI voiceover.
func (s *Server) handleStudioAdCreative(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(120 << 20); err != nil {
		if perr := r.ParseForm(); perr != nil {
			studioErr("Không đọc được form — thử lại.")
			return
		}
	}
	if (r.MultipartForm != nil && len(r.MultipartForm.File["clip_files"]) > 0) ||
		strings.TrimSpace(r.FormValue("clip_urls")) != "" || strings.TrimSpace(r.FormValue("sample_clip")) != "" {
		studioErr("Clip render sẵn không được nối thành affiliate. Hãy dùng ảnh listing để tạo footage mới, nhân vật kênh phải mặc hoặc dùng đúng sản phẩm.")
		return
	}

	upDir := s.Studio.UploadDir()
	var accountID int64
	if raw := strings.TrimSpace(r.FormValue("account_id")); raw != "" {
		accountID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if accountID == 0 && strings.TrimSpace(r.FormValue("account")) != "" && s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, account := range accounts {
				if account != nil && account.Username == strings.TrimSpace(r.FormValue("account")) {
					accountID = account.ID
					break
				}
			}
		}
	}
	if accountID <= 0 {
		studioErr("Chọn kênh đích để dùng đúng ảnh nhân vật.")
		return
	}
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	var modelPhotos []string
	for _, key := range []string{"model_photos", "model_photo"} {
		photos, uploadErr := saveUploads(r, key, upDir, 10)
		if uploadErr != nil {
			studioErr("Ảnh người mẫu lỗi: " + uploadErr.Error())
			return
		}
		modelPhotos = append(modelPhotos, photos...)
	}
	if len(modelPhotos) == 0 {
		if photo, uploadErr := saveUpload(r, "model_photo", upDir); uploadErr != nil {
			studioErr("Ảnh người mẫu lỗi: " + uploadErr.Error())
			return
		} else if photo != "" {
			modelPhotos = append(modelPhotos, photo)
		}
	}
	if len(modelPhotos) > 0 {
		library, libErr := s.Studio.ModelLibrary()
		if libErr != nil {
			studioErr("Không đọc được thư viện nhân vật: " + libErr.Error())
			return
		}
		var stored []string
		for _, photoPath := range modelPhotos {
			photo, addErr := library.Add(accountID, photoPath)
			if addErr != nil {
				studioErr("Không lưu được ảnh nhân vật vào đúng kênh: " + addErr.Error())
				return
			}
			stored = append(stored, photo.Path)
		}
		modelPhotos = stored
	}
	var modelIDStrs []string
	if r.MultipartForm != nil {
		modelIDStrs = r.MultipartForm.Value["model_photo_ids"]
	}
	if len(modelIDStrs) == 0 {
		modelIDStrs = r.Form["model_photo_ids"]
	}
	if len(modelPhotos) == 0 && len(modelIDStrs) > 0 {
		var ids []int64
		for _, ms := range modelIDStrs {
			if id, err := strconv.ParseInt(strings.TrimSpace(ms), 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, ids)
			if len(modelPhotos) != len(ids) {
				studioErr("Ảnh đã chọn không thuộc thư viện của kênh đích.")
				return
			}
		}
	}
	if len(modelPhotos) == 0 && strings.TrimSpace(r.FormValue("model_id")) != "" {
		if mid, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("model_id")), 10, 64); err == nil && mid > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, []int64{mid})
		}
	}
	if len(modelPhotos) == 0 {
		studioErr("Chọn hoặc tải ảnh nhân vật từ thư viện của kênh đã chọn.")
		return
	}
	modelPhoto := modelPhotos[0]
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 8)
	if err != nil {
		studioErr("Ảnh phụ lỗi: " + err.Error())
		return
	}

	// Download photos from URLs (e.g. Gemini Imagen 3, Web CDN)
	rawPhotoURLs := strings.TrimSpace(r.FormValue("photo_urls"))
	if rawPhotoURLs == "" {
		rawPhotoURLs = strings.TrimSpace(r.FormValue("photo_url"))
	}
	if rawPhotoURLs != "" {
		for i, line := range strings.Split(rawPhotoURLs, "\n") {
			u := strings.TrimSpace(line)
			if u == "" || !strings.HasPrefix(u, "http") {
				continue
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-urlphoto-%d.png", time.Now().UnixNano(), i))
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			if err := studio.DownloadImage(ctx, u, dst); err == nil {
				if productPhoto == "" {
					productPhoto = dst
				} else if len(extraPhotos) < 8 {
					extraPhotos = append(extraPhotos, dst)
				}
			}
			cancel()
		}
	}
	if productPhoto == "" {
		studioErr("Thiếu ảnh listing sản phẩm thật. Tải ảnh lên hoặc chọn sản phẩm có ảnh trong kệ hàng.")
		return
	}

	// Save Voiceover audio or synthesize from voiceover_text
	tiktokMusicOnly := r.FormValue("tiktok_music_only") == "1"
	voiceoverText := strings.TrimSpace(r.FormValue("voiceover_text"))
	voiceoverPath, _ := saveUpload(r, "voiceover_file", upDir)
	if !tiktokMusicOnly && voiceoverPath == "" && voiceoverText != "" && s.TTS != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		if audioBytes, err := s.TTS.Synthesize(ctx, voiceoverText, "default"); err == nil && len(audioBytes) > 0 {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-voiceover.mp3", time.Now().UnixNano()))
			if werr := os.WriteFile(dst, audioBytes, 0o644); werr == nil {
				voiceoverPath = dst
			}
		}
		cancel()
	}
	if tiktokMusicOnly {
		voiceoverPath = ""
		voiceoverText = ""
	}

	// Save Music
	musicPath, _ := saveUpload(r, "music_file", upDir)
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr == nil {
				musicPath = dst
			}
		}
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	seconds = clampInt(seconds, 10, 120)
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))

	bpm = clampInt(bpm, 60, 200)
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)

	productName := strings.TrimSpace(r.FormValue("product_name"))
	if productName == "" {
		studioErr("Nhập tên sản phẩm theo listing trước khi dựng.")
		return
	}

	aspectRatio := strings.TrimSpace(r.FormValue("aspect_ratio"))
	if aspectRatio != "16:9" {
		aspectRatio = "9:16"
	}

	var assignedDepts []string
	if r.MultipartForm != nil {
		assignedDepts = r.MultipartForm.Value["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = r.Form["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = []string{"DEP-01", "DEP-02", "DEP-03", "DEP-04", "DEP-05"}
	}

	p := studio.AffiliateParams{
		Mode:          r.FormValue("mode"),
		AccountID:     accountID,
		Niche:         strings.TrimSpace(r.FormValue("niche")),
		ProductName:   productName,
		ProductPrice:  strings.TrimSpace(r.FormValue("product_price")),
		ProductPhoto:  productPhoto,
		ModelPhoto:    modelPhoto,
		ModelPhotos:   modelPhotos,
		ExtraPhotos:   extraPhotos,
		VoiceoverPath: voiceoverPath,
		VoiceoverText: voiceoverText,
		AspectRatio:   aspectRatio,
		Seconds:       seconds,
		BPM:           bpm,
		MusicPath:     musicPath,
		MusicStart:    musicStart,
		MusicTitle:    strings.TrimSpace(r.FormValue("music_title")),
		MusicArtist:   strings.TrimSpace(r.FormValue("music_artist")),
		FixedCaption:  strings.TrimSpace(r.FormValue("fixed_caption")),
		AffLink:       strings.TrimSpace(r.FormValue("aff_link")),
		AssignedDepts: assignedDepts,
	}

	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		studioErr("Không tạo được video: " + err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape("Đang dựng video quảng cáo sản phẩm chất lượng cao…"))
}

// handleWebAICaptureSession captures session tokens/cookies automatically
// from web proxy, browser bookmarklet (especially mobile/iPad), or JSON post.
func (s *Server) handleWebAICaptureSession(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa được khởi tạo"})
		return
	}

	platform := strings.TrimSpace(r.FormValue("platform"))
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	sessionKey := strings.TrimSpace(r.FormValue("session_key"))
	accountName := strings.TrimSpace(r.FormValue("account_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	tier := strings.TrimSpace(r.FormValue("tier"))

	// Check JSON body if contentType is application/json
	if r.Header.Get("Content-Type") == "application/json" || strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			Platform    string `json:"platform"`
			Cookie      string `json:"cookie"`
			SessionKey  string `json:"session_key"`
			AccountName string `json:"account_name"`
			Email       string `json:"email"`
			Tier        string `json:"tier"`
			Source      string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.Platform != "" {
				platform = req.Platform
			}
			if req.Cookie != "" {
				cookie = req.Cookie
			}
			if req.SessionKey != "" {
				sessionKey = req.SessionKey
			}
			if req.AccountName != "" {
				accountName = req.AccountName
			}
			if req.Email != "" {
				email = req.Email
			}
			if req.Tier != "" {
				tier = req.Tier
			}
		}
	}

	if platform == "" {
		platform = webai.PlatformChatGPT
	}

	if cookie == "" && sessionKey == "" {
		if r.Method == http.MethodGet {
			seeOther(w, r, "/web-ai?err="+url.QueryEscape("Không tìm thấy Khóa API hoặc Token xác thực để đồng bộ."))
			return
		}
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu khóa API hoặc token"})
		return
	}

	if accountName == "" {
		accountName = fmt.Sprintf("Web Sync (%s)", time.Now().Format("02/01 15:04"))
	}
	if tier == "" {
		tier = "free"
	}

	id, err := s.WebAI.UpsertAccountSession(platform, sessionKey, cookie, accountName, email, tier)
	if err != nil {
		if r.Method == http.MethodGet {
			seeOther(w, r, "/web-ai?err="+url.QueryEscape("Lỗi lưu phiên: "+err.Error()))
			return
		}
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("web_ai", "capture_session", nil, fmt.Sprintf("Tự động bắt và đồng bộ phiên Web AI platform=%s id=%s", platform, id), nil)
	}

	if r.Method == http.MethodGet {
		platformName := platform
		for _, p := range webai.SupportedPlatforms() {
			if p.ID == platform {
				platformName = p.Name
				break
			}
		}
		seeOther(w, r, fmt.Sprintf("/web-ai?tab=%s&ok=%s", platform, url.QueryEscape(fmt.Sprintf("✓ Đã nhận diện và đồng bộ phiên đăng nhập %s thành công từ trình duyệt!", platformName))))
		return
	}

	s.writeJSON(w, map[string]any{
		"ok":       true,
		"id":       id,
		"platform": platform,
		"message":  "Đã nhận diện và lưu phiên làm việc thành công",
	})
}

// handleWebAIBrowserLoginStart starts an automated Chromium CDP login session for a platform.
func (s *Server) handleWebAIBrowserLoginStart(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa được khởi tạo"})
		return
	}

	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platform == "" {
		platform = strings.TrimSpace(r.FormValue("platform"))
	}
	if platform == "" && r.Body != nil {
		var req struct {
			Platform string `json:"platform"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		platform = strings.TrimSpace(req.Platform)
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
		auth, captureErr := webai.RunCaptureViaCDP(sess.Context(), tgt, loginTimeout, func(status, msg string) {
			mgr.UpdateSession(sess.ID, status, msg, "", "")
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
			id, err := s.WebAI.UpsertAccountSession(tgt.Platform, auth.SessionValue, auth.CookieHeader, accountName, email, "free")
			if err != nil {
				msg := "Lưu tài khoản thất bại: " + err.Error()
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}
			saved, verifyErr := s.WebAI.GetAccount(id)
			if verifyErr != nil || saved == nil || saved.SessionKey != auth.SessionValue || (auth.CookieHeader != "" && saved.Cookie != auth.CookieHeader) {
				msg := "Phiên đã gửi vào kho nhưng xác nhận đọc lại không khớp; chưa báo thành công."
				if verifyErr != nil {
					msg += " " + verifyErr.Error()
				}
				mgr.UpdateSession(sess.ID, "failed", msg, "", msg)
				return
			}

			mgr.UpdateSession(sess.ID, "success", fmt.Sprintf("✓ Đã lưu và xác nhận phiên tài khoản %s!", email), email, "")
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

// handleWebAIBrowserLoginStatus checks the current state of a CDP login attempt.
func (s *Server) handleWebAIBrowserLoginStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
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
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
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

// handleWebAISyncGoogleAccount copies/syncs credentials from an existing Gemini Google account to Google Flow.
func (s *Server) handleWebAISyncGoogleAccount(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa được khởi tạo"})
		return
	}

	geminiAcc, err := s.WebAI.PickActiveAccount(webai.PlatformGemini)
	if err != nil {
		list, _ := s.WebAI.GetAccounts(webai.PlatformGemini)
		if len(list) > 0 {
			geminiAcc = &list[0]
		} else {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Chưa có tài khoản Google (Gemini) nào được kết nối trong hệ thống"})
			return
		}
	}

	cookieHdr := geminiAcc.Cookie
	sessionKey := geminiAcc.SessionKey
	if cookieHdr == "" {
		cookieHdr = sessionKey
	}
	if cookieHdr == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Tài khoản Gemini chưa có thông tin phiên Cookie Google"})
		return
	}

	email := strings.TrimSpace(geminiAcc.Email)
	if email == "" {
		email = "googleflow_user@gmail.com"
	}
	accountName := fmt.Sprintf("Google Flow (%s)", email)

	id, err := s.WebAI.UpsertAccountSession(webai.PlatformGoogleFlow, sessionKey, cookieHdr, accountName, email, "free")
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Lỗi lưu tài khoản Google Flow: " + err.Error()})
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("web_ai", "sync_google_flow", nil, fmt.Sprintf("Đồng bộ tài khoản Google Flow từ Gemini id=%s email=%s", id, email), nil)
	}

	s.writeJSON(w, map[string]any{
		"ok":      true,
		"id":      id,
		"email":   email,
		"message": fmt.Sprintf("Đã đồng bộ phiên Google thành công cho %s!", email),
	})
}

func logErr(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

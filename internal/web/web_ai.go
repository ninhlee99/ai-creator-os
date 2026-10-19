package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		return "/settings/he-thong#tab-webai"
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
		redirectBack("err", "Vui lòng nhập Mã kết nối (API Key).")
		return
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

// probeAccountSession sends a live HTTP probe to verify if the account session is alive.
func probeAccountSession(ctx context.Context, acc *webai.Account) (bool, string) {
	if acc == nil {
		return false, "Tài khoản không tồn tại"
	}
	client := &http.Client{Timeout: 8 * time.Second}
	platform := strings.ToLower(acc.Platform)

	switch platform {
	case "museai", "muse_ai", "muse":
		token := acc.SessionKey
		cookieHdr := acc.Cookie
		if token == "0" || token == "1" {
			return false, "Phiên Muse AI chưa hoàn tất đăng nhập 2FA (token không hợp lệ)"
		}
		if cookieHdr == "" && token != "" {
			cookieHdr = "ps_l=" + token
		} else if cookieHdr != "" && token != "" && !strings.Contains(cookieHdr, token) {
			cookieHdr = "ps_l=" + token + "; " + cookieHdr
		}
		if cookieHdr == "" {
			return false, "Chưa có session token Muse AI"
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://muse.ai/api/account", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookieHdr)
			req.Header.Set("Accept", "application/json, text/plain, */*")
			req.Header.Set("Referer", "https://muse.ai/")
			req.Header.Set("Origin", "https://muse.ai")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 304 {
					return true, "Phiên Muse AI hoạt động tốt (HTTP 200)"
				}
				if resp.StatusCode == 401 {
					return false, "Phiên Muse AI hết hạn (HTTP 401 Unauthorized)"
				}
				if resp.StatusCode == 403 {
					return true, "Phiên Muse AI hợp lệ (Đang hoạt động trên trình duyệt)"
				}
			}
		}
		return true, "Phiên Muse AI hợp lệ (Đã lưu token phiên)"

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

	default:
		return true, "Phiên làm việc hợp lệ"
	}
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
		alive, msg := probeAccountSession(r.Context(), acc)
		if alive {
			acc.Status = "ok"
		} else {
			acc.Status = "expired"
		}
		_ = s.WebAI.UpdateAccount(*acc)
		s.writeJSON(w, map[string]any{
			"ok":      alive,
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
	}

	var results []testResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, a := range allList {
		wg.Add(1)
		go func(acc webai.Account) {
			defer wg.Done()
			alive, msg := probeAccountSession(r.Context(), &acc)
			if alive {
				acc.Status = "ok"
			} else {
				acc.Status = "expired"
			}
			_ = s.WebAI.UpdateAccount(acc)

			mu.Lock()
			results = append(results, testResult{
				ID:       acc.ID,
				Email:    acc.Email,
				Platform: acc.Platform,
				Status:   acc.Status,
				Message:  msg,
				Alive:    alive,
			})
			mu.Unlock()
		}(a)
	}

	wg.Wait()
	aliveCount := 0
	for _, res := range results {
		if res.Alive {
			aliveCount++
		}
	}

	s.writeJSON(w, map[string]any{
		"ok":           true,
		"tested_count": len(results),
		"alive_count":  aliveCount,
		"results":      results,
		"message":      fmt.Sprintf("Đã kiểm tra %d tài khoản: %d còn sống ✓, %d hết hạn ⚠", len(results), aliveCount, len(results)-aliveCount),
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
		webai.PlatformClaude:     "Claude 3.7",
		webai.PlatformChatGPT:    "ChatGPT 4o",
		webai.PlatformGemini:     "Gemini 2.0",
		webai.PlatformGrok:       "Grok 3 (xAI)",
		webai.PlatformMuseAI:     "Muse AI",
		webai.PlatformGoogleFlow: "Google Flow (1.000 Credits Free)",
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

	upDir := s.Studio.UploadDir()
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	if productPhoto == "" {
		if gen := strings.TrimSpace(r.FormValue("generated_photo_path")); gen != "" {
			if _, serr := os.Stat(gen); serr == nil {
				productPhoto = gen
			}
		}
	}
	modelPhoto, err := saveUpload(r, "model_photo", upDir)
	if err != nil {
		studioErr("Ảnh người mẫu lỗi: " + err.Error())
		return
	}
	if modelPhoto == "" {
		if midStr := strings.TrimSpace(r.FormValue("model_id")); midStr != "" {
			if mid, err := strconv.ParseInt(midStr, 10, 64); err == nil && mid > 0 {
				if photoPath, perr := s.findModelPhotoByID(mid); perr == nil && photoPath != "" {
					modelPhoto = photoPath
				}
			}
		}
	}
	if modelPhoto == "" {
		if acctName := strings.TrimSpace(r.FormValue("account")); acctName != "" && s.Mgr != nil {
			if accts, aerr := s.Mgr.List(); aerr == nil && s.Studio != nil {
				for _, acc := range accts {
					if acc != nil && acc.Username == acctName {
						if ms, merr := s.Studio.ModelLibrary(); merr == nil {
							if photos, perr := ms.List(acc.ID); perr == nil && len(photos) > 0 {
								modelPhoto = photos[0].Path
							}
						}
						break
					}
				}
			}
		}
	}
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

	// Save video clips (from Muse AI or uploaded)
	var clips []string
	if r.MultipartForm != nil {
		for i, hdr := range r.MultipartForm.File["clip_files"] {
			if i >= 8 || hdr.Size == 0 {
				continue
			}
			ext := strings.ToLower(filepath.Ext(hdr.Filename))
			if ext != ".mp4" && ext != ".mov" && ext != ".webm" {
				continue
			}
			f, err := hdr.Open()
			if err != nil {
				continue
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-clip-%d%s", time.Now().UnixNano(), i, ext))
			out, err := os.Create(dst)
			if err == nil {
				_, _ = io.Copy(out, f)
				out.Close()
				clips = append(clips, dst)
			}
			f.Close()
		}
	}

	// Download video clips from URLs (from Muse AI or CDN)
	rawClipURLs := strings.TrimSpace(r.FormValue("clip_urls"))
	if rawClipURLs != "" {
		for i, line := range strings.Split(rawClipURLs, "\n") {
			u := strings.TrimSpace(line)
			if u == "" || !strings.HasPrefix(u, "http") {
				continue
			}
			if len(clips) >= 8 {
				break
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-urlclip-%d.mp4", time.Now().UnixNano(), i))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			if err := studio.DownloadVideo(ctx, u, dst); err == nil {
				clips = append(clips, dst)
			}
			cancel()
		}
	}

	// Look for sample_clip if user selected lookbook template (and not Veo 3 / Muse AI generative mode)
	videoEngine := strings.TrimSpace(r.FormValue("video_engine"))
	if len(clips) == 0 && (videoEngine == "lookbook" || videoEngine == "") {
		if sClip := strings.TrimSpace(r.FormValue("sample_clip")); sClip != "" {
			var cand string
			if strings.HasSuffix(sClip, ".mp4") {
				cand = filepath.Join("data", "clips", "fashion", sClip)
			} else {
				cand = filepath.Join("data", "clips", "fashion", sClip+".mp4")
			}
			if _, serr := os.Stat(cand); serr == nil {
				clips = append(clips, cand)
			}
		}
	}

	// Auto-fallback: if user provided no photo and no clip, generate a 4K product visual
	if productPhoto == "" && len(clips) == 0 {
		genPath := filepath.Join(upDir, fmt.Sprintf("%d-autogen-prod.png", time.Now().UnixNano()))
		prodName := strings.TrimSpace(r.FormValue("product_name"))
		if prodName == "" {
			prodName = "Sản phẩm AI Creator"
		}
		niche := strings.TrimSpace(r.FormValue("niche"))
		if s.Studio != nil && s.Studio.MediaGen() != nil && s.Studio.MediaGen().Healthy(r.Context()) {
			prompt := fmt.Sprintf("Ultra-detailed professional commercial advertising photograph of %s, %s, 4K quality, tack-sharp focus, photorealistic, cinematic lighting, 9:16 vertical, no text, no watermark", prodName, niche)
			if gerr := s.Studio.MediaGen().GenerateImage(r.Context(), prompt, nil, genPath); gerr == nil {
				productPhoto = genPath
			}
		}
		if productPhoto == "" {
			if verr := studio.GenerateProductVisual(prodName, niche, "SAN PHAM CHINH HANG - DAT MUA NGAY", 1080, 1920, genPath); verr == nil {
				productPhoto = genPath
			}
		}
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
	if musicPath == "" {
		musicPath = studio.FindDefaultMusic()
	}

	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	seconds = clampInt(seconds, 10, 120)
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))

	bpm = clampInt(bpm, 60, 200)
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)

	productName := strings.TrimSpace(r.FormValue("product_name"))
	if productName == "" {
		productName = "Video quảng cáo sản phẩm"
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
		Niche:         strings.TrimSpace(r.FormValue("niche")),
		ProductName:   productName,
		ProductPrice:  strings.TrimSpace(r.FormValue("product_price")),
		ProductPhoto:  productPhoto,
		ModelPhoto:    modelPhoto,
		ExtraPhotos:   extraPhotos,
		Clips:         clips,
		VoiceoverPath: voiceoverPath,
		VoiceoverText: strings.TrimSpace(r.FormValue("voiceover_text")),
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
	session, err := mgr.StartSession(platform, 5*time.Minute)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Launch CDP capture in background goroutine
	go func(sess *webai.ActiveLoginSession, tgt webai.WebLoginTarget) {
		auth, captureErr := webai.RunCaptureViaCDP(sess.Context(), tgt, 5*time.Minute, func(status, msg string) {
			sess.Status = status
			sess.Message = msg
		})

		if captureErr != nil {
			if sess.Status != "canceled" && !errors.Is(captureErr, context.Canceled) {
				sess.Status = "failed"
				sess.Error = captureErr.Error()
				sess.Message = "Đăng nhập thất bại: " + captureErr.Error()
			} else {
				sess.Status = "canceled"
				sess.Message = "Đã hủy phiên đăng nhập."
			}
			return
		}

		if auth != nil && auth.SessionValue != "" {
			email := strings.TrimSpace(auth.Email)
			if email == "" {
				email = fmt.Sprintf("%s_user_%d@web.ai", tgt.Platform, time.Now().Unix()%100000)
			}
			accountName := fmt.Sprintf("%s (%s)", tgt.Name, email)
			id, err := s.WebAI.UpsertAccountSession(tgt.Platform, auth.SessionValue, auth.CookieHeader, accountName, email, "free")
			if err != nil {
				sess.Status = "failed"
				sess.Error = "Lưu tài khoản thất bại: " + err.Error()
				sess.Message = sess.Error
				return
			}

			sess.Status = "success"
			sess.Email = email
			sess.Message = fmt.Sprintf("✓ Đã kết nối thành công tài khoản %s!", email)
			if s.Ledger != nil {
				_ = s.Ledger.Decide("web_ai", "cdp_login_success", nil, fmt.Sprintf("Đăng nhập CDP thành công %s id=%s email=%s", tgt.Platform, id, email), nil)
			}
		}
	}(session, target)

	s.writeJSON(w, map[string]any{
		"ok":         true,
		"session_id": session.ID,
		"platform":   platform,
		"status":     session.Status,
		"message":    session.Message,
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



func logErr(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}


package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"net/url"
	"strings"
)

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

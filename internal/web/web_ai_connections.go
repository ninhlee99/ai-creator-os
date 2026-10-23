package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"net/url"
	"strings"
)

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

// handleWebAIPoolStatus separates saved account state from Studio eligibility.
func (s *Server) handleWebAIPoolStatus(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "total_connected": 0, "active_accounts": 0, "studio_ready": 0, "accounts": map[string]any{}})
		return
	}

	type platformSummary struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Icon            string `json:"icon"`
		Total           int    `json:"total"`
		Active          int    `json:"active"`
		Ready           int    `json:"ready"`
		StudioReady     int    `json:"studio_ready"`
		Cooling         int    `json:"cooling"`
		Status          string `json:"status"`
		StudioSupported bool   `json:"studio_supported"`
	}

	platforms := []string{webai.PlatformClaude, webai.PlatformChatGPT, webai.PlatformGemini, webai.PlatformGrok, webai.PlatformMuseAI, webai.PlatformGoogleFlow}
	names := map[string]string{
		webai.PlatformClaude:     "Claude",
		webai.PlatformChatGPT:    "ChatGPT",
		webai.PlatformGemini:     "Gemini",
		webai.PlatformGrok:       "Grok",
		webai.PlatformMuseAI:     "Muse AI",
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
	activeAccounts := 0
	studioReady := 0

	for _, p := range platforms {
		ready, cool, total := s.WebAI.CountByPlatform(p)
		accounts, _ := s.WebAI.GetAccounts(p)
		active, hasSavedSession := 0, false
		for _, account := range accounts {
			if !account.IsActive {
				continue
			}
			active++
			if account.Status == "ok" || account.Status == "connected" {
				hasSavedSession = true
			}
		}
		status := "offline"
		if hasSavedSession {
			status = "ok"
		} else if cool > 0 {
			status = "cooldown"
		} else if active > 0 {
			status = "needs_attention"
		}
		studioSupported := webAIPlatformSupportsStudio(p)
		platformStudioReady := 0
		if studioSupported {
			platformStudioReady = ready
		}
		summaries[p] = platformSummary{
			ID:              p,
			Name:            names[p],
			Icon:            icons[p],
			Total:           total,
			Active:          active,
			Ready:           ready,
			StudioReady:     platformStudioReady,
			Cooling:         cool,
			Status:          status,
			StudioSupported: studioSupported,
		}
		totalConnected += ready
		activeAccounts += active
		studioReady += platformStudioReady
	}

	s.writeJSON(w, map[string]any{
		"ok":              true,
		"total_connected": totalConnected,
		"active_accounts": activeAccounts,
		"studio_ready":    studioReady,
		"accounts":        summaries,
	})
}

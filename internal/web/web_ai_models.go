package web

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// WebAIModelItem describes a specific model available on a web AI platform.
type WebAIModelItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tier        string `json:"tier"` // "free", "pro", "plus", "advanced", "max", "supergrok"
	IsFree      bool   `json:"is_free"`
	Badge       string `json:"badge,omitempty"`
}

// PlatformModelsInfo contains all available models for a web platform based on the user's tier.
type PlatformModelsInfo struct {
	Platform     string           `json:"platform"`
	PlatformName string           `json:"platform_name"`
	Icon         string           `json:"icon"`
	ActiveTier   string           `json:"active_tier"` // "free", "pro", "plus", "advanced", etc.
	TierLabel    string           `json:"tier_label"`
	AccountCount int              `json:"account_count"`
	AccountEmail string           `json:"account_email"`
	Models       []WebAIModelItem `json:"models"`
}

// GetModelsForPlatformAndTier returns the exact verified 2026 models based on platform and tier.
func GetModelsForPlatformAndTier(platform, tier string) []WebAIModelItem {
	platform = strings.ToLower(strings.TrimSpace(platform))
	tier = strings.ToLower(strings.TrimSpace(tier))
	isPaid := tier != "" && tier != "free" && tier != "unknown"

	switch platform {
	case "claude":
		if isPaid {
			return []WebAIModelItem{
				{ID: "claude-5-1-fable", Name: "Fable 5.1", Description: "For your toughest challenges · Pro/Max", Tier: "pro", IsFree: false, Badge: "Pro/Max"},
				{ID: "claude-5-5-opus", Name: "Opus 5.5", Description: "For complex work and everyday tasks · Pro", Tier: "pro", IsFree: false, Badge: "Pro"},
				{ID: "claude-5-5-sonet", Name: "Sonnet 5.5", Description: "Most efficient for simpler tasks", Tier: "free", IsFree: true},
				{ID: "claude-5-5-haiku", Name: "Haiku 5.5", Description: "Fastest for quick answers", Tier: "free", IsFree: true},
			}
		}
		return []WebAIModelItem{
			{ID: "claude-5-5-sonet", Name: "Sonnet 5.5", Description: "Most efficient for simpler tasks · Miễn phí", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "claude-5-5-haiku", Name: "Haiku 5.5", Description: "Fastest for quick answers · Miễn phí", Tier: "free", IsFree: true, Badge: "Free"},
		}

	case "gemini":
		if isPaid {
			argonBadge := "Advanced"
			if tier == "ultra" {
				argonBadge = "Ultra"
			}
			return []WebAIModelItem{
				{ID: "gemini-4-argon", Name: "4 Argon", Description: "Nhiệm vụ phức tạp · Gemini Ultra / Advanced", Tier: "ultra", IsFree: false, Badge: argonBadge},
				{ID: "gemini-3.1-pro", Name: "3.1 Pro", Description: "Suy luận nâng cao · Gemini Advanced", Tier: "advanced", IsFree: false, Badge: "Advanced"},
				{ID: "gemini-3.8-flash", Name: "3.8 Flash", Description: "Trợ giúp toàn diện", Tier: "free", IsFree: true},
				{ID: "gemini-3.5-flash-lite", Name: "3.5 Flash-Lite", Description: "Câu trả lời nhanh nhất", Tier: "free", IsFree: true},
				{ID: "imagen-3", Name: "Imagen 3", Description: "Tạo ảnh photorealistic từ prompt", Tier: "free", IsFree: true},
			}
		}
		return []WebAIModelItem{
			{ID: "gemini-3.8-flash", Name: "3.8 Flash", Description: "Trợ giúp toàn diện · Miễn phí", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "gemini-3.5-flash-lite", Name: "3.5 Flash-Lite", Description: "Câu trả lời nhanh nhất · Miễn phí", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "imagen-3", Name: "Imagen 3", Description: "Tạo ảnh photorealistic từ prompt", Tier: "free", IsFree: true, Badge: "Free"},
		}

	case "chatgpt":
		if isPaid {
			return []WebAIModelItem{
				{ID: "gpt-6-astra", Name: "GPT-6 Astra", Description: "Flagship Deep Reasoning & Multi-agent · Plus/Pro", Tier: "plus", IsFree: false, Badge: "Plus/Pro"},
				{ID: "gpt-6-luna", Name: "GPT-6 Luna", Description: "Phản hồi siêu tốc tức thì", Tier: "free", IsFree: true},
				{ID: "dall-e-3", Name: "DALL·E 3", Description: "Tạo và bóc tách ảnh trực tiếp", Tier: "free", IsFree: true},
			}
		}
		return []WebAIModelItem{
			{ID: "gpt-6-luna", Name: "GPT-6 Luna", Description: "Mặc định Miễn phí · Phản hồi siêu tốc", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "gpt-6-luna-think", Name: "GPT-6 Luna (Think)", Description: "Chế độ Think suy luận logic có giới hạn", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "dall-e-3", Name: "DALL·E 3", Description: "Tạo và bóc tách ảnh", Tier: "free", IsFree: true, Badge: "Free"},
		}

	case "grok":
		if isPaid {
			return []WebAIModelItem{
				{ID: "grok-4-7", Name: "Grok 4.7", Description: "Flagship 500k context & Lý luận sâu · Premium", Tier: "premium", IsFree: false, Badge: "SuperGrok"},
				{ID: "grok-4-think", Name: "Grok 4 Think", Description: "Tự sửa lỗi & Suy luận từng bước · Premium", Tier: "premium", IsFree: false, Badge: "SuperGrok"},
				{ID: "grok-4-deepsearch", Name: "Grok 4 DeepSearch", Description: "Tìm kiếm và tổng hợp sâu đa nguồn · Premium", Tier: "premium", IsFree: false, Badge: "SuperGrok"},
				{ID: "grok-4-fast", Name: "Grok 4 Fast", Description: "Chế độ Fast · Tốc độ phản hồi tức thì", Tier: "free", IsFree: true},
				{ID: "flux-1", Name: "Flux 1", Description: "Tạo ảnh nghệ thuật & chi tiết cao", Tier: "free", IsFree: true},
			}
		}
		return []WebAIModelItem{
			{ID: "grok-4-fast", Name: "Grok 4 Fast", Description: "Chế độ Fast · Tốc độ phản hồi tức thì · Miễn phí", Tier: "free", IsFree: true, Badge: "Free"},
			{ID: "flux-1", Name: "Flux 1", Description: "Tạo ảnh nghệ thuật & chi tiết cao", Tier: "free", IsFree: true, Badge: "Free"},
		}

	case "googleflow":
		return []WebAIModelItem{
			{ID: "veo-2", Name: "Google Veo 2", Description: "Mô hình tạo video 1080p chuẩn của Flow", Tier: "free", IsFree: true, Badge: "Flow"},
			{ID: "veo-3.1-fast", Name: "Google Veo 3.1 Fast", Description: "Tạo video tốc độ cao 9:16", Tier: "free", IsFree: true, Badge: "Flow"},
		}

	case "museai":
		return []WebAIModelItem{
			{ID: "muse-video", Name: "Muse AI Video Shot", Description: "Tạo cảnh video chuyển động đa góc", Tier: "free", IsFree: true, Badge: "Muse"},
			{ID: "muse-lipsync", Name: "Muse AI Lipsync", Description: "Đồng bộ khẩu hình nhân vật", Tier: "free", IsFree: true, Badge: "Muse"},
		}
	}
	return nil
}

func formatTierLabel(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "pro":
		return "Pro"
	case "plus":
		return "Plus"
	case "max":
		return "Max"
	case "ultra":
		return "Ultra"
	case "advanced":
		return "Advanced"
	case "premium", "supergrok":
		return "SuperGrok"
	case "team":
		return "Team"
	case "enterprise":
		return "Enterprise"
	default:
		return "Miễn phí"
	}
}

func extractChatGPTJWTTier(raw string) string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ' ' || r == '='
	})
	for _, p := range parts {
		if !strings.HasPrefix(p, "ey") {
			continue
		}
		segments := strings.Split(p, ".")
		if len(segments) < 2 {
			continue
		}
		payloadBytes, err := base64.RawURLEncoding.DecodeString(segments[1])
		if err != nil {
			payloadBytes, err = base64.URLEncoding.DecodeString(segments[1])
		}
		if err == nil && len(payloadBytes) > 0 {
			lower := strings.ToLower(string(payloadBytes))
			if strings.Contains(lower, `"plan_type":"pro"`) {
				return "pro"
			}
			if strings.Contains(lower, `"plan_type":"plus"`) || strings.Contains(lower, `"plan_type":"team"`) {
				return "plus"
			}
			if strings.Contains(lower, `"plan_type":"free"`) {
				return "free"
			}
		}
	}
	return ""
}

func probeLiveGeminiTier(ctx context.Context, cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gemini.google.com/app", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", cookie)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil || len(bodyBytes) == 0 {
		return ""
	}
	body := string(bodyBytes)

	if strings.Contains(body, "Ultra") || strings.Contains(body, "1.0 Ultra") || strings.Contains(body, "45781253") {
		return "ultra"
	}
	if strings.Contains(body, "Advanced") || strings.Contains(body, "one_ai_premium") {
		return "advanced"
	}
	return ""
}

// detectAccountTier inspects session credentials and metadata to auto-detect tier.
func detectAccountTier(ctx context.Context, acc *webai.Account) (string, string) {
	if acc == nil {
		return "free", "Miễn phí"
	}
	platform := strings.ToLower(strings.TrimSpace(acc.Platform))

	// If the user already confirmed a paid tier manually, honor it.
	currentTier := strings.ToLower(strings.TrimSpace(acc.Tier))
	if acc.TierConfirmed && currentTier != "" && currentTier != "free" && currentTier != "unknown" {
		return currentTier, formatTierLabel(currentTier)
	}

	cookie := strings.TrimSpace(acc.Cookie)
	if cookie == "" {
		for _, c := range acc.CookieJar {
			if cookie != "" {
				cookie += "; "
			}
			cookie += c.Name + "=" + c.Value
		}
	}

	switch platform {
	case "claude":
		lowerCookie := strings.ToLower(cookie)
		if strings.Contains(lowerCookie, "max") || strings.Contains(lowerCookie, "plan=max") || strings.Contains(lowerCookie, "tier=max") {
			return "max", "Max"
		}
		if strings.Contains(lowerCookie, "claude_pro") || strings.Contains(lowerCookie, "tier=pro") || strings.Contains(lowerCookie, "plan=pro") {
			return "pro", "Pro"
		}
		return "free", "Miễn phí"

	case "chatgpt":
		lowerCookie := strings.ToLower(cookie)
		if strings.Contains(lowerCookie, "plan=ultra") || strings.Contains(lowerCookie, "plan_type=ultra") {
			return "ultra", "Ultra"
		}
		if strings.Contains(lowerCookie, "plan=pro") || strings.Contains(lowerCookie, "plan_type=pro") {
			return "pro", "Pro"
		}
		if strings.Contains(lowerCookie, "plan=plus") || strings.Contains(lowerCookie, "plan_type=plus") {
			return "plus", "Plus"
		}
		if tierFromJWT := extractChatGPTJWTTier(cookie); tierFromJWT != "" {
			return tierFromJWT, formatTierLabel(tierFromJWT)
		}
		return "free", "Miễn phí"

	case "gemini":
		lowerCookie := strings.ToLower(cookie)
		if strings.Contains(lowerCookie, "ultra") || strings.Contains(lowerCookie, "gemini_ultra") || strings.Contains(lowerCookie, "one_ai_premium") {
			return "ultra", "Ultra"
		}
		if strings.Contains(lowerCookie, "advanced") {
			return "advanced", "Advanced"
		}
		if liveTier := probeLiveGeminiTier(ctx, cookie); liveTier != "" {
			return liveTier, formatTierLabel(liveTier)
		}
		return "free", "Miễn phí"

	case "grok":
		lowerCookie := strings.ToLower(cookie)
		if strings.Contains(lowerCookie, "ultra") || strings.Contains(lowerCookie, "supergrok") || strings.Contains(lowerCookie, "premium_plus") {
			return "ultra", "SuperGrok Ultra"
		}
		if strings.Contains(lowerCookie, "premium") {
			return "premium", "SuperGrok"
		}
		return "free", "Miễn phí"

	default:
		return "free", "Tiêu chuẩn"
	}
}

// handleWebAIModelsActive returns the active model catalog organized by platform and account tier.
func (s *Server) handleWebAIModelsActive(w http.ResponseWriter, _ *http.Request) {
	platformsInfo := s.compileActivePlatformModels()
	s.writeJSON(w, map[string]any{
		"ok":        true,
		"platforms": platformsInfo,
	})
}

// handleWebAIModelsSync auto-detects tiers for all connected accounts and syncs available models.
func (s *Server) handleWebAIModelsSync(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa sẵn sàng"})
		return
	}

	allAccsMap, err := s.WebAI.GetAllAccounts()
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Lỗi lấy danh sách tài khoản: " + err.Error()})
		return
	}

	var allList []webai.Account
	for _, list := range allAccsMap {
		allList = append(allList, list...)
	}

	var updatedCount int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, a := range allList {
		wg.Add(1)
		go func(acc webai.Account) {
			defer wg.Done()
			newTier, _ := detectAccountTier(r.Context(), &acc)
			if newTier != "" && newTier != acc.Tier {
				if err := s.WebAI.UpdateAccountTier(acc.ID, newTier, false); err == nil {
					mu.Lock()
					updatedCount++
					mu.Unlock()
				}
			}
		}(a)
	}
	wg.Wait()

	platformsInfo := s.compileActivePlatformModels()
	summaryMsg := fmt.Sprintf("Đã đồng bộ model Web AI thành công cho %d nền tảng!", len(platformsInfo))

	s.writeJSON(w, map[string]any{
		"ok":            true,
		"updated_count": updatedCount,
		"message":       summaryMsg,
		"platforms":     platformsInfo,
	})
}

// handleWebAIAccountSetTier manually sets or overrides an account's tier.
func (s *Server) handleWebAIAccountSetTier(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa sẵn sàng"})
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	tier := strings.ToLower(strings.TrimSpace(r.FormValue("tier")))

	if id == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu mã tài khoản (id)"})
		return
	}
	if tier == "" {
		tier = "free"
	}

	if err := s.WebAI.UpdateAccountTier(id, tier, true); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Lỗi cập nhật hạng tài khoản: " + err.Error()})
		return
	}

	platformsInfo := s.compileActivePlatformModels()
	s.writeJSON(w, map[string]any{
		"ok":        true,
		"message":   fmt.Sprintf("Đã cập nhật hạng tài khoản thành %s!", formatTierLabel(tier)),
		"platforms": platformsInfo,
	})
}

// compileActivePlatformModels builds the list of active platforms with their active tier and verified models.
func (s *Server) compileActivePlatformModels() map[string]PlatformModelsInfo {
	platformMeta := map[string]struct {
		name string
		icon string
	}{
		"chatgpt":    {name: "ChatGPT (chatgpt.com)", icon: "🤖"},
		"gemini":     {name: "Google Gemini (gemini.google.com)", icon: "✨"},
		"claude":     {name: "Anthropic Claude (claude.ai)", icon: "🧠"},
		"grok":       {name: "xAI Grok (grok.com)", icon: "⚡"},
		"googleflow": {name: "Google Flow Video (flow.google.com)", icon: "🎥"},
		"museai":     {name: "Muse AI Video (muse.ai)", icon: "🎬"},
	}

	accsMap := make(map[string][]webai.Account)
	if s.WebAI != nil {
		if all, err := s.WebAI.GetAllAccounts(); err == nil {
			accsMap = all
		}
	}

	res := make(map[string]PlatformModelsInfo)

	for pKey, meta := range platformMeta {
		accs := accsMap[pKey]
		var activeTier = "free"
		var firstEmail = ""

		// Check if any connected active account has a paid tier
		for _, acc := range accs {
			if firstEmail == "" && acc.Email != "" {
				firstEmail = acc.Email
			}
			t := strings.ToLower(strings.TrimSpace(acc.Tier))
			if t != "" && t != "free" && t != "unknown" {
				activeTier = t
				break
			}
		}

		models := GetModelsForPlatformAndTier(pKey, activeTier)

		res[pKey] = PlatformModelsInfo{
			Platform:     pKey,
			PlatformName: meta.name,
			Icon:         meta.icon,
			ActiveTier:   activeTier,
			TierLabel:    formatTierLabel(activeTier),
			AccountCount: len(accs),
			AccountEmail: firstEmail,
			Models:       models,
		}
	}

	return res
}

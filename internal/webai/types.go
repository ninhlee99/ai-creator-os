package webai

import "time"

// Platform constants for supported web AI services.
const (
	PlatformChatGPT = "chatgpt"
	PlatformClaude  = "claude"
	PlatformGemini  = "gemini"
	PlatformMuseAI  = "museai"
)

// Status constants for session connections.
const (
	StatusConnected    = "connected"
	StatusDisconnected = "disconnected"
	StatusExpired      = "expired"
)

// PlatformInfo describes a supported web AI service.
type PlatformInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Badge       string `json:"badge"`
	Description string `json:"description"`
	AuthGuide   string `json:"auth_guide"`
	Placeholder string `json:"placeholder"`
	Features    []string `json:"features"`
}

// SupportedPlatforms returns metadata for all 4 supported web platforms.
func SupportedPlatforms() []PlatformInfo {
	return []PlatformInfo{
		{
			ID:          PlatformChatGPT,
			Name:        "ChatGPT",
			URL:         "https://chatgpt.com",
			Icon:        "💬",
			Color:       "#10a37f",
			Badge:       "OpenAI",
			Description: "Kịch bản & Hook bán hàng",
			AuthGuide:   "F12 > Application > Cookies > accessToken hoặc session token.",
			Placeholder: "Dán token hoặc cookie...",
			Features:    []string{"Kịch bản bán hàng", "Hook 3 giây", "Lời thoại voiceover", "Phân tích khách hàng"},
		},
		{
			ID:          PlatformClaude,
			Name:        "Claude",
			URL:         "https://claude.ai",
			Icon:        "🟣",
			Color:       "#d97706",
			Badge:       "Anthropic",
			Description: "Phân cảnh & Chỉ đạo nghệ thuật",
			AuthGuide:   "F12 > Application > Cookies > sessionKey (sk-ant-sid01-...).",
			Placeholder: "Dán sessionKey...",
			Features:    []string{"Phân cảnh điện ảnh", "Prompt Gemini & Muse", "Tối ưu câu từ", "Caption & hashtag"},
		},
		{
			ID:          PlatformGemini,
			Name:        "Gemini",
			URL:         "https://gemini.google.com",
			Icon:        "✨",
			Color:       "#4285f4",
			Badge:       "Google",
			Description: "Tạo ảnh sản phẩm 4K",
			AuthGuide:   "F12 > Cookies > __Secure-1PSID.",
			Placeholder: "Dán cookie __Secure-1PSID...",
			Features:    []string{"Ảnh sản phẩm 4K", "Đồng nhất người mẫu", "Bối cảnh quảng cáo", "Miễn phí"},
		},
		{
			ID:          PlatformMuseAI,
			Name:        "Muse AI",
			URL:         "https://muse.ai",
			Icon:        "🎬",
			Color:       "#ec4899",
			Badge:       "Video Studio",
			Description: "Tạo video chuyển động",
			AuthGuide:   "F12 > Cookies > session cookie hoặc API key.",
			Placeholder: "Dán auth token hoặc cookie...",
			Features:    []string{"Video chuyển động", "Lưu trữ 1080p/4K", "Vòng lặp sản phẩm", "Không watermark"},
		},
	}
}

// Account stores credentials and connection state for an AI Web service.
type Account struct {
	ID                   string    `json:"id"`
	Platform             string    `json:"platform"`
	AccountName          string    `json:"account_name"`
	Email                string    `json:"email"`
	SessionKey           string    `json:"session_key"`
	Cookie               string    `json:"cookie"`
	Status               string    `json:"status"` // ok | cooldown | rate_limited | invalid | disconnected
	Tier                 string    `json:"tier"`   // free | plus | pro
	CooldownUntil        time.Time `json:"cooldown_until"`
	CooldownRemainingSec int64     `json:"cooldown_remaining_sec"`
	CooldownLabel        string    `json:"cooldown_label"`
	Requests             int       `json:"requests"`
	RateLimitHits        int       `json:"rate_limit_hits"`
	LastRateLimitLabel   string    `json:"last_rate_limit_label"`
	IsActive             bool      `json:"is_active"`
	LastUsed             time.Time `json:"last_used"`
	LastChecked          time.Time `json:"last_checked"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}


// AdScene represents a single scene in a product ad storyboard.
type AdScene struct {
	Index          int    `json:"index"`
	DurationSec    int    `json:"duration_sec"`
	SceneName      string `json:"scene_name"`
	VisualConcept  string `json:"visual_concept"`
	GeminiPrompt   string `json:"gemini_prompt"`
	MuseAIPrompt   string `json:"muse_ai_prompt"`
	VoiceoverText  string `json:"voiceover_text"`
	OnScreenText   string `json:"on_screen_text"`
	CameraMotion   string `json:"camera_motion"`
}

// HookVariant represents a targeted psychological hook variation for A/B testing.
type HookVariant struct {
	Type  string `json:"type"`  // "curiosity", "pain_point", "social_proof", "warning"
	Title string `json:"title"` // Display label (e.g., "Hook Tò Mò", "Hook Nỗi Đau")
	Text  string `json:"text"`  // The hook copy
	Angle string `json:"angle"` // Strategy explanation
}

// PlatformPost contains platform-tailored copywriting ready to publish.
type PlatformPost struct {
	Platform     string   `json:"platform"` // tiktok, youtube_shorts, facebook_reels, threads
	Title        string   `json:"title"`
	Caption      string   `json:"caption"`
	Hashtags     []string `json:"hashtags"`
	CallToAction string   `json:"call_to_action"`
	Tip          string   `json:"tip"`
}

// AdScriptPackage is a complete creative package for a product ad.
type AdScriptPackage struct {
	ProductName           string                  `json:"product_name"`
	Niche                 string                  `json:"niche"`
	Framework             string                  `json:"framework"` // problem_solution, three_reasons, transformation, unboxing, flash_sale
	HookText              string                  `json:"hook_text"`
	HookVariants          []HookVariant           `json:"hook_variants"`
	CallToAction          string                  `json:"call_to_action"`
	TotalDuration         int                     `json:"total_duration"`
	Scenes                []AdScene               `json:"scenes"`
	Caption               string                  `json:"caption"`
	Hashtags              []string                `json:"hashtags"`
	PlatformPosts         map[string]PlatformPost `json:"platform_posts"`
	PsychologicalTriggers []string                `json:"psychological_triggers"`
	MediaRetrievalGuides  map[string]string       `json:"media_retrieval_guides"`
	ClaudePrompt          string                  `json:"claude_prompt"`
	ChatGPTPrompt         string                  `json:"chatgpt_prompt"`
}


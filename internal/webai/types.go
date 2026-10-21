package webai

import "time"

// Platform constants for supported web AI services.
const (
	PlatformChatGPT    = "chatgpt"
	PlatformClaude     = "claude"
	PlatformGemini     = "gemini"
	PlatformGrok       = "grok"
	PlatformMuseAI     = "museai"
	PlatformGoogleFlow = "googleflow"
)

// Status constants for session connections.
const (
	StatusConnected    = "connected"
	StatusDisconnected = "disconnected"
	StatusExpired      = "expired"
	StatusUnverified   = "unknown"
)

// PlatformInfo describes a supported web AI service.
type PlatformInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	LoginURL    string   `json:"login_url"`
	Icon        string   `json:"icon"`
	Color       string   `json:"color"`
	Badge       string   `json:"badge"`
	Description string   `json:"description"`
	AuthGuide   string   `json:"auth_guide"`
	Placeholder string   `json:"placeholder"`
	Features    []string `json:"features"`
}

// SupportedPlatforms returns metadata for all supported web platforms.
func SupportedPlatforms() []PlatformInfo {
	return []PlatformInfo{
		{
			ID:          PlatformChatGPT,
			Name:        "ChatGPT",
			URL:         "https://chatgpt.com",
			LoginURL:    "https://chatgpt.com/auth/login",
			Icon:        "💬",
			Color:       "#10a37f",
			Badge:       "OpenAI",
			Description: "Kết nối OpenAI đã cấu hình",
			AuthGuide:   "Khóa API từ OpenAI (sk-...)",
			Placeholder: "Dán mã API Key (sk-...) vào đây...",
			Features:    nil,
		},
		{
			ID:          PlatformClaude,
			Name:        "Claude",
			URL:         "https://claude.ai",
			LoginURL:    "https://claude.ai/login",
			Icon:        "🟣",
			Color:       "#d97706",
			Badge:       "Anthropic",
			Description: "Phân cảnh điện ảnh & Kể chuyện cảm xúc (Claude)",
			AuthGuide:   "Khóa API từ Anthropic (sk-ant-...)",
			Placeholder: "Dán mã API Key (sk-ant-...) vào đây...",
			Features:    nil,
		},
		{
			ID:          PlatformGemini,
			Name:        "Gemini",
			URL:         "https://gemini.google.com",
			LoginURL:    "https://aistudio.google.com/app/apikey",
			Icon:        "✨",
			Color:       "#4285f4",
			Badge:       "Google",
			Description: "Kết nối Google AI đã cấu hình",
			AuthGuide:   "Khóa API Google AI Studio (AIzaSy...)",
			Placeholder: "Dán mã API Key (AIzaSy...) vào đây...",
			Features:    nil,
		},
		{
			ID:          PlatformGrok,
			Name:        "Grok",
			URL:         "https://grok.com",
			LoginURL:    "https://grok.com",
			Icon:        "🚀",
			Color:       "#111827",
			Badge:       "xAI",
			Description: "Kết nối xAI đã cấu hình",
			AuthGuide:   "Khóa API xAI Console (xai-...)",
			Placeholder: "Dán mã API Key (xai-...) vào đây...",
			Features:    nil,
		},
		{
			ID:          PlatformMuseAI,
			Name:        "Muse Chat (Meta)",
			URL:         "https://muse.ai",
			LoginURL:    "https://muse.ai/signin",
			Icon:        "🎬",
			Color:       "#ec4899",
			Badge:       "Tạo nội dung",
			Description: "Viết kịch bản, tạo ảnh và video trong Muse Chat",
			AuthGuide:   "Đăng nhập tài khoản Meta có quyền truy cập Muse Chat",
			Placeholder: "Dùng phiên Muse đã lưu; gọi gateway trực tiếp",
			Features:    nil,
		},
		{
			ID:          PlatformGoogleFlow,
			Name:        "Google Flow",
			URL:         "https://labs.google/fx/tools/flow",
			LoginURL:    "https://labs.google/fx/tools/flow",
			Icon:        "🌊",
			Color:       "#2563eb",
			Badge:       "Video · web API",
			Description: "Tạo và tải MP4 từ phiên Google Flow đã lưu",
			AuthGuide:   "Dùng cookie phiên Google Flow; quyền truy cập và credit tùy tài khoản",
			Placeholder: "Kết nối tài khoản Flow để gọi web API",
			Features:    nil,
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
	CreditsRemaining     int       `json:"credits_remaining"`
	CreditsTotal         int       `json:"credits_total"`
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
	Index         int    `json:"index"`
	DurationSec   int    `json:"duration_sec"`
	SceneName     string `json:"scene_name"`
	VisualConcept string `json:"visual_concept"`
	GeminiPrompt  string `json:"gemini_prompt"`
	MuseAIPrompt  string `json:"muse_ai_prompt"`
	VoiceoverText string `json:"voiceover_text"`
	OnScreenText  string `json:"on_screen_text"`
	CameraMotion  string `json:"camera_motion"`
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

// QCScorecard reports deterministic script-field checks, not rendered-video quality.
type QCScorecard struct {
	Scope            string   `json:"scope"`
	OverallScore     int      `json:"overall_score"`      // 0 - 100
	HookScore        int      `json:"hook_score"`         // 0 - 100
	NicheMatchScore  int      `json:"niche_match_score"`  // 0 - 100
	PersuasionScore  int      `json:"persuasion_score"`   // 0 - 100
	AudioVisualScore int      `json:"audio_visual_score"` // 0 - 100
	Verdict          string   `json:"verdict"`            // "ĐẠT CHUẨN XUẤT BẢN (A+)", "CHẤT LƯỢNG CAO (A)", etc.
	Summary          string   `json:"summary"`            // Detailed analysis summary
	Strengths        []string `json:"strengths"`          // Highlights
	Recommendations  []string `json:"recommendations"`    // Actionable improvements
	CheckedAt        string   `json:"checked_at"`
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
	QC                    QCScorecard             `json:"qc"`
	AIEngine              string                  `json:"ai_engine"`
	AIEngineBadge         string                  `json:"ai_engine_badge"`
}

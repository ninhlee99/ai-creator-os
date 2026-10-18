package webai

import "time"

// Platform constants for supported web AI services.
const (
	PlatformChatGPT = "chatgpt"
	PlatformClaude  = "claude"
	PlatformGemini  = "gemini"
	PlatformGrok    = "grok"
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
			Name:        "ChatGPT (GPT-4o / o3-mini)",
			URL:         "https://chatgpt.com",
			LoginURL:    "https://chatgpt.com/auth/login",
			Icon:        "💬",
			Color:       "#10a37f",
			Badge:       "OpenAI",
			Description: "Kịch bản viral, Hook bán hàng & FOMO (GPT-4o, o3-mini)",
			AuthGuide:   "Khóa API từ OpenAI (sk-...)",
			Placeholder: "Dán mã API Key (sk-...) vào đây...",
			Features:    []string{"Kịch bản bán hàng", "Hook 3 giây", "Lời thoại voiceover", "Phân tích khách hàng"},
		},
		{
			ID:          PlatformClaude,
			Name:        "Claude 3.7 Sonnet",
			URL:         "https://claude.ai",
			LoginURL:    "https://claude.ai/login",
			Icon:        "🟣",
			Color:       "#d97706",
			Badge:       "Anthropic",
			Description: "Phân cảnh điện ảnh & Kể chuyện cảm xúc (Claude 3.7 Sonnet)",
			AuthGuide:   "Khóa API từ Anthropic (sk-ant-...)",
			Placeholder: "Dán mã API Key (sk-ant-...) vào đây...",
			Features:    []string{"Phân cảnh điện ảnh", "Prompt Gemini & Muse", "Tối ưu câu từ", "Caption & hashtag"},
		},
		{
			ID:          PlatformGemini,
			Name:        "Gemini 2.0 Flash / Pro",
			URL:         "https://gemini.google.com",
			LoginURL:    "https://aistudio.google.com/app/apikey",
			Icon:        "✨",
			Color:       "#4285f4",
			Badge:       "Google",
			Description: "Tạo ảnh sản phẩm 4K & Kịch bản đa giác quan (Gemini 2.0 Flash)",
			AuthGuide:   "Khóa API Google AI Studio (Miễn phí 100%, AIzaSy...)",
			Placeholder: "Dán mã API Key (AIzaSy...) vào đây...",
			Features:    []string{"Ảnh sản phẩm 4K", "Đồng nhất người mẫu", "Bối cảnh quảng cáo", "Miễn phí 100%"},
		},
		{
			ID:          PlatformGrok,
			Name:        "Grok 3 (xAI)",
			URL:         "https://grok.com",
			LoginURL:    "https://grok.com",
			Icon:        "🚀",
			Color:       "#111827",
			Badge:       "xAI",
			Description: "Ý tưởng viral & Bùng nổ tranh luận (Grok 3)",
			AuthGuide:   "Khóa API xAI Console (xai-...)",
			Placeholder: "Dán mã API Key (xai-...) vào đây...",
			Features:    []string{"Bắt trend realtime X/Twitter", "Phản biện đa chiều", "Kịch bản tranh luận", "Tư duy sắc sảo"},
		},
		{
			ID:          PlatformMuseAI,
			Name:        "Muse AI",
			URL:         "https://muse.ai",
			LoginURL:    "https://muse.ai/signin",
			Icon:        "🎬",
			Color:       "#ec4899",
			Badge:       "Video Studio",
			Description: "Tạo video chuyển động",
			AuthGuide:   "Khóa API từ Muse.ai",
			Placeholder: "Dán mã API Key vào đây...",
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

// QCScorecard holds rigorous quality evaluation metrics for the script and video package.
type QCScorecard struct {
	OverallScore     int      `json:"overall_score"`      // 0 - 100
	HookScore        int      `json:"hook_score"`         // 0 - 100
	NicheMatchScore  int      `json:"niche_match_score"`   // 0 - 100
	PersuasionScore  int      `json:"persuasion_score"`   // 0 - 100
	AudioVisualScore int      `json:"audio_visual_score"`  // 0 - 100
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



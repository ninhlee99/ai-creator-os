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
			Name:        "ChatGPT Web",
			URL:         "https://chatgpt.com",
			Icon:        "💬",
			Color:       "#10a37f",
			Badge:       "OpenAI",
			Description: "Viết kịch bản video quảng cáo viral, đòn bẩy tâm lý mua hàng, hook 3 giây giữ chân và lời bình voiceover tự nhiên.",
			AuthGuide:   "Đăng nhập chatgpt.com trên trình duyệt, sao chép session token hoặc cookie accessToken từ DevTools (F12 > Application > Cookies).",
			Placeholder: "Dán accessToken hoặc session cookie vào đây...",
			Features:    []string{"Kịch bản bán hàng AIDA/PAS", "Tối ưu hóa Hook 3 giây", "Viết lời thoại lồng tiếng tự nhiên", "Phân tích đối tượng mục tiêu"},
		},
		{
			ID:          PlatformClaude,
			Name:        "Claude Web",
			URL:         "https://claude.ai",
			Icon:        "🟣",
			Color:       "#d97706",
			Badge:       "Anthropic",
			Description: "Đạo diễn kịch bản chiều sâu, chia phân cảnh chi tiết, viết prompt tạo hình ảnh và chuyển động video đạt chuẩn điện ảnh.",
			AuthGuide:   "Đăng nhập claude.ai, mở F12 > Application > Cookies > sao chép giá trị cookie 'sessionKey'.",
			Placeholder: "Dán sessionKey cookie (sk-ant-sid01-...) vào đây...",
			Features:    []string{"Đạo diễn phân cảnh điện ảnh", "Prompt Gemini & Muse AI chuyên sâu", "Tối ưu ngôn từ bán hàng tinh tế", "Viết caption & hashtag thịnh hành"},
		},
		{
			ID:          PlatformGemini,
			Name:        "Gemini Web",
			URL:         "https://gemini.google.com",
			Icon:        "✨",
			Color:       "#4285f4",
			Badge:       "Google",
			Description: "Tạo hình ảnh sản phẩm 4K siêu thực (Imagen 3), giữ nhất quán người mẫu/sản phẩm và gợi ý chuyển động video hoàn toàn miễn phí.",
			AuthGuide:   "Đăng nhập gemini.google.com, mở F12 > Cookies > sao chép cookie '__Secure-1PSID' hoặc token phiên làm việc.",
			Placeholder: "Dán cookie __Secure-1PSID hoặc token phiên Google vào đây...",
			Features:    []string{"Tạo ảnh sản phẩm 4K sắc nét", "Giữ đồng nhất người mẫu trong mọi cảnh", "Tạo bối cảnh chụp ảnh quảng cáo", "Miễn phí không tốn API key"},
		},
		{
			ID:          PlatformMuseAI,
			Name:        "Muse AI",
			URL:         "https://muse.ai",
			Icon:        "🎬",
			Color:       "#ec4899",
			Badge:       "Video Studio",
			Description: "Nền tảng tạo video AI chuyển động mượt mà, lưu trữ video chất lượng cao, trích xuất khoảnh khắc đắt giá và tạo hiệu ứng động cho sản phẩm.",
			AuthGuide:   "Đăng nhập muse.ai, mở cài đặt tài khoản hoặc F12 > Cookies > sao chép session cookie hoặc API key cá nhân.",
			Placeholder: "Dán auth token hoặc cookie phiên làm việc Muse AI...",
			Features:    []string{"Tạo video chuyển động từ prompt/ảnh", "Lưu trữ video 1080p/4K tốc độ cao", "Tạo vòng lặp chuyển động sản phẩm", "Xuất video không dính watermark"},
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


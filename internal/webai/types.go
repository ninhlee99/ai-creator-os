package webai

import (
	"fmt"
	"time"
)

// WebSessionAuthError marks a provider response that explicitly rejected the
// saved session. Callers can rotate accounts and prompt for reconnect.
type WebSessionAuthError struct {
	Platform   string
	StatusCode int
}

func (e *WebSessionAuthError) Error() string {
	return fmt.Sprintf("%s session rejected (HTTP %d)", e.Platform, e.StatusCode)
}

// Platform constants for supported web AI services.
const (
	PlatformChatGPT    = "chatgpt"
	PlatformClaude     = "claude"
	PlatformGemini     = "gemini"
	PlatformGrok       = "grok"
	PlatformMuseAI     = "museai"
	PlatformGoogleFlow = "googleflow"
	PlatformTikTok     = "tiktok"
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
			ID: PlatformChatGPT, Name: "ChatGPT", URL: "https://chatgpt.com",
			LoginURL: "https://chatgpt.com/auth/login", Icon: "💬", Color: "#10a37f",
			Badge: "OpenAI", Description: "Chat trên web · tải ảnh/video · tạo ảnh",
			AuthGuide: "Đăng nhập bằng trình duyệt trên máy tính", Placeholder: "",
			Features: []string{"Chat text và tải ảnh/video", "Tạo ảnh"},
		},
		{
			ID: PlatformClaude, Name: "Claude", URL: "https://claude.ai",
			LoginURL: "https://claude.ai/login", Icon: "🟣", Color: "#d97706",
			Badge: "Anthropic", Description: "Chat trên web · tải ảnh · phản hồi text",
			AuthGuide: "Đăng nhập bằng trình duyệt trên máy tính", Placeholder: "",
			Features: []string{"Chat text và tải ảnh"},
		},
		{
			ID: PlatformGemini, Name: "Gemini", URL: "https://gemini.google.com",
			LoginURL: "https://aistudio.google.com/app/apikey", Icon: "✨", Color: "#4285f4",
			Badge: "Google", Description: "Chat trên web · tải ảnh/video · tạo ảnh/video",
			AuthGuide: "Đăng nhập tài khoản Google bằng trình duyệt trên máy tính", Placeholder: "",
			Features: []string{"Chat text, tải ảnh/video", "Tạo ảnh/video theo quyền tài khoản"},
		},
		{
			ID: PlatformGrok, Name: "Grok", URL: "https://grok.com",
			LoginURL: "https://grok.com", Icon: "🚀", Color: "#111827",
			Badge: "xAI", Description: "Chat trên web · tải ảnh/video · tạo ảnh/video",
			AuthGuide: "Đăng nhập bằng trình duyệt trên máy tính", Placeholder: "",
			Features: []string{"Chat text, tải ảnh/video", "Tạo ảnh/video theo quyền tài khoản"},
		},
		{
			ID:          PlatformMuseAI,
			Name:        "Muse AI",
			URL:         "https://muse.ai",
			LoginURL:    "https://muse.ai/",
			Icon:        "🎬",
			Color:       "#ec4899",
			Badge:       "Tạo nội dung",
			Description: "Viết kịch bản, tạo ảnh và video trong Muse Chat",
			AuthGuide:   "Đăng nhập tài khoản Meta có quyền truy cập Muse Chat",
			Placeholder: "Dùng phiên Muse đã lưu; tạo video bằng Chrome/Edge ẩn",
			Features:    nil,
		},
		{
			ID:          PlatformGoogleFlow,
			Name:        "Google Flow",
			URL:         "https://flow.google.com/",
			LoginURL:    "https://flow.google.com/",
			Icon:        "🌊",
			Color:       "#2563eb",
			Badge:       "Video · web API",
			Description: "Tạo và tải MP4 từ phiên Google Flow đã lưu",
			AuthGuide:   "Dùng cookie phiên Google Flow; quyền truy cập và credit tùy tài khoản",
			Placeholder: "Kết nối tài khoản Flow để gọi web API",
			Features:    nil,
		},
		{
			ID:          PlatformTikTok,
			Name:        "TikTok",
			URL:         "https://www.tiktok.com",
			LoginURL:    "https://www.tiktok.com/login",
			Icon:        "🎵",
			Color:       "#fe2c55",
			Badge:       "Radar & Crawler",
			Description: "Đăng nhập TikTok lấy cookie duyệt video xu hướng không bị chặn bot",
			AuthGuide:   "Đăng nhập bằng mã QR hoặc tài khoản trên trình duyệt máy tính",
			Placeholder: "Lưu cookie session TikTok cho Crawler",
			Features:    []string{"Quét xu hướng TikTok Radar", "Vượt bảo vệ captcha / bot detection"},
		},
	}
}

// Account stores credentials and connection state for an AI Web service.
type Account struct {
	ID                   string          `json:"id"`
	Platform             string          `json:"platform"`
	AccountName          string          `json:"account_name"`
	Email                string          `json:"email"`
	SessionKey           string          `json:"session_key,omitempty"`
	Cookie               string          `json:"cookie,omitempty"`
	CookieJar            []BrowserCookie `json:"cookie_jar,omitempty"`
	Status               string          `json:"status"` // ok | cooldown | rate_limited | invalid | disconnected
	Tier                 string          `json:"tier"`   // provider plan, when known
	TierConfirmed        bool            `json:"-"`      // legacy database column; no longer gates saved sessions
	CreditsRemaining     int             `json:"credits_remaining"`
	CreditsTotal         int             `json:"credits_total"`
	CooldownUntil        time.Time       `json:"cooldown_until"`
	CooldownRemainingSec int64           `json:"cooldown_remaining_sec"`
	CooldownLabel        string          `json:"cooldown_label"`
	Requests             int             `json:"requests"`
	RateLimitHits        int             `json:"rate_limit_hits"`
	LastRateLimitLabel   string          `json:"last_rate_limit_label"`
	IsActive             bool            `json:"is_active"`
	LastUsed             time.Time       `json:"last_used"`
	LastChecked          time.Time       `json:"last_checked"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

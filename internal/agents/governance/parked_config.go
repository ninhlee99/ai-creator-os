//go:build parked

// parked_config.go — bản sao parked của internal/agents/config (đã xoá).
//
// Giữ lại trong vùng parked để 5 package agent (governance/hunter/analyst/content/streamer)
// vẫn build được khi bật tag -tags parked. Runtime chính KHÔNG dùng Config này:
// cấu hình thật duy nhất nằm ở ledger settings (xem docs/ARCHITECTURE.md §12).
//
// Central configuration: environment variables only, typed, read once.
// Every operational capability of the AI Creator OS reads its settings
// from here; nothing hardcodes credentials or tuning knobs.
package governance

import (
	"os"
	"strconv"
	"strings"
)

// Config mirrors the Python dataclass field for field. The zero value is
// NOT usable on its own — call Load() to read the environment.
type Config struct {
	AppEnv       string
	DryRun       bool
	DatabasePath string
	Timezone     string

	// LLM chain
	GeminiAPIKey  string
	OllamaBaseURL string
	OllamaModel   string

	// TTS / avatar
	TTSAPIKey      string
	AvatarProvider string
	AvatarAPIKey   string

	// TikTok
	TikTokRTMPURL    string
	TikTokRTMPKey    string
	AIDisclosureText string

	// Governance
	DailyAPIBudgetUSD        float64
	MaxLiveMinutesPerSession int
	KillSwitch               bool

	// Hunter / analyst tuning
	MinSellerRating     float64
	MaxPrice            float64 // VND
	KillViewsNoOrder    int
	KillSessionsNoOrder int

	// Alerting
	TelegramBotToken string
	TelegramChatID   string
}

func get(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

func getFloat(name string, def float64) float64 {
	raw := strings.TrimSpace(get(name, ""))
	if raw == "" {
		return def
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	return def
}

func getInt(name string, def int) int {
	raw := strings.TrimSpace(get(name, ""))
	if raw == "" {
		return def
	}
	if i, err := strconv.Atoi(raw); err == nil {
		return i
	}
	return def
}

func getBool(name string, def bool) bool {
	raw := strings.ToLower(strings.TrimSpace(get(name, "")))
	if raw == "" {
		return def
	}
	return raw == "1" || raw == "true" || raw == "yes"
}

// Load reads the full configuration from environment variables, applying
// the same defaults as the Python Config dataclass.
func Load() Config {
	return Config{
		AppEnv:       get("APP_ENV", "development"),
		DryRun:       getBool("DRY_RUN", true),
		DatabasePath: get("DATABASE_PATH", "./data/ledger.db"),
		Timezone:     get("TIMEZONE", "Asia/Ho_Chi_Minh"),

		GeminiAPIKey:  get("GEMINI_API_KEY", ""),
		OllamaBaseURL: get("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaModel:   get("OLLAMA_MODEL", "qwen3:4b"),

		TTSAPIKey:      get("TTS_API_KEY", ""),
		AvatarProvider: get("AVATAR_PROVIDER", "local-stylized"),
		AvatarAPIKey:   get("AVATAR_API_KEY", ""),

		TikTokRTMPURL:    get("TIKTOK_RTMP_URL", ""),
		TikTokRTMPKey:    get("TIKTOK_RTMP_KEY", ""),
		AIDisclosureText: get("AI_DISCLOSURE_TEXT", "AI-generated stream"),

		DailyAPIBudgetUSD:        getFloat("DAILY_API_BUDGET_USD", 5.0),
		MaxLiveMinutesPerSession: getInt("MAX_LIVE_MINUTES_PER_SESSION", 120),
		KillSwitch:               getBool("KILL_SWITCH", false),

		MinSellerRating:     getFloat("MIN_SELLER_RATING", 4.0),
		MaxPrice:            getFloat("MAX_PRICE", 1_000_000),
		KillViewsNoOrder:    getInt("KILL_VIEWS_NO_ORDER", 10_000),
		KillSessionsNoOrder: getInt("KILL_SESSIONS_NO_ORDER", 3),

		TelegramBotToken: get("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   get("TELEGRAM_CHAT_ID", ""),
	}
}

// LiveEnabled reports whether real (non-dry-run) live operations may run.
func (c Config) LiveEnabled() bool {
	return !c.DryRun && !c.KillSwitch
}

// ValidateForLive returns the list of blockers for going live.
// Empty means clear to go live.
func (c Config) ValidateForLive() []string {
	var blockers []string
	if c.KillSwitch {
		blockers = append(blockers, "KILL_SWITCH is on")
	}
	// Đợt H1: TikTok Shop đã loại bỏ (Accesstrade-only).
	if c.TikTokRTMPURL == "" || c.TikTokRTMPKey == "" {
		blockers = append(blockers, "missing TIKTOK_RTMP_URL/KEY")
	}
	if c.TTSAPIKey == "" {
		blockers = append(blockers, "missing TTS_API_KEY (Gemini; Edge fallback needs no key)")
	}
	return blockers
}

package web

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// Config is the Go port of apps/orchestrator/config.py: env vars only,
// typed, validated once. dryRun/killSwitch are runtime-mutable (the
// dashboard toggles them) so they are atomic; everything else is
// read-once at startup. This Config is the SINGLE source of truth for
// kill/dry-run: cmd wiring passes it to the network daemon as its
// network.Gate, so every layer observes the same switches live.
type Config struct {
	AppEnv       string
	DatabasePath string
	Timezone     string
	// Version là chuỗi đóng dấu lúc build (ldflags -X main.version=...),
	// mặc định "dev". Hiện ở Cài đặt · Hệ thống (R2-W7).
	Version string

	// LLM chain
	GeminiAPIKey  string   // legacy single key / first key (backward compat)
	GeminiAPIKeys []string // key rotation: GEMINI_API_KEYS (comma-separated) wins,
	// legacy GEMINI_API_KEY is the single-key fallback
	OllamaBaseURL string
	OllamaModel   string

	// TTS (avatar đã park — PIVOT 2026-10-02)
	TTSAPIKey string

	// TikTok (RTMP live đã park — giữ field cho tương thích cấu hình cũ)
	TiktokRTMPURL    string
	TiktokRTMPKey    string
	AIDisclosureText string

	// governance
	DailyAPIBudgetUSD        float64
	MaxLiveMinutesPerSession int

	// hunter / analyst tuning
	MinSellerRating     float64
	MaxPrice            float64
	KillViewsNoOrder    int
	KillSessionsNoOrder int

	// alerting
	TelegramBotToken string
	TelegramChatID   string

	dryRun     atomic.Bool
	killSwitch atomic.Bool
}

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func getenvFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

func getenvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

// getenvList reads a comma-separated env var into trimmed non-empty items.
// When the primary var is empty, each fallback var is tried in order (also
// parsed as a list, so a single-key fallback just yields one item).
func getenvList(name string, fallbacks ...string) []string {
	parse := func(v string) []string {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	if out := parse(os.Getenv(name)); len(out) > 0 {
		return out
	}
	for _, fb := range fallbacks {
		if out := parse(os.Getenv(fb)); len(out) > 0 {
			return out
		}
	}
	return nil
}

// LoadConfig reads the configuration from the environment, mirroring
// config.py defaults.
func LoadConfig() *Config {
	c := &Config{
		AppEnv:       getenv("APP_ENV", "development"),
		DatabasePath: getenv("DATABASE_PATH", "./data/ledger.db"),
		Timezone:     getenv("TIMEZONE", "Asia/Ho_Chi_Minh"),

		GeminiAPIKey:  getenv("GEMINI_API_KEY", ""),
		GeminiAPIKeys: getenvList("GEMINI_API_KEYS", "GEMINI_API_KEY"),
		OllamaBaseURL: getenv("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaModel:   getenv("OLLAMA_MODEL", "qwen3:4b"),

		TTSAPIKey: getenv("TTS_API_KEY", ""),

		TiktokRTMPURL:    getenv("TIKTOK_RTMP_URL", ""),
		TiktokRTMPKey:    getenv("TIKTOK_RTMP_KEY", ""),
		AIDisclosureText: getenv("AI_DISCLOSURE_TEXT", "AI-generated stream"),

		DailyAPIBudgetUSD:        getenvFloat("DAILY_API_BUDGET_USD", 5.0),
		MaxLiveMinutesPerSession: getenvInt("MAX_LIVE_MINUTES_PER_SESSION", 120),

		MinSellerRating:     getenvFloat("MIN_SELLER_RATING", 4.0),
		MaxPrice:            getenvFloat("MAX_PRICE", 1_000_000),
		KillViewsNoOrder:    getenvInt("KILL_VIEWS_NO_ORDER", 10_000),
		KillSessionsNoOrder: getenvInt("KILL_SESSIONS_NO_ORDER", 3),

		TelegramBotToken: getenv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getenv("TELEGRAM_CHAT_ID", ""),
	}
	c.dryRun.Store(getenvBool("DRY_RUN", true))
	c.killSwitch.Store(getenvBool("KILL_SWITCH", false))
	return c
}

// RefreshEnv re-reads the UI-editable env vars into the Config fields
// that were read once at startup (R2-W4, R2-05). The Settings env-save
// handler calls this right after persisting, so the runtime Config
// reflects UI edits without a restart. Fields with no UI-editable env
// counterpart are untouched.
func (c *Config) RefreshEnv() {
	if c == nil {
		return
	}
	c.TTSAPIKey = os.Getenv("TTS_API_KEY")
}

// DryRun reports whether the system is in dry-run (safe) mode.
func (c *Config) DryRun() bool { return c.dryRun.Load() }

// SetDryRun toggles dry-run mode at runtime (dashboard control).
func (c *Config) SetDryRun(v bool) { c.dryRun.Store(v) }

// KillSwitch reports whether the kill switch is engaged.
func (c *Config) KillSwitch() bool { return c.killSwitch.Load() }

// SetKillSwitch engages/releases the kill switch at runtime.
func (c *Config) SetKillSwitch(v bool) { c.killSwitch.Store(v) }

// LiveEnabled mirrors config.py's live_enabled: nothing automated runs
// while dry-run is on or the kill switch is engaged.
func (c *Config) LiveEnabled() bool { return !c.DryRun() && !c.KillSwitch() }

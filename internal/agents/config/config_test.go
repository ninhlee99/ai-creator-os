package config

import (
	"os"
	"testing"
)

func setenv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv(k, v)
}

// unsetEnvs removes the vars entirely (t.Setenv cannot unset). os.LookupEnv
// distinguishes unset from empty, so defaults only apply when unset —
// matching Python's os.environ.get(name, default).
func unsetEnvs(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			if err := os.Unsetenv(k); err != nil {
				t.Fatal(err)
			}
			vv, kk := v, k
			t.Cleanup(func() { os.Setenv(kk, vv) })
		}
	}
}

var allKeys = []string{
	"APP_ENV", "DRY_RUN", "DATABASE_PATH", "TIMEZONE",
	"GEMINI_API_KEY", "OLLAMA_BASE_URL", "OLLAMA_MODEL",
	"TTS_API_KEY", "AVATAR_PROVIDER", "AVATAR_API_KEY",
	"TIKTOK_SHOP_APP_KEY", "TIKTOK_SHOP_APP_SECRET",
	"TIKTOK_SHOP_ACCESS_TOKEN", "TIKTOK_SHOP_CIPHER",
	"TIKTOK_RTMP_URL", "TIKTOK_RTMP_KEY", "AI_DISCLOSURE_TEXT",
	"DAILY_API_BUDGET_USD", "MAX_LIVE_MINUTES_PER_SESSION", "KILL_SWITCH",
	"MIN_SELLER_RATING", "MAX_PRICE", "KILL_VIEWS_NO_ORDER",
	"KILL_SESSIONS_NO_ORDER", "TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID",
}

func TestDefaults(t *testing.T) {
	unsetEnvs(t, allKeys...)
	c := Load()
	if c.AppEnv != "development" {
		t.Errorf("AppEnv = %q", c.AppEnv)
	}
	if !c.DryRun {
		t.Error("DryRun default must be true")
	}
	if c.DatabasePath != "./data/ledger.db" {
		t.Errorf("DatabasePath = %q", c.DatabasePath)
	}
	if c.Timezone != "Asia/Ho_Chi_Minh" {
		t.Errorf("Timezone = %q", c.Timezone)
	}
	if c.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("OllamaBaseURL = %q", c.OllamaBaseURL)
	}
	if c.OllamaModel != "qwen3:4b" {
		t.Errorf("OllamaModel = %q", c.OllamaModel)
	}
	if c.AvatarProvider != "local-stylized" {
		t.Errorf("AvatarProvider = %q", c.AvatarProvider)
	}
	if c.AIDisclosureText != "AI-generated stream" {
		t.Errorf("AIDisclosureText = %q", c.AIDisclosureText)
	}
	if c.DailyAPIBudgetUSD != 5.0 {
		t.Errorf("DailyAPIBudgetUSD = %v", c.DailyAPIBudgetUSD)
	}
	if c.MaxLiveMinutesPerSession != 120 {
		t.Errorf("MaxLiveMinutesPerSession = %v", c.MaxLiveMinutesPerSession)
	}
	if c.KillSwitch {
		t.Error("KillSwitch default must be false")
	}
	if c.MinSellerRating != 4.0 || c.MaxPrice != 1_000_000 {
		t.Errorf("tuning = %v / %v", c.MinSellerRating, c.MaxPrice)
	}
	if c.KillViewsNoOrder != 10_000 || c.KillSessionsNoOrder != 3 {
		t.Errorf("kill thresholds = %v / %v", c.KillViewsNoOrder, c.KillSessionsNoOrder)
	}
}

func TestEnvOverrides(t *testing.T) {
	unsetEnvs(t, allKeys...)
	setenv(t, "APP_ENV", "production")
	setenv(t, "DRY_RUN", "false")
	setenv(t, "DAILY_API_BUDGET_USD", "12.5")
	setenv(t, "MAX_LIVE_MINUTES_PER_SESSION", "45")
	setenv(t, "KILL_SWITCH", "1")
	setenv(t, "GEMINI_API_KEY", "gk")
	c := Load()
	if c.AppEnv != "production" || c.DryRun {
		t.Errorf("got env=%q dry=%v", c.AppEnv, c.DryRun)
	}
	if c.DailyAPIBudgetUSD != 12.5 || c.MaxLiveMinutesPerSession != 45 || !c.KillSwitch {
		t.Errorf("got budget=%v maxmin=%v kill=%v", c.DailyAPIBudgetUSD, c.MaxLiveMinutesPerSession, c.KillSwitch)
	}
	if c.GeminiAPIKey != "gk" {
		t.Errorf("GeminiAPIKey = %q", c.GeminiAPIKey)
	}
}

func TestEnvBadValuesFallBackToDefaults(t *testing.T) {
	unsetEnvs(t, allKeys...)
	setenv(t, "DAILY_API_BUDGET_USD", "not-a-number")
	setenv(t, "MAX_LIVE_MINUTES_PER_SESSION", "nope")
	setenv(t, "DRY_RUN", "maybe")
	c := Load()
	if c.DailyAPIBudgetUSD != 5.0 {
		t.Errorf("budget = %v", c.DailyAPIBudgetUSD)
	}
	if c.MaxLiveMinutesPerSession != 120 {
		t.Errorf("maxmin = %v", c.MaxLiveMinutesPerSession)
	}
	// Unknown bool strings fall back to false, exactly like Python's
	// _get_bool ("maybe" is not in ("1","true","yes")).
}

func TestLiveEnabled(t *testing.T) {
	c := Config{DryRun: false}
	if !c.LiveEnabled() {
		t.Error("expected live enabled")
	}
	c.DryRun = true
	if c.LiveEnabled() {
		t.Error("dry-run must disable live")
	}
	c = Config{DryRun: false, KillSwitch: true}
	if c.LiveEnabled() {
		t.Error("kill switch must disable live")
	}
}

func TestValidateForLive(t *testing.T) {
	c := Config{DryRun: false}
	b := c.ValidateForLive()
	if len(b) == 0 {
		t.Fatal("expected blockers with empty credentials")
	}
	c.TikTokShopAccessToken = "tok"
	c.TikTokRTMPURL = "rtmp://x"
	c.TikTokRTMPKey = "k"
	c.TTSAPIKey = "tts"
	if got := c.ValidateForLive(); len(got) != 0 {
		t.Errorf("expected clear, got %v", got)
	}
	c.KillSwitch = true
	if got := c.ValidateForLive(); len(got) != 1 || got[0] != "KILL_SWITCH is on" {
		t.Errorf("kill switch blocker = %v", got)
	}
}

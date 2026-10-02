package web

import (
	"net/url"
	"os"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

// TestEnvSaveRefreshesRuntimeConfig — "sửa env qua UI → Cfg runtime đổi
// theo" (nghiệm thu R2-W4 bắt buộc, R2-05): giá trị lưu qua
// POST /settings/env phải hiện ngay trong web.Config, không cần restart.
func TestEnvSaveRefreshesRuntimeConfig(t *testing.T) {
	s := newTestServer(t)
	// Known starting point, independent of the developer's machine env.
	old, hadOld := os.LookupEnv("TTS_API_KEY")
	s.Cfg.TTSAPIKey = "old-key"
	t.Cleanup(func() {
		if hadOld {
			os.Setenv("TTS_API_KEY", old)
		} else {
			os.Unsetenv("TTS_API_KEY")
		}
	})

	rec := postForm(t, s, "/settings/env", url.Values{
		"name":  {"TTS_API_KEY"},
		"value": {"new-key-123"},
	})
	if rec.Code != 303 {
		t.Fatalf("POST /settings/env code = %d, want 303", rec.Code)
	}
	// The runtime Config must reflect the UI edit immediately.
	if s.Cfg.TTSAPIKey != "new-key-123" {
		t.Errorf("Cfg.TTSAPIKey = %q after UI save, want the new value (stale config)", s.Cfg.TTSAPIKey)
	}
	// And the effective env (the automation layer's read path) agrees.
	if v, _ := s.EffectiveEnv("TTS_API_KEY"); v != "new-key-123" {
		t.Errorf("EffectiveEnv(TTS_API_KEY) = %q, want new-key-123", v)
	}
	// Clearing through the UI clears the runtime Config too.
	postForm(t, s, "/settings/env", url.Values{
		"name":  {"TTS_API_KEY"},
		"value": {""},
	})
	if s.Cfg.TTSAPIKey != "" {
		t.Errorf("Cfg.TTSAPIKey = %q after UI clear, want empty", s.Cfg.TTSAPIKey)
	}
}

// TestMasterSwitchToggleThroughUI — công tắc chính bật/tắt qua
// POST /settings/master và đọc live được ngay (không restart).
func TestMasterSwitchToggleThroughUI(t *testing.T) {
	s := newTestServer(t)
	if automation.MasterOn(s.settings()) {
		t.Fatal("master starts ON, want the persisted OFF default")
	}
	rec := postForm(t, s, "/settings/master", url.Values{"value": {"1"}})
	if rec.Code != 303 {
		t.Fatalf("POST /settings/master code = %d, want 303", rec.Code)
	}
	if !automation.MasterOn(s.settings()) {
		t.Error("master not ON after UI toggle to 1")
	}
	// The daemon gate reads the same value live.
	gate := automation.MasterSwitchGate{Settings: s.settings()}
	if !gate.MasterEnabled() {
		t.Error("daemon gate not enabled right after the UI toggle (restart must not be needed)")
	}
	rec = postForm(t, s, "/settings/master", url.Values{"value": {"0"}})
	if rec.Code != 303 {
		t.Fatalf("POST /settings/master off code = %d, want 303", rec.Code)
	}
	if automation.MasterOn(s.settings()) {
		t.Error("master still ON after UI toggle to 0")
	}
}

// TestAPIBudgetThroughUI — ngân sách API lưu qua UI thắng giá trị env
// mặc định; giá trị hỏng bị từ chối với ?err=.
func TestAPIBudgetThroughUI(t *testing.T) {
	s := newTestServer(t)
	if got := automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD); got != s.Cfg.DailyAPIBudgetUSD {
		t.Fatalf("budget = %v before save, want the env default %v", got, s.Cfg.DailyAPIBudgetUSD)
	}
	rec := postForm(t, s, "/settings/api-budget", url.Values{"value": {"7.5"}})
	if rec.Code != 303 {
		t.Fatalf("POST /settings/api-budget code = %d, want 303", rec.Code)
	}
	if got := automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD); got != 7.5 {
		t.Errorf("budget = %v after UI save, want 7.5", got)
	}
	// Invalid input is rejected with an error redirect, never stored.
	rec = postForm(t, s, "/settings/api-budget", url.Values{"value": {"khong-phai-so"}})
	if rec.Code != 303 {
		t.Fatalf("invalid budget code = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !containsQuery(loc, "err=") {
		t.Errorf("invalid budget redirect = %q, want ?err=", loc)
	}
	if got := automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD); got != 7.5 {
		t.Errorf("budget = %v after invalid save, want the previous 7.5 kept", got)
	}
}

func containsQuery(loc, q string) bool {
	for i := 0; i+len(q) <= len(loc); i++ {
		if loc[i:i+len(q)] == q {
			return true
		}
	}
	return false
}

// TestGrowthThresholdsThroughUI — ngưỡng growth lưu qua /growth
// ảnh hưởng vòng sync (LoadThresholds), và nút reset về mặc định.
func TestGrowthThresholdsThroughUI(t *testing.T) {
	s, _, _ := newGrowthServer(t)
	rec := postForm(t, s, "/growth/thresholds", url.Values{
		"kill_min_videos": {"5"},
		"kill_min_days":   {"10"},
	})
	if rec.Code != 200 {
		t.Fatalf("POST /growth/thresholds code = %d, want 200", rec.Code)
	}
	got := automation.LoadThresholds(s.settings())
	if got.KillMinVideos != 5 || got.KillMinDays != 10 {
		t.Errorf("thresholds = %+v after UI save, want kill 5/10", got)
	}
	rec = postForm(t, s, "/growth/thresholds", url.Values{"reset": {"1"}})
	if rec.Code != 200 {
		t.Fatalf("POST /growth/thresholds reset code = %d, want 200", rec.Code)
	}
	def := growth.DefaultConfig()
	if got := automation.LoadThresholds(s.settings()); got != def {
		t.Errorf("thresholds after reset = %+v, want defaults", got)
	}
}

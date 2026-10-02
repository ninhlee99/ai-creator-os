package automation

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

var errNoLedger = errors.New("automation: no ledger wired")

// Settings is the single configuration facade of the app (R2-W4, R2-05).
// Everything the UI or the automation layer reads/writes as a setting
// goes through this interface, persisted in the ledger database — the
// products.db settings table and the read-once web.Config fields are no
// longer control planes (see MigrateProductsSettings and
// (*web.Config).RefreshEnv).
type Settings interface {
	// Get returns the stored value; ok=false when the key was never set.
	Get(key string) (string, bool)
	// Set persists the value.
	Set(key, value string) error
}

// Setting keys (all live in the ledger settings table).
const (
	// KeyMasterSwitch is the persisted network-daemon on/off button
	// ("1"/"0"). Default OFF. Seeded once from MASTER_SWITCH env.
	KeyMasterSwitch = "master.switch"
	// KeyGrowthThresholds holds the growth governance thresholds as JSON
	// (a growth.Config). Empty = all defaults from growth.DefaultConfig().
	KeyGrowthThresholds = "growth.thresholds"
	// KeyAPIBudgetUSD is the daily API spend ceiling in USD, editable in
	// Settings. Empty = the DAILY_API_BUDGET_USD env default ($5).
	KeyAPIBudgetUSD = "ops.api_budget_usd"
	// KeyProductsSettingsMigrated marks the one-time import of the legacy
	// products.db settings into this facade.
	KeyProductsSettingsMigrated = "migration.products_settings_v1"
	// KeyAPIEnabled is the legacy /api/* JSON API switch ("1"/"0").
	// Default OFF (R2-W7, R2-09): nothing in the UI uses it, so it stays
	// closed unless the operator explicitly opens it.
	KeyAPIEnabled = "api.enabled"
)

// LedgerSettings adapts *ledger.Ledger to the Settings facade.
type LedgerSettings struct{ L *ledger.Ledger }

func (s LedgerSettings) Get(key string) (string, bool) {
	if s.L == nil {
		return "", false
	}
	v, ok, err := s.L.GetSetting(key)
	if err != nil || !ok {
		return "", false
	}
	return v, true
}

func (s LedgerSettings) Set(key, value string) error {
	if s.L == nil {
		return errNoLedger
	}
	return s.L.SetSetting(key, value)
}

// ------------------------------------------------------------ migration

// productsSettingKeys are the control-plane keys that used to live in the
// products.db settings table. They now live here; values are copied once.
var productsSettingKeys = []string{
	"autopilot_enabled",
	"autopilot_interval_hours",
	"autopilot_last_run",
	"autopilot_auto_publish",
	"autopilot_music_name",
}

// MigrateProductsSettings copies the legacy products.db control settings
// into the facade the first time it runs. Idempotent: a stored marker in
// the facade means "already copied", so a later products.db is never
// re-read. Existing ledger values always win over products.db values.
func MigrateProductsSettings(ps *products.Store, s Settings) error {
	if ps == nil || s == nil {
		return nil
	}
	if _, ok := s.Get(KeyProductsSettingsMigrated); ok {
		return nil
	}
	for _, k := range productsSettingKeys {
		if v, ok := ps.GetSetting(k); ok {
			if _, exists := s.Get(k); !exists {
				if err := s.Set(k, v); err != nil {
					return err
				}
			}
		}
	}
	return s.Set(KeyProductsSettingsMigrated, "1")
}

// -------------------------------------------------------- master switch

// MasterOn reports the persisted network-daemon switch. Default OFF; a
// stored "1" is an explicit operator choice (R2-08: tắt một cách vô hình
// là lỗi — trạng thái luôn hiển thị trên Trang chủ).
func MasterOn(s Settings) bool {
	if s == nil {
		return false
	}
	v, _ := s.Get(KeyMasterSwitch)
	return v == "1"
}

// SetMasterOn persists the network-daemon switch.
func SetMasterOn(s Settings, on bool) error {
	if on {
		return s.Set(KeyMasterSwitch, "1")
	}
	return s.Set(KeyMasterSwitch, "0")
}

// SeedMasterSwitch writes the switch from the environment exactly once,
// so existing MASTER_SWITCH=1 deployments keep working; afterwards the
// Settings UI is the only control plane. Never overwrites a stored value.
func SeedMasterSwitch(s Settings, envOn bool) error {
	if s == nil {
		return nil
	}
	if _, ok := s.Get(KeyMasterSwitch); ok {
		return nil
	}
	return SetMasterOn(s, envOn)
}

// ------------------------------------------------- growth thresholds UI

// LoadThresholds returns the effective growth governance thresholds: the
// persisted JSON overlaid on growth.DefaultConfig(). Corrupt or partial
// JSON falls back field-by-field to the defaults — a bad edit can never
// brick the engine.
func LoadThresholds(s Settings) growth.Config {
	cfg := growth.DefaultConfig()
	if s == nil {
		return cfg
	}
	raw, ok := s.Get(KeyGrowthThresholds)
	if !ok || raw == "" {
		return cfg
	}
	var over growth.Config
	if err := json.Unmarshal([]byte(raw), &over); err != nil {
		return cfg
	}
	// Field-by-field overlay so a partial edit keeps sane defaults.
	if over.KillMinVideos > 0 {
		cfg.KillMinVideos = over.KillMinVideos
	}
	if over.KillMinDays > 0 {
		cfg.KillMinDays = over.KillMinDays
	}
	if over.KillCompletionFloor > 0 {
		cfg.KillCompletionFloor = over.KillCompletionFloor
	}
	if over.KillProxyFloor > 0 {
		cfg.KillProxyFloor = over.KillProxyFloor
	}
	if over.DoubleDownFactor > 0 {
		cfg.DoubleDownFactor = over.DoubleDownFactor
	}
	if over.BreakoutFactor > 0 {
		cfg.BreakoutFactor = over.BreakoutFactor
	}
	if over.PenaltyViewsDrop > 0 && over.PenaltyViewsDrop < 1 {
		cfg.PenaltyViewsDrop = over.PenaltyViewsDrop
	}
	if over.StalledDays > 0 {
		cfg.StalledDays = over.StalledDays
	}
	if over.StalledGrowthMax > 0 {
		cfg.StalledGrowthMax = over.StalledGrowthMax
	}
	return cfg
}

// SaveThresholds persists the thresholds; "" clears back to defaults.
func SaveThresholds(s Settings, cfg growth.Config) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.Set(KeyGrowthThresholds, string(raw))
}

// ResetThresholds clears the persisted overlay back to defaults.
func ResetThresholds(s Settings) error {
	return s.Set(KeyGrowthThresholds, "")
}

// ------------------------------------------------------ API budget knob

// APIBudgetUSD returns the effective daily API spend ceiling: persisted
// override wins, otherwise the env default. The UI (Settings · Hệ thống)
// edits the persisted value (R2-05: knob vận hành phải có UI).
func APIBudgetUSD(s Settings, envDefault float64) float64 {
	if s != nil {
		if v, ok := s.Get(KeyAPIBudgetUSD); ok && v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				return f
			}
		}
	}
	return envDefault
}

// SetAPIBudgetUSD persists the daily API spend ceiling.
func SetAPIBudgetUSD(s Settings, usd float64) error {
	return s.Set(KeyAPIBudgetUSD, strconv.FormatFloat(usd, 'f', 2, 64))
}

// ------------------------------------------------------ legacy API knob

// APIEnabled reports whether the legacy /api/* JSON endpoints are open.
// Default OFF (R2-09): no UI calls them, so they stay closed on LAN
// exposure unless the operator explicitly opens the switch in Settings.
func APIEnabled(s Settings) bool {
	if s == nil {
		return false
	}
	v, ok := s.Get(KeyAPIEnabled)
	return ok && v == "1"
}

// SetAPIEnabled persists the legacy /api/* switch.
func SetAPIEnabled(s Settings, on bool) error {
	if on {
		return s.Set(KeyAPIEnabled, "1")
	}
	return s.Set(KeyAPIEnabled, "0")
}

// ------------------------------------------------- autopilot switches

// autopilotOn reads a switch with the Đợt 3 semantics: ON when unset, a
// stored "0" is an explicit operator choice and always wins.
func autopilotOn(s Settings, key string) bool {
	if s == nil {
		return true
	}
	v, ok := s.Get(key)
	return !ok || v != "0"
}

// AutopilotEnabled reports the affiliate-cycle scheduler switch.
func AutopilotEnabled(s Settings) bool { return autopilotOn(s, "autopilot_enabled") }

// AutopilotIntervalHours returns the scheduler cadence, default 6.
func AutopilotIntervalHours(s Settings) int {
	if s != nil {
		if v, ok := s.Get("autopilot_interval_hours"); ok {
			if h, err := strconv.Atoi(v); err == nil && h >= 1 && h <= 168 {
				return h
			}
		}
	}
	return 6
}

// AutopilotLastRun returns the stored RFC3339 timestamp of the last
// scheduler cycle, "" when never.
func AutopilotLastRun(s Settings) string {
	if s == nil {
		return ""
	}
	v, _ := s.Get("autopilot_last_run")
	return v
}

// AutopilotAutoPublish reports the auto-publish-to-TikTok switch.
func AutopilotAutoPublish(s Settings) bool { return autopilotOn(s, "autopilot_auto_publish") }

// AutopilotMusicName returns the UI-uploaded music bed filename, "" when none.
func AutopilotMusicName(s Settings) string {
	if s == nil {
		return ""
	}
	v, _ := s.Get("autopilot_music_name")
	return v
}

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ChainConfigJSON is the raw JSON form of a provider chain config. The web
// layer only stores and forwards it; it never parses it deeply — the
// engines worker owns the semantics.
type ChainConfigJSON = json.RawMessage

// ProviderEntry is one provider in a chain, in priority order.
// JSON field names match the engines worker's ChainConfig contract
// (decoded by tts.ParseChainJSON): name / enabled / api_key / api_keys /
// timeout (seconds) / retries.
type ProviderEntry struct {
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	APIKey     string   `json:"api_key"`
	APIKeys    []string `json:"api_keys,omitempty"`
	TimeoutSec int      `json:"timeout"`
	Retries    int      `json:"retries"`
}

// APIKeysText renders the key list for the settings textarea: one key per
// line. Falls back to the legacy single APIKey when APIKeys is empty.
func (e ProviderEntry) APIKeysText() string {
	if len(e.APIKeys) > 0 {
		return strings.Join(e.APIKeys, "\n")
	}
	return e.APIKey
}

// SetAPIKeysText parses the settings textarea: one key per line, blanks
// and surrounding whitespace ignored. An empty text clears the list; the
// legacy APIKey is kept in sync with the first key (or cleared).
func (e *ProviderEntry) SetAPIKeysText(text string) {
	var keys []string
	for _, line := range strings.Split(text, "\n") {
		if k := strings.TrimSpace(line); k != "" {
			keys = append(keys, k)
		}
	}
	e.APIKeys = keys
	if len(keys) > 0 {
		e.APIKey = keys[0]
	} else {
		e.APIKey = ""
	}
}

// ChainConfig is the user-editable chain order: first entry = default tier.
type ChainConfig struct {
	Order []ProviderEntry `json:"order"`
}

// DefaultTTSConfig mirrors the engines default: gemini -> vieneu -> edge.
func DefaultTTSConfig(geminiKeys []string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, TimeoutSec: 60, Retries: 1},
		{Name: "vieneu", Enabled: true, TimeoutSec: 180, Retries: 0},
		{Name: "edge", Enabled: true, TimeoutSec: 60, Retries: 1},
	}}
}

// DefaultLLMConfig mirrors the engines default:
// gemini -> llama-server -> paid (disabled placeholder).
func DefaultLLMConfig(geminiKeys []string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, TimeoutSec: 30, Retries: 1},
		{Name: "llama-server", Enabled: true, TimeoutSec: 120, Retries: 0},
		{Name: "paid", Enabled: false},
	}}
}

// DefaultAvatarConfig mirrors the avatar engine default:
// local (free sidecar) -> heygen (paid, disabled) -> did (paid, disabled).
func DefaultAvatarConfig() ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "local", Enabled: true, TimeoutSec: 1800, Retries: 0},
		{Name: "heygen", Enabled: false, TimeoutSec: 600, Retries: 1},
		{Name: "did", Enabled: false, TimeoutSec: 600, Retries: 1},
	}}
}

// MarshalChain serializes a chain config for the settings table.
func MarshalChain(c ChainConfig) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalChain parses stored JSON; an empty/invalid value yields an
// empty Order (caller falls back to defaults).
func UnmarshalChain(raw string) ChainConfig {
	var c ChainConfig
	if raw == "" {
		return c
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return ChainConfig{}
	}
	return c
}

// LLMClient is the text-generation client used by onboarding/topic
// planning. Structurally identical to network.LLMClient so values are
// freely interchangeable; defined locally because the web package must
// not import internal/engines (built in parallel).
type LLMClient interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
	Name() string
}

// TTSChainAPI is the speech-synthesis chain behind video rendering and
// live narration.
type TTSChainAPI interface {
	Synthesize(ctx context.Context, text, voice string) ([]byte, error)
	SetConfig(cfg ChainConfigJSON) // raw JSON, applied immediately
	ProviderNames() []string
}

// AvatarChainAPI is the avatar provider chain behind clip rendering and
// the settings "render thử" button.
type AvatarChainAPI interface {
	// RenderTestClip renders a test clip: the adapter synthesizes text via
	// the TTS chain, parses emotion cues and renders the avatar clip.
	// It returns the mp4 path relative to the media dir ("/media/x.mp4").
	RenderTestClip(ctx context.Context, characterID int64, text string) (relPath string, err error)
	SetConfig(cfg ChainConfigJSON) // raw JSON, applied immediately
	ProviderNames() []string
	SupportsRealtime() bool
}

// AvatarSidecarCtl controls the avatar sidecar from the settings page
// (same shape as VieNeuCtl).
type AvatarSidecarCtl interface {
	Status() (state, detail string)
	Start(ctx context.Context) error
	Stop() error
	Restart(ctx context.Context) error
	EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error
	ModelConfigured() bool
	ModelPresent() bool
}

// HealthChecker is one provider's cheap "can it serve right now?" probe,
// used by the settings page "Kiểm tra kết nối" button.
type HealthChecker interface {
	Healthy(ctx context.Context) bool
	Name() string
}

// KeyStatus is one API key's rotation state, shown on the settings page.
// It mirrors the engines keyring's JSON shape but lives in the web
// package so web never imports internal/engines (layering). The adapters
// in cmd/aicos convert to this type. Full keys are never exposed — only
// the last 4 characters.
type KeyStatus struct {
	Index int    `json:"index"`
	Last4 string `json:"last4"`
	// Masked is the render-ready display form: bullets + last 4 chars.
	Masked string `json:"masked"`
	State  string `json:"state"` // "ok" | "cooldown" | "invalid"
	// BadgeClass / StateLabel are precomputed for the template and the
	// keyring JS so both render identically.
	BadgeClass           string `json:"badge_class"`
	StateLabel           string `json:"state_label"`
	CooldownRemainingSec int64  `json:"cooldown_remaining_sec"`
	RateLimitHits        int    `json:"rate_limit_hits"`
	Requests             int    `json:"requests"`
	// LastRateLimitUnix is the Unix time of the most recent rate-limit
	// hit (0 = never).
	LastRateLimitUnix int64 `json:"last_rate_limit_unix"`
	// LastRateLimitLabel is the Vietnamese relative time ("5 phút trước"),
	// computed by FillDerived; empty when never rate-limited.
	LastRateLimitLabel string `json:"last_rate_limit_label"`
	Total              int    `json:"total"`
}

// MaskKey renders a key for display: bullets + last 4 characters. The raw
// key is never returned to the client. Very short values are fully hidden.
func MaskKey(k string) string {
	k = strings.TrimSpace(k)
	if len(k) <= 4 {
		return "••••"
	}
	return "••••••••" + k[len(k)-4:]
}

// FillDerived computes the display fields (Masked, BadgeClass, StateLabel)
// from Index/Last4/State/CooldownRemainingSec. Call before rendering or
// encoding to JSON.
func (k *KeyStatus) FillDerived() {
	// Real API keys are always longer than 4 chars, so Last4 is exactly
	// 4 chars. Anything shorter could be a full short key — hide it.
	if len(k.Last4) == 4 {
		k.Masked = "••••••••" + k.Last4
	} else {
		k.Masked, k.Last4 = "••••", "••••"
	}
	switch k.State {
	case "ok":
		k.BadgeClass, k.StateLabel = "badge-ok", "Hoạt động"
	case "cooldown":
		k.BadgeClass, k.StateLabel = "badge-warn", "Nghỉ cooldown"
	case "invalid":
		k.BadgeClass, k.StateLabel = "badge-err", "Key hỏng"
	default:
		k.BadgeClass, k.StateLabel = "badge-no", k.State
	}
	if k.LastRateLimitUnix > 0 {
		k.LastRateLimitLabel = relTime(time.Unix(k.LastRateLimitUnix, 0))
	}
}

// relTime renders a Vietnamese relative time ("vừa xong", "5 phút trước").
func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "vừa xong"
	case d < time.Hour:
		return fmt.Sprintf("%d phút trước", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d giờ trước", int(d.Hours()))
	default:
		return fmt.Sprintf("%d ngày trước", int(d.Hours()/24))
	}
}

// CooldownLabel renders the cooldown badge text, e.g. "Nghỉ cooldown còn 42s".
func (k KeyStatus) CooldownLabel() string {
	if k.State != "cooldown" {
		return k.StateLabel
	}
	return fmt.Sprintf("Nghỉ cooldown còn %ds", k.CooldownRemainingSec)
}

// findChainEntry returns a pointer to the named provider row, or nil.
func findChainEntry(cfg ChainConfig, name string) *ProviderEntry {
	for i := range cfg.Order {
		if cfg.Order[i].Name == name {
			return &cfg.Order[i]
		}
	}
	return nil
}

// last4 returns the last 4 characters of a key for safe display/logging.
// Short values are returned as-is; callers must mask them (see FillDerived).
func last4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}

// KeyStatusProvider is implemented by chain adapters that rotate several
// API keys (gemini). The settings page queries it to show per-key state.
type KeyStatusProvider interface {
	KeyStatus(provider string) []KeyStatus
}

// KeyTester is implemented by chain adapters that can probe one key
// (the settings page "test" button).
type KeyTester interface {
	ValidateKey(ctx context.Context, provider string, idx int) error
}

// VieNeuCtl controls the VieNeu TTS sidecar from the settings page.
type VieNeuCtl interface {
	Status() (state, detail string)
	Start(ctx context.Context) error
	Stop() error
	Restart(ctx context.Context) error
	SetVoice(preset string) error
	EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error
	Voices() []string
}

// VieNeuVoiceFallback is shown when the sidecar is not wired yet or
// reports no voices: Bắc / Trung / Nam presets.
var VieNeuVoiceFallback = []string{"bac", "trung", "nam"}

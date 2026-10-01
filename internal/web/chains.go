package web

import (
	"context"
	"encoding/json"
)

// ChainConfigJSON is the raw JSON form of a provider chain config. The web
// layer only stores and forwards it; it never parses it deeply — the
// engines worker owns the semantics.
type ChainConfigJSON = json.RawMessage

// ProviderEntry is one provider in a chain, in priority order.
// JSON field names match the engines worker's ChainConfig contract:
// name / enabled / api_key / timeout / retries.
type ProviderEntry struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	APIKey     string `json:"api_key"`
	TimeoutSec int    `json:"timeout"`
	Retries    int    `json:"retries"`
}

// ChainConfig is the user-editable chain order: first entry = default tier.
type ChainConfig struct {
	Order []ProviderEntry `json:"order"`
}

// DefaultTTSConfig mirrors the engines default: gemini -> vieneu -> edge.
func DefaultTTSConfig(geminiKey string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKey: geminiKey, TimeoutSec: 60, Retries: 1},
		{Name: "vieneu", Enabled: true, TimeoutSec: 180, Retries: 0},
		{Name: "edge", Enabled: true, TimeoutSec: 60, Retries: 1},
	}}
}

// DefaultLLMConfig mirrors the engines default:
// gemini -> llama-server -> paid (disabled placeholder).
func DefaultLLMConfig(geminiKey string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKey: geminiKey, TimeoutSec: 30, Retries: 1},
		{Name: "llama-server", Enabled: true, TimeoutSec: 120, Retries: 0},
		{Name: "paid", Enabled: false},
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

// HealthChecker is one provider's cheap "can it serve right now?" probe,
// used by the settings page "Kiểm tra kết nối" button.
type HealthChecker interface {
	Healthy(ctx context.Context) bool
	Name() string
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

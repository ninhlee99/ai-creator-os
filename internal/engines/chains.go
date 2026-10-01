package engines

import (
	"fmt"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/local"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
)

// DecisionLogger is the local logging interface for failover decisions.
//
// It intentionally mirrors the ledger's Decide method without importing
// internal/ledger (which is built by another worker): the ledger package
// will satisfy this interface structurally. Every tier switch is logged so
// the user always knows which tier is serving — e.g. when Gemini runs out
// of quota mid-live and the chain jumps to VieNeu, the decision records
// from=gemini, to=vieneu, reason=quota/rate-limit.
type DecisionLogger interface {
	Decide(agent, action string, target *string, reason string, inputs map[string]any)
}

// DefaultLLMConfig returns the default LLM chain order:
// gemini (free API) -> llama-server (local) -> paid (disabled placeholder).
func DefaultLLMConfig(geminiKey string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKey: geminiKey, Timeout: 30 * time.Second, Retries: 1},
		{Name: "llama-server", Enabled: true, Timeout: 120 * time.Second, Retries: 0},
		{Name: "paid", Enabled: false},
	}}
}

// DefaultTTSConfig returns the default TTS chain order:
// gemini (free API, default) -> vieneu (local fallback) -> edge (last resort).
func DefaultTTSConfig(geminiKey string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKey: geminiKey, Timeout: 60 * time.Second, Retries: 1},
		{Name: "vieneu", Enabled: true, Timeout: 180 * time.Second, Retries: 0},
		{Name: "edge", Enabled: true, Timeout: 60 * time.Second, Retries: 1},
	}}
}

// ConfigSource loads the saved chain configs (e.g. from the DB settings
// table). It returns the LLM and TTS configs; an empty Order means "no saved
// settings yet" and the defaults are kept.
type ConfigSource func() (llmCfg ChainConfig, ttsCfg ChainConfig)

// DefaultChains wires the default provider chains:
//
//	LLM: gemini -> llama-server -> paid(disabled)
//	TTS: gemini -> vieneu -> edge
//
// If cfgSrc != nil it is consulted once at startup and any non-empty config
// replaces the defaults. Afterwards the web layer pushes settings changes
// with chain.SetConfig (applies immediately, no restart).
//
// decide may be nil (failovers then stay silent). Otherwise every tier
// switch is logged via Decide("engines", "tier_failover", nil, reason,
// {"from","to","reason"}).
//
// Note: sidecars are NOT started here. The caller starts what it needs:
// vieNeu.Start(ctx) for the TTS sidecar, and the *local.Process returned by
// NewLlamaServerProcess for the LLM tier-2.
func DefaultChains(dataDir string, geminiKey string, decide DecisionLogger, cfgSrc ConfigSource) (*LLMChain, *TTSChain) {
	reg := local.NewRegistry(dataDir)

	onSwitch := func(from, to, reason string) {
		if decide == nil {
			return
		}
		decide.Decide("engines", "tier_failover", nil,
			fmt.Sprintf("%s -> %s (%s)", from, to, reason),
			map[string]any{"from": from, "to": to, "reason": reason})
	}

	llmChain := NewLLMChain([]LLMProvider{
		NewGeminiProvider(geminiKey),
		NewLlamaServerProvider("http://127.0.0.1:8081", "qwen2.5-7b-instruct-q4_k_m"),
		NewPaidLLMProvider(),
	}, onSwitch)
	ttsChain := tts.NewTTSChain([]tts.TTSProvider{
		tts.NewGeminiTTSProvider(geminiKey),
		tts.NewVieNeuProvider(dataDir, reg),
		tts.NewEdgeTTSProvider(),
	}, onSwitch)

	llmCfg, ttsCfg := DefaultLLMConfig(geminiKey), DefaultTTSConfig(geminiKey)
	if cfgSrc != nil {
		if savedLLM, savedTTS := cfgSrc(); len(savedLLM.Order) > 0 || len(savedTTS.Order) > 0 {
			if len(savedLLM.Order) > 0 {
				llmCfg = savedLLM
			}
			if len(savedTTS.Order) > 0 {
				ttsCfg = savedTTS
			}
		}
	}
	llmChain.SetConfig(llmCfg)
	ttsChain.SetConfig(ttsCfg)
	return llmChain, ttsChain
}

// DefaultRegistry returns the model registry for dataDir.
func DefaultRegistry(dataDir string) *local.Registry {
	return local.NewRegistry(dataDir)
}

// NewLlamaServerProcess builds the supervised llama-server process for the
// default GGUF model:
//
//	llama-server -m <data>/models/qwen2.5-7b-instruct-q4_k_m.gguf --port 8081 -c 4096
//
// Port 8081 is used instead of llama-server's default 8080 so it never
// collides with the dashboard's default -addr :8080 in the same binary.
//
// A Metal-enabled llama.cpp build accelerates automatically on Apple Silicon;
// a CPU-only build still works. The model file must exist (Registry.Download)
// before starting.
func NewLlamaServerProcess(reg *local.Registry) *local.Process {
	model := reg.ModelPath("qwen2.5-7b-instruct-q4_k_m.gguf")
	return &local.Process{
		Name:      "llama-server",
		Bin:       "llama-server",
		Args:      []string{"-m", model, "--port", "8081", "-c", "4096"},
		HealthURL: "http://127.0.0.1:8081/health",
	}
}

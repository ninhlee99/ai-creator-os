package engines

import (
	"fmt"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
)

// DecisionLogger is the local logging interface for failover decisions.
type DecisionLogger interface {
	Decide(agent, action string, target *string, reason string, inputs map[string]any)
}

// DefaultLLMConfig configures Gemini as the LLM provider.
func DefaultLLMConfig(geminiKeys []string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, Timeout: 30 * time.Second, Retries: 0},
	}}
}

// DefaultTTSConfig uses local macOS speech first, then Edge, then Gemini.
// This keeps routine narration off API quotas while preserving network
// fallbacks when the local speech engine is unavailable.
func DefaultTTSConfig(geminiKeys []string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "say", Enabled: true, Timeout: 60 * time.Second, Retries: 0},
		{Name: "edge", Enabled: true, Timeout: 60 * time.Second, Retries: 0},
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, Timeout: 60 * time.Second, Retries: 0},
	}}
}

// ConfigSource loads saved chain configs. An empty Order means no saved
// settings, so defaults remain active.
type ConfigSource func() (llmCfg ChainConfig, ttsCfg ChainConfig)

// DefaultChains wires Gemini for LLM and local speech → Edge → Gemini for TTS. Saved
// non-empty configs replace defaults; settings changes apply immediately.
func DefaultChains(geminiKeys []string, decide DecisionLogger, cfgSrc ConfigSource) (*LLMChain, *TTSChain) {
	onSwitch := func(from, to, reason string) {
		if decide == nil {
			return
		}
		decide.Decide("engines", "tier_failover", nil,
			fmt.Sprintf("%s -> %s (%s)", from, to, reason),
			map[string]any{"from": from, "to": to, "reason": reason})
	}

	llmChain := NewLLMChain([]LLMProvider{NewGeminiProviderKeys(geminiKeys)}, onSwitch)
	ttsChain := tts.NewTTSChain([]tts.TTSProvider{
		tts.NewSayTTSProvider(),
		tts.NewEdgeTTSProvider(),
		tts.NewGeminiTTSProviderKeys(geminiKeys),
	}, onSwitch)

	llmCfg, ttsCfg := DefaultLLMConfig(geminiKeys), DefaultTTSConfig(geminiKeys)
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

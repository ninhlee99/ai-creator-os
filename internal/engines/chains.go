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
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, Timeout: 30 * time.Second, Retries: 1},
	}}
}

// DefaultTTSConfig returns the default TTS chain: Gemini followed by Edge.
func DefaultTTSConfig(geminiKeys []string) ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKeys: geminiKeys, Timeout: 60 * time.Second, Retries: 1},
		{Name: "edge", Enabled: true, Timeout: 60 * time.Second, Retries: 1},
	}}
}

// ConfigSource loads saved chain configs. An empty Order means no saved
// settings, so defaults remain active.
type ConfigSource func() (llmCfg ChainConfig, ttsCfg ChainConfig)

// DefaultChains wires Gemini for LLM and Gemini → Edge for TTS. Saved
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
		tts.NewGeminiTTSProviderKeys(geminiKeys),
		tts.NewEdgeTTSProvider(),
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

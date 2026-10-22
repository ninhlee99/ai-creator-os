package main

import (
	"encoding/json"
	"log"

	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func hasEnabledChainEntry(order []engines.ProviderEntry) bool {
	for _, entry := range order {
		if entry.Enabled {
			return true
		}
	}
	return false
}

func defaultChainWithSavedGeminiKeys(def engines.ChainConfig, saved []engines.ProviderEntry) engines.ChainConfig {
	for i := range def.Order {
		if def.Order[i].Name != "gemini" {
			continue
		}
		for _, entry := range saved {
			if entry.Name != "gemini" {
				continue
			}
			if len(entry.APIKeys) > 0 {
				def.Order[i].APIKeys = append([]string(nil), entry.APIKeys...)
				def.Order[i].APIKey = entry.APIKey
			} else if entry.APIKey != "" {
				def.Order[i].APIKey = entry.APIKey
			}
			break
		}
		break
	}
	return def
}

func loadEngineChains(l *ledger.Ledger, geminiKeys []string, decider engines.DecisionLogger) (*engines.LLMChain, *tts.TTSChain) {
	loadChain := func(key string, def engines.ChainConfig) (engines.ChainConfig, bool) {
		raw, ok, err := l.GetSetting(key)
		if err != nil {
			log.Printf("settings %s: read error, using default: %v", key, err)
			return def, false
		}
		if !ok || raw == "" {
			return def, false
		}
		cfg, err := tts.ParseChainJSON(json.RawMessage(raw))
		if err != nil || len(cfg.Order) == 0 {
			log.Printf("settings %s: invalid JSON, using default", key)
			return def, false
		}
		if key == ledger.SettingTTSChain || key == ledger.SettingLLMChain {
			filtered := cfg.Order[:0]
			removed := false
			for _, entry := range cfg.Order {
				supported := key == ledger.SettingTTSChain && (entry.Name == "say" || entry.Name == "gemini" || entry.Name == "edge")
				supported = supported || key == ledger.SettingLLMChain && entry.Name == "gemini"
				if !supported {
					removed = true
					continue
				}
				filtered = append(filtered, entry)
			}
			cfg.Order = filtered
			if removed {
				saved := len(cfg.Order) > 0 && hasEnabledChainEntry(cfg.Order)
				if !saved {
					cfg = defaultChainWithSavedGeminiKeys(def, cfg.Order)
				}
				if clean, marshalErr := json.Marshal(cfg); marshalErr == nil {
					if saveErr := l.SetSetting(key, string(clean)); saveErr != nil {
						log.Printf("settings %s: remove retired provider: %v", key, saveErr)
					}
				}
				return cfg, saved
			}
		}
		return cfg, true
	}
	cfgSrc := func() (engines.ChainConfig, engines.ChainConfig) {
		llmCfg, llmSaved := loadChain(ledger.SettingLLMChain, engines.DefaultLLMConfig(geminiKeys))
		ttsCfg, ttsSaved := loadChain(ledger.SettingTTSChain, engines.DefaultTTSConfig(geminiKeys))
		if llmSaved {
			log.Printf("llm: using saved chain config (llm.chain)")
		} else {
			log.Printf("llm: using default chain config")
		}
		if ttsSaved {
			log.Printf("tts: using saved chain config (tts.chain)")
		} else {
			log.Printf("tts: using default chain config")
		}
		return llmCfg, ttsCfg
	}
	llmChain, ttsChain := engines.DefaultChains(geminiKeys, decider, cfgSrc)
	log.Printf("llm: active providers: %v", llmChain.ActiveProviders())
	log.Printf("tts: active providers: %v", ttsChain.ActiveProviders())
	if len(geminiKeys) == 0 {
		log.Printf("note: GEMINI_API_KEYS / GEMINI_API_KEY not set — LLM calls need a Gemini key; TTS uses local speech, then Edge")
	} else {
		log.Printf("gemini: %d API key(s) configured (rotation enabled)", len(geminiKeys))
	}
	return llmChain, ttsChain
}

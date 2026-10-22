package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func (s *Server) chainKey(name string) (string, bool) {
	switch name {
	case "tts":
		return ledger.SettingTTSChain, true
	case "llm":
		return ledger.SettingLLMChain, true
	}
	return "", false
}

// loadChain returns the stored chain config, falling back to defaults when
// nothing was saved yet.
func (s *Server) loadChain(name string) ChainConfig {
	key, ok := s.chainKey(name)
	if !ok {
		return ChainConfig{}
	}
	raw, found, err := s.Ledger.GetSetting(key)
	if err == nil && found && raw != "" {
		if c := UnmarshalChain(raw); len(c.Order) > 0 {
			previous := c.Order
			c.Order = withoutRetiredProviders(name, previous)
			retiredRemoved := len(c.Order) != len(previous)
			if retiredRemoved && !hasEnabledProvider(c.Order) {
				if name == "tts" {
					return defaultChainWithSavedGeminiKeys(DefaultTTSConfig(s.Cfg.GeminiAPIKeys), c.Order)
				}
				return defaultChainWithSavedGeminiKeys(DefaultLLMConfig(s.Cfg.GeminiAPIKeys), c.Order)
			}
			if len(c.Order) > 0 {
				return c
			}
		}
	}
	if name == "tts" {
		return DefaultTTSConfig(s.Cfg.GeminiAPIKeys)
	}
	return DefaultLLMConfig(s.Cfg.GeminiAPIKeys)
}

// saveChain persists the config and applies it to the live chain
// immediately (no restart).
func (s *Server) saveChain(name string, c ChainConfig) error {
	key, ok := s.chainKey(name)
	if !ok {
		return fmt.Errorf("unknown chain %q", name)
	}
	raw, err := MarshalChain(c)
	if err != nil {
		return err
	}
	if err := s.Ledger.SetSetting(key, raw); err != nil {
		return err
	}
	s.applyChain(name, ChainConfigJSON(raw))
	return nil
}

func (s *Server) applyChain(name string, raw ChainConfigJSON) {
	switch name {
	case "tts":
		if s.TTS != nil {
			s.TTS.SetConfig(raw)
		}
	case "llm":
		// LLM is the narrow LLMClient interface; the real chain (injected
		// by the wiring worker) also implements SetConfig.
		if sc, ok := s.LLM.(interface{ SetConfig(ChainConfigJSON) }); ok && s.LLM != nil {
			sc.SetConfig(raw)
		}
	}
}

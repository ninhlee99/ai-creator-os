package engines

import (
	"testing"
)

func TestDefaultChainsTTSWiring(t *testing.T) {
	_, ttsChain := DefaultChains(nil, nil, nil)

	// Default TTS order must use local speech before network providers.
	var names []string
	for _, e := range ttsChain.Config().Order {
		names = append(names, e.Name)
	}
	want := []string{"say", "edge", "gemini"}
	if len(names) != len(want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order = %v, want %v", names, want)
		}
	}

}

func TestDefaultChainsConfigSource(t *testing.T) {
	// Saved settings replace the defaults; empty Order keeps defaults.
	src := func() (ChainConfig, ChainConfig) {
		return ChainConfig{Order: []ProviderEntry{{Name: "gemini", Enabled: false}}}, ChainConfig{}
	}
	llm, ttsChain := DefaultChains([]string{"k"}, nil, src)
	if got := llm.ActiveProviders(); len(got) != 0 {
		t.Fatalf("llm active = %v", got)
	}
	// TTS had no saved config -> default order kept.
	if got := ttsChain.ActiveProviders(); len(got) != 3 || got[0] != "say" {
		t.Fatalf("tts active = %v", got)
	}

	// nil source -> defaults.
	llm2, _ := DefaultChains([]string{"k"}, nil, nil)
	if got := llm2.ActiveProviders(); len(got) != 1 || got[0] != "gemini" {
		t.Fatalf("llm default active = %v", got)
	}
}

func TestDefaultTTSConfig(t *testing.T) {
	cfg := DefaultTTSConfig([]string{"key"})
	if len(cfg.Order) != 3 {
		t.Fatalf("order len = %d", len(cfg.Order))
	}
	if cfg.Order[0].Name != "say" || cfg.Order[1].Name != "edge" || cfg.Order[2].Name != "gemini" {
		t.Fatalf("order = %v", cfg.Order)
	}
	for _, e := range cfg.Order {
		if !e.Enabled {
			t.Fatalf("%s should be enabled", e.Name)
		}
	}
}

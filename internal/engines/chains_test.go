package engines

import (
	"context"
	"testing"
	"time"
)

// fakeDecider captures Decide calls for assertions.
type fakeDecider struct {
	calls []decideCall
}

type decideCall struct {
	agent  string
	action string
	target *string
	reason string
	inputs map[string]any
}

func (f *fakeDecider) Decide(agent, action string, target *string, reason string, inputs map[string]any) {
	f.calls = append(f.calls, decideCall{agent, action, target, reason, inputs})
}

func TestDefaultChainsLLMFailoverLogged(t *testing.T) {
	// No key, nothing on :8080: gemini fails fast (missing key), llama-server
	// fails fast (connection refused). No real network beyond localhost.
	fake := &fakeDecider{}
	llm, _ := DefaultChains(t.TempDir(), "", fake, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := llm.Complete(ctx, "sys", "hi")
	if err == nil {
		t.Fatal("expected chain error")
	}
	if len(fake.calls) != 1 {
		t.Fatalf("decide calls = %+v, want 1", fake.calls)
	}
	c := fake.calls[0]
	if c.agent != "engines" || c.action != "tier_failover" {
		t.Fatalf("call = %+v", c)
	}
	if c.inputs["from"] != "gemini" || c.inputs["to"] != "llama-server" {
		t.Fatalf("inputs = %v", c.inputs)
	}
	if c.inputs["reason"] != "missing-key" {
		t.Fatalf("reason = %v, want missing-key", c.inputs["reason"])
	}
}

func TestDefaultChainsTTSWiring(t *testing.T) {
	fake := &fakeDecider{}
	_, ttsChain := DefaultChains(t.TempDir(), "", fake, nil)

	// Default TTS order must be gemini -> vieneu -> edge.
	var names []string
	for _, e := range ttsChain.Config().Order {
		names = append(names, e.Name)
	}
	want := []string{"gemini", "vieneu", "edge"}
	if len(names) != len(want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order = %v, want %v", names, want)
		}
	}

	// Restrict to gemini+vieneu so the test needs no network: gemini fails
	// fast on missing key, vieneu fails fast on missing repo.
	ttsChain.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, Timeout: 5 * time.Second},
		{Name: "vieneu", Enabled: true, Timeout: 5 * time.Second},
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := ttsChain.Synthesize(ctx, "xin chào", "default")
	if err == nil {
		t.Fatal("expected chain error")
	}
	if len(fake.calls) != 1 {
		t.Fatalf("decide calls = %+v, want 1", fake.calls)
	}
	c := fake.calls[0]
	if c.inputs["from"] != "gemini" || c.inputs["to"] != "vieneu" {
		t.Fatalf("inputs = %v", c.inputs)
	}
}

func TestDefaultChainsConfigSource(t *testing.T) {
	// Saved settings replace the defaults; empty Order keeps defaults.
	src := func() (ChainConfig, ChainConfig) {
		return ChainConfig{Order: []ProviderEntry{
			{Name: "llama-server", Enabled: true, Timeout: time.Minute},
			{Name: "gemini", Enabled: false},
			{Name: "paid", Enabled: false},
		}}, ChainConfig{}
	}
	llm, ttsChain := DefaultChains(t.TempDir(), "k", nil, src)
	if got := llm.ActiveProviders(); len(got) != 1 || got[0] != "llama-server" {
		t.Fatalf("llm active = %v", got)
	}
	// TTS had no saved config -> default order kept.
	if got := ttsChain.ActiveProviders(); len(got) != 3 || got[0] != "gemini" {
		t.Fatalf("tts active = %v", got)
	}

	// nil source -> defaults.
	llm2, _ := DefaultChains(t.TempDir(), "k", nil, nil)
	if got := llm2.ActiveProviders(); len(got) != 2 || got[0] != "gemini" || got[1] != "llama-server" {
		t.Fatalf("llm default active = %v", got)
	}
}

func TestDefaultTTSConfig(t *testing.T) {
	cfg := DefaultTTSConfig("key")
	if len(cfg.Order) != 3 {
		t.Fatalf("order len = %d", len(cfg.Order))
	}
	if cfg.Order[0].Name != "gemini" || cfg.Order[1].Name != "vieneu" || cfg.Order[2].Name != "edge" {
		t.Fatalf("order = %v", cfg.Order)
	}
	for _, e := range cfg.Order {
		if !e.Enabled {
			t.Fatalf("%s should be enabled", e.Name)
		}
	}
}

func TestNewLlamaServerProcess(t *testing.T) {
	reg := DefaultRegistry(t.TempDir())
	p := NewLlamaServerProcess(reg)
	if p.Bin != "llama-server" {
		t.Fatalf("bin = %q", p.Bin)
	}
	if p.HealthURL != "http://127.0.0.1:8081/health" {
		t.Fatalf("health = %q", p.HealthURL)
	}
	found := false
	for i, a := range p.Args {
		if a == "--port" && i+1 < len(p.Args) && p.Args[i+1] == "8081" {
			found = true
		}
	}
	if !found {
		t.Fatalf("args = %v, want --port 8080", p.Args)
	}
}

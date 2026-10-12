// Adapter nối các chain/provider với các interface mà web và studio cần.
// Tách khỏi main.go để file main giữ mỏng theo STANDARDS (≤700 dòng).
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

// decideAdapter adapts *ledger.Ledger to engines.DecisionLogger: the ledger
// returns errors, the chain interface does not — log instead of crashing.
type decideAdapter struct{ l *ledger.Ledger }

func (d decideAdapter) Decide(agent, action string, target *string, reason string, inputs map[string]any) {
	if err := d.l.Decide(agent, action, target, reason, inputs); err != nil {
		log.Printf("decide(%s/%s): %v", agent, action, err)
	}
}

// ttsChainAdapter adapts *engines.TTSChain to web.TTSChainAPI. The engines
// chain takes a typed ChainConfig; the web layer passes raw JSON in the
// dashboard contract (timeouts in seconds) — decoded by tts.ParseChainJSON.
type ttsChainAdapter struct{ c *engines.TTSChain }

func (a ttsChainAdapter) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	return a.c.Synthesize(ctx, text, voice)
}

func (a ttsChainAdapter) ProviderNames() []string { return a.c.ActiveProviders() }

func (a ttsChainAdapter) SetConfig(raw json.RawMessage) {
	cfg, err := tts.ParseChainJSON(raw)
	if err != nil {
		log.Printf("tts: SetConfig: invalid JSON, keeping current config: %v", err)
		return
	}
	a.c.SetConfig(cfg)
	log.Printf("tts: config updated via dashboard: active=%v", a.c.ActiveProviders())
}

// KeyStatus exposes per-key rotation state for the settings page.
func (a ttsChainAdapter) KeyStatus(provider string) []web.KeyStatus {
	return toWebKeyStatus(a.c.KeyStatus(provider))
}

// ValidateKey tests one API key of a provider (settings "test" button).
func (a ttsChainAdapter) ValidateKey(ctx context.Context, provider string, idx int) error {
	return a.c.ValidateKey(ctx, provider, idx)
}

// llmChainAdapter adapts *engines.LLMChain to the web layer: it satisfies
// web.LLMClient (Complete + Name) and additionally offers raw-JSON
// SetConfig plus key-status / key-test for the settings page. (The typed
// SetConfig(ChainConfig) cannot satisfy the web's SetConfig(json.RawMessage)
// interface, hence this adapter.)
type llmChainAdapter struct{ c *engines.LLMChain }

func (a llmChainAdapter) Complete(ctx context.Context, system, prompt string) (string, error) {
	return a.c.Complete(ctx, system, prompt)
}

func (a llmChainAdapter) Name() string { return a.c.Name() }

// studioLLMAdapter adapts *engines.LLMChain to studio.LLM (the director).
type studioLLMAdapter struct{ c *engines.LLMChain }

func (a studioLLMAdapter) Complete(ctx context.Context, system, prompt string) (string, error) {
	return a.c.Complete(ctx, system, prompt)
}

func (a studioLLMAdapter) Name() string { return a.c.Name() }

func (a studioLLMAdapter) Healthy(context.Context) bool { return len(a.c.ActiveProviders()) > 0 }

func (a llmChainAdapter) SetConfig(raw json.RawMessage) {
	cfg, err := tts.ParseChainJSON(raw)
	if err != nil {
		log.Printf("llm: SetConfig: invalid JSON, keeping current config: %v", err)
		return
	}
	a.c.SetConfig(cfg)
	log.Printf("llm: config updated via dashboard: active=%v", a.c.ActiveProviders())
}

// KeyStatus exposes per-key rotation state for the settings page.
func (a llmChainAdapter) KeyStatus(provider string) []web.KeyStatus {
	return toWebKeyStatus(a.c.KeyStatus(provider))
}

// ValidateKey tests one API key of a provider (settings "test" button).
func (a llmChainAdapter) ValidateKey(ctx context.Context, provider string, idx int) error {
	return a.c.ValidateKey(ctx, provider, idx)
}

// toWebKeyStatus converts the engines keyring statuses to the web
// package's render-ready type (masked, with badge/label fields). The web
// package never imports internal/engines, so the conversion lives here.
func toWebKeyStatus(in []tts.KeyStatus) []web.KeyStatus {
	out := make([]web.KeyStatus, 0, len(in))
	for _, ks := range in {
		w := web.KeyStatus{
			Index:                ks.Index,
			Last4:                ks.Last4,
			State:                ks.State,
			CooldownRemainingSec: ks.CooldownRemainingSec,
			RateLimitHits:        ks.RateLimitHits,
			Requests:             ks.Requests,
			LastRateLimitUnix:    ks.LastRateLimitUnix,
			Total:                len(in),
		}
		w.FillDerived()
		out = append(out, w)
	}
	return out
}

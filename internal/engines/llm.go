package engines

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProviderEntry and ChainConfig are the shared chain-configuration types,
// defined once in the tts subpackage and aliased here so the LLM chain and
// the TTS chain use exactly one type.
type ProviderEntry = tts.ProviderEntry
type ChainConfig = tts.ChainConfig

// TTSChain is the speech-synthesis chain (defined in the tts subpackage).
type TTSChain = tts.TTSChain

// apiKeySetter is implemented by providers whose API keys come from config.
// SetAPIKeys is preferred when the entry carries several keys; SetAPIKey is
// the legacy single-key path (kept as a wrapper).
type apiKeySetter interface {
	SetAPIKey(key string)
	SetAPIKeys(keys []string)
}

// enabledSetter is implemented by providers that mirror the config flag.
type enabledSetter interface{ SetEnabled(bool) }

// LLMProvider is a text-generation backend.
type LLMProvider interface {
	// Complete returns the model's text reply.
	Complete(ctx context.Context, system, prompt string) (string, error)
	// Name is the stable provider id used in config, logs and the ledger.
	Name() string
	// Healthy reports whether the provider can serve right now (cheap check,
	// no quota burned).
	Healthy(ctx context.Context) bool
}

// LLMChain tries providers in the configured order; the first success wins.
// A provider must never crash the chain: errors (and panics) fall through to
// the next tier. The order is user-configurable at runtime via SetConfig.
type LLMChain struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider
	order     []ProviderEntry
	onSwitch  func(from, to, reason string)
}

// NewLLMChain builds a chain over providers. The initial order follows the
// providers slice (all enabled); use SetConfig / DefaultLLMConfig for the
// real order. onSwitch may be nil; it fires on every tier change with a
// classified reason (quota/rate-limit, timeout, missing-key, unreachable,
// error).
func NewLLMChain(providers []LLMProvider, onSwitch func(from, to, reason string)) *LLMChain {
	c := &LLMChain{providers: make(map[string]LLMProvider, len(providers)), onSwitch: onSwitch}
	for _, p := range providers {
		c.providers[p.Name()] = p
		c.order = append(c.order, ProviderEntry{Name: p.Name(), Enabled: true})
	}
	return c
}

// Provider returns a registered provider by name. It reports false when
// the name is unknown. Read-only: safe for concurrent use.
func (c *LLMChain) Provider(name string) (LLMProvider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[name]
	return p, ok
}

// Name identifies the chain itself (not the active provider); it satisfies
// interfaces that expect a single named LLM client (e.g. network.LLMClient).
func (c *LLMChain) Name() string { return "llm-chain" }

// SetConfig replaces the chain order immediately — no restart needed.
// In-flight calls keep the snapshot they started with. API keys and enabled
// flags are pushed into providers that support them.
func (c *LLMChain) SetConfig(cfg ChainConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order = append([]ProviderEntry(nil), cfg.Order...)
	for _, e := range cfg.Order {
		p, ok := c.providers[e.Name]
		if !ok {
			continue
		}
		if s, ok := p.(apiKeySetter); ok {
			if len(e.APIKeys) > 0 {
				s.SetAPIKeys(e.APIKeys)
			} else {
				s.SetAPIKey(e.APIKey)
			}
		}
		if s, ok := p.(enabledSetter); ok {
			s.SetEnabled(e.Enabled)
		}
	}
}

// Config returns a snapshot of the current configuration (for the dashboard).
func (c *LLMChain) Config() ChainConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ChainConfig{Order: append([]ProviderEntry(nil), c.order...)}
}

// ActiveProviders returns the enabled provider names in chain order.
func (c *LLMChain) ActiveProviders() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []string
	for _, e := range c.order {
		if e.Enabled {
			if _, ok := c.providers[e.Name]; ok {
				out = append(out, e.Name)
			}
		}
	}
	return out
}

// KeyStatus returns the per-key rotation state for a provider (e.g.
// "gemini"). Providers without key rotation report nil. Never exposes full
// keys — see tts.KeyStatus.
func (c *LLMChain) KeyStatus(provider string) []tts.KeyStatus {
	c.mu.RLock()
	p, ok := c.providers[provider]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	if ks, ok := p.(interface{ KeyStatus() []tts.KeyStatus }); ok {
		return ks.KeyStatus()
	}
	return nil
}

// ValidateKey runs one minimal request with the provider's key at idx to
// check whether it is valid. Providers without key rotation return an error.
func (c *LLMChain) ValidateKey(ctx context.Context, provider string, idx int) error {
	c.mu.RLock()
	p, ok := c.providers[provider]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown provider %q", provider)
	}
	v, ok := p.(interface {
		ValidateKey(context.Context, int) error
	})
	if !ok {
		return fmt.Errorf("provider %q does not support key testing", provider)
	}
	return v.ValidateKey(ctx, idx)
}

// Complete walks the configured order: disabled providers are skipped, each
// provider is tried up to Retries+1 times with its own per-attempt Timeout,
// and a missing API key fails that attempt fast (still failing over).
func (c *LLMChain) Complete(ctx context.Context, system, prompt string) (string, error) {
	c.mu.RLock()
	order := append([]ProviderEntry(nil), c.order...)
	onSwitch := c.onSwitch
	c.mu.RUnlock()

	var lastErr error
	var prev string
	attempted := false
	for _, e := range order {
		if !e.Enabled {
			continue
		}
		p, ok := c.providers[e.Name]
		if !ok {
			continue // unknown name in config; validate via Config()
		}
		if attempted && onSwitch != nil {
			onSwitch(prev, p.Name(), failReason(lastErr))
		}
		prev, attempted = p.Name(), true

		attempts := e.Retries + 1
		if attempts < 1 {
			attempts = 1
		}
		var text string
		var err error
		for a := 0; a < attempts; a++ {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			text, err = c.attempt(ctx, p, e.Timeout, system, prompt)
			if err == nil {
				return text, nil
			}
			if isMissingKey(err) {
				break // no key: retrying is pointless, fail over fast
			}
		}
		lastErr = err
	}
	if !attempted {
		return "", fmt.Errorf("llm chain: no enabled providers")
	}
	return "", fmt.Errorf("llm chain: all enabled providers failed: %v", lastErr)
}

// attempt runs one provider call with its own timeout; a panic becomes an
// error so the chain survives.
func (c *LLMChain) attempt(ctx context.Context, p LLMProvider, timeout time.Duration, system, prompt string) (text string, err error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("provider %s panicked: %v", p.Name(), r)
		}
	}()
	return p.Complete(ctx, system, prompt)
}

// failReason classifies a provider error for failover logging.
func failReason(err error) string {
	if err == nil {
		return "unknown"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "quota") || strings.Contains(s, "rate limit") ||
		strings.Contains(s, "rate-limit") || strings.Contains(s, "429") ||
		strings.Contains(s, "resource_exhausted"):
		return "quota/rate-limit"
	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "timeout") ||
		strings.Contains(s, "timed out"):
		return "timeout"
	case strings.Contains(s, "api key"):
		return "missing-key"
	case strings.Contains(s, "connection refused") || strings.Contains(s, "no such host") ||
		strings.Contains(s, "network unreachable") || strings.Contains(s, "connection reset"):
		return "unreachable"
	default:
		return "error"
	}
}

func isMissingKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "api key")
}

// ---------------------------------------------------------------------------
// HTTP helpers

func postJSON(ctx context.Context, client *http.Client, url string, body any) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	return data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------------------------------------------------------------------------
// Tier 1: Gemini free tier ("gemini")

// Model không ghi cứng: gmodels.Resolve(gmodels.Text) chọn model còn sống
// (ghi đè GEMINI_TEXT_MODEL → tự dò khi model bị Google tắt → mặc định).

// GeminiProvider calls the Gemini free tier (AI Studio keys).
//
// It holds a KeyRing of API keys and rotates round-robin. Keys belonging to
// the same Google AI Studio project share quota; rotation does not increase
// that project's free-tier limits. A key that
// hits 429/quota cools down (60s -> 5m -> 15m); a rejected key is marked
// invalid until the config changes; transient errors stop after one request
// rather than replaying the same prompt against another key.
// When every key is cooling down the provider fails over to the next chain
// tier immediately instead of spamming retries.

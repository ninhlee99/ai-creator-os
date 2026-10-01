package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProviderEntry is one row of a chain configuration: which provider, whether
// it is enabled, its API key (empty = not entered), its per-attempt timeout
// and how many times to retry it before failing over to the next provider.
type ProviderEntry struct {
	Name    string        // "gemini", "vieneu", "edge", "llama-server", "paid"
	Enabled bool          // false = skipped entirely
	APIKey  string        // key for providers that need one; "" = not entered
	Timeout time.Duration // per-attempt timeout; <=0 = provider default
	Retries int           // retries before failover (attempts = Retries+1)
}

// ChainConfig is the user-editable chain order. The web settings page loads
// it from DB at startup (via ConfigSource) and pushes it back with SetConfig
// after every save — applied immediately, no restart.
type ChainConfig struct {
	Order []ProviderEntry // priority order, first = default tier
}

// TTSProvider is one speech-synthesis backend. The output contract is fixed:
// WAV bytes, PCM s16le, 24 kHz, mono.
type TTSProvider interface {
	// Synthesize returns WAV bytes (PCM s16le 24kHz mono).
	Synthesize(ctx context.Context, text, voice string) (wavBytes []byte, err error)
	// Name is the stable provider id used in config, logs and the ledger.
	Name() string
	// Healthy reports whether the provider can serve right now (cheap check).
	Healthy(ctx context.Context) bool
}

// TTSChain tries providers in the configured order; the first success wins.
// A provider must never crash the chain: errors (and panics) fall through to
// the next tier.
type TTSChain struct {
	mu        sync.RWMutex
	providers map[string]TTSProvider
	order     []ProviderEntry
	onSwitch  func(from, to, reason string)
}

// NewTTSChain builds a chain over providers. The initial order follows the
// providers slice (all enabled); use SetConfig / DefaultTTSConfig for the
// real order. onSwitch may be nil; it fires on every tier change with a
// classified reason.
func NewTTSChain(providers []TTSProvider, onSwitch func(from, to, reason string)) *TTSChain {
	c := &TTSChain{providers: make(map[string]TTSProvider, len(providers)), onSwitch: onSwitch}
	for _, p := range providers {
		c.providers[p.Name()] = p
		c.order = append(c.order, ProviderEntry{Name: p.Name(), Enabled: true})
	}
	return c
}

// Provider returns the provider registered under name (e.g. "vieneu") for
// sidecar control from the caller. It reports false when the name is
// unknown. Read-only: safe for concurrent use.
func (c *TTSChain) Provider(name string) (TTSProvider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[name]
	return p, ok
}

// Name identifies the chain itself (not the active provider); it satisfies
// interfaces that expect a single named TTS client.
func (c *TTSChain) Name() string { return "tts-chain" }

// SetConfig replaces the chain order immediately — no restart needed.
// In-flight calls keep the snapshot they started with. API keys and enabled
// flags are pushed into providers that support them.
func (c *TTSChain) SetConfig(cfg ChainConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order = append([]ProviderEntry(nil), cfg.Order...)
	for _, e := range cfg.Order {
		p, ok := c.providers[e.Name]
		if !ok {
			continue
		}
		if s, ok := p.(interface{ SetAPIKey(string) }); ok {
			s.SetAPIKey(e.APIKey)
		}
		if s, ok := p.(interface{ SetEnabled(bool) }); ok {
			s.SetEnabled(e.Enabled)
		}
	}
}

// Config returns a snapshot of the current configuration (for the dashboard).
func (c *TTSChain) Config() ChainConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ChainConfig{Order: append([]ProviderEntry(nil), c.order...)}
}

// ActiveProviders returns the enabled provider names in chain order.
func (c *TTSChain) ActiveProviders() []string {
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

// Synthesize walks the configured order: disabled providers are skipped,
// each provider is tried up to Retries+1 times with its own per-attempt
// Timeout, and a missing API key fails that attempt fast (still failing
// over to the next tier).
func (c *TTSChain) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
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
		var wav []byte
		var err error
		for a := 0; a < attempts; a++ {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			wav, err = c.attempt(ctx, p, e.Timeout, text, voice)
			if err == nil {
				return wav, nil
			}
			if isMissingKey(err) {
				break // no key: retrying is pointless, fail over fast
			}
		}
		lastErr = err
	}
	if !attempted {
		return nil, fmt.Errorf("tts chain: no enabled providers")
	}
	return nil, fmt.Errorf("tts chain: all enabled providers failed: %v", lastErr)
}

// attempt runs one provider call with its own timeout; a panic becomes an
// error so the chain survives.
func (c *TTSChain) attempt(ctx context.Context, p TTSProvider, timeout time.Duration, text, voice string) (wav []byte, err error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	defer func() {
		if r := recover(); r != nil {
			wav, err = nil, fmt.Errorf("provider %s panicked: %v", p.Name(), r)
		}
	}()
	return p.Synthesize(ctx, text, voice)
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

// postJSON POSTs a JSON body and returns the response bytes (2xx only).
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

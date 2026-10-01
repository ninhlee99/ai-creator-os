package avatar

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DecisionLogger mirrors the ledger's Decide without importing it (see
// engines.DecisionLogger): every tier switch is recorded so the owner can
// always see which avatar tier rendered a clip.
type DecisionLogger interface {
	Decide(agent, action string, target *string, reason string, inputs map[string]any)
}

// ProviderEntry is one row of the avatar chain config: which provider,
// whether enabled, API keys (paid tiers), per-attempt timeout and retries.
// JSON matches the dashboard contract (timeouts are seconds in JSON).
type ProviderEntry struct {
	Name    string        `json:"name"` // "local", "heygen", "did"
	Enabled bool          `json:"enabled"`
	APIKey  string        `json:"api_key,omitempty"`
	APIKeys []string      `json:"api_keys,omitempty"`
	Timeout time.Duration `json:"timeout"`
	Retries int           `json:"retries"`
}

// ChainConfig is the user-editable chain order (first = default tier).
type ChainConfig struct {
	Order []ProviderEntry
}

// DefaultAvatarConfig returns the default chain: local (free) -> heygen ->
// did. Paid tiers are registered but DISABLED by default and have no keys;
// enabling them without a key fails fast with a clear error.
func DefaultAvatarConfig() ChainConfig {
	return ChainConfig{Order: []ProviderEntry{
		{Name: "local", Enabled: true, Timeout: 30 * time.Minute, Retries: 0},
		{Name: "heygen", Enabled: false, Timeout: 10 * time.Minute, Retries: 1},
		{Name: "did", Enabled: false, Timeout: 10 * time.Minute, Retries: 1},
	}}
}

// AvatarChain tries providers in the configured order; the first success
// wins. Errors (and panics) fall through to the next tier, and every
// failover is logged via the DecisionLogger.
type AvatarChain struct {
	mu        sync.RWMutex
	providers map[string]AvatarVideoProvider
	order     []ProviderEntry
	decide    DecisionLogger
}

// NewAvatarChain builds a chain over providers. The initial order follows
// the providers slice (all enabled); use SetConfig / DefaultAvatarConfig
// for the real order.
func NewAvatarChain(providers []AvatarVideoProvider, decide DecisionLogger) *AvatarChain {
	c := &AvatarChain{providers: make(map[string]AvatarVideoProvider), decide: decide}
	for _, p := range providers {
		c.providers[p.Name()] = p
		c.order = append(c.order, ProviderEntry{Name: p.Name(), Enabled: true})
	}
	return c
}

// Provider returns the provider registered under name for sidecar control.
// Read-only: safe for concurrent use.
func (c *AvatarChain) Provider(name string) (AvatarVideoProvider, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.providers[name]
	return p, ok
}

func (c *AvatarChain) Name() string { return "avatar-chain" }

// SetConfig replaces the chain order immediately — no restart. API keys
// and enabled flags are pushed into providers that support them.
func (c *AvatarChain) SetConfig(cfg ChainConfig) {
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

// Config returns a snapshot of the current configuration (for tests).
func (c *AvatarChain) Config() ChainConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ChainConfig{Order: append([]ProviderEntry(nil), c.order...)}
}

// ActiveProviders returns the enabled provider names in chain order.
func (c *AvatarChain) ActiveProviders() []string {
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

// SupportsRealtime reports whether any enabled provider can stream frames.
func (c *AvatarChain) SupportsRealtime() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, e := range c.order {
		if !e.Enabled {
			continue
		}
		if p, ok := c.providers[e.Name]; ok && p.SupportsRealtime() {
			return true
		}
	}
	return false
}

// RenderClip walks the configured order: disabled providers are skipped,
// each is tried up to Retries+1 times with its own Timeout, and every
// failover is logged. The identity lock is verified once, up front.
func (c *AvatarChain) RenderClip(ctx context.Context, ch Character, audioWAV []byte, opts RenderOpts) (string, error) {
	if err := ch.Validate(); err != nil {
		return "", err
	}
	if err := ch.VerifyIdentityLock(); err != nil {
		return "", err
	}
	if len(audioWAV) == 0 {
		return "", fmt.Errorf("avatar: empty audio")
	}

	c.mu.RLock()
	order := append([]ProviderEntry(nil), c.order...)
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
			continue
		}
		if attempted {
			c.logSwitch(prev, p.Name(), lastErr)
		}
		prev, attempted = p.Name(), true

		attempts := e.Retries + 1
		if attempts < 1 {
			attempts = 1
		}
		var path string
		var err error
		for a := 0; a < attempts; a++ {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			path, err = c.attempt(ctx, p, e.Timeout, ch, audioWAV, opts)
			if err == nil {
				return path, nil
			}
			if isMissingKey(err) {
				break // no key: fail over fast
			}
		}
		lastErr = err
	}
	if !attempted {
		return "", fmt.Errorf("avatar chain: no enabled providers")
	}
	return "", fmt.Errorf("avatar chain: all enabled providers failed: %v", lastErr)
}

// OpenStream opens a realtime frame stream on the first enabled provider
// that supports it.
func (c *AvatarChain) OpenStream(ctx context.Context, ch Character, opts StreamOpts) (FrameStream, error) {
	if err := ch.Validate(); err != nil {
		return nil, err
	}
	if err := ch.VerifyIdentityLock(); err != nil {
		return nil, err
	}
	c.mu.RLock()
	order := append([]ProviderEntry(nil), c.order...)
	c.mu.RUnlock()
	for _, e := range order {
		if !e.Enabled {
			continue
		}
		p, ok := c.providers[e.Name]
		if !ok || !p.SupportsRealtime() {
			continue
		}
		return p.OpenStream(ctx, ch, opts)
	}
	return nil, fmt.Errorf("avatar chain: no enabled provider supports realtime streaming")
}

func (c *AvatarChain) attempt(ctx context.Context, p AvatarVideoProvider, timeout time.Duration, ch Character, audioWAV []byte, opts RenderOpts) (path string, err error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	defer func() {
		if r := recover(); r != nil {
			path, err = "", fmt.Errorf("provider %s panicked: %v", p.Name(), r)
		}
	}()
	return p.RenderClip(ctx, ch, audioWAV, opts)
}

func (c *AvatarChain) logSwitch(from, to string, err error) {
	if c.decide == nil {
		return
	}
	reason := "error"
	if err != nil {
		reason = failReason(err)
	}
	c.decide.Decide("engines", "avatar_tier_failover", nil,
		fmt.Sprintf("%s -> %s (%s)", from, to, reason),
		map[string]any{"from": from, "to": to, "reason": reason})
}

func failReason(err error) string {
	if err == nil {
		return "unknown"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "quota") || strings.Contains(s, "rate limit") ||
		strings.Contains(s, "429") || strings.Contains(s, "resource_exhausted"):
		return "quota/rate-limit"
	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "timeout") ||
		strings.Contains(s, "timed out"):
		return "timeout"
	case strings.Contains(s, "api key"):
		return "missing-key"
	case strings.Contains(s, "sidecar") && strings.Contains(s, "not running"):
		return "sidecar-down"
	case strings.Contains(s, "connection refused") || strings.Contains(s, "no such host"):
		return "unreachable"
	default:
		return "error"
	}
}

func isMissingKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "api key")
}

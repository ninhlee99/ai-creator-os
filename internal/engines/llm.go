package engines

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

	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
)

// ProviderEntry and ChainConfig are the shared chain-configuration types,
// defined once in the tts subpackage and aliased here so the LLM chain and
// the TTS chain use exactly one type.
type ProviderEntry = tts.ProviderEntry
type ChainConfig = tts.ChainConfig

// TTSChain is the speech-synthesis chain (defined in the tts subpackage).
type TTSChain = tts.TTSChain

// apiKeySetter is implemented by providers whose API key comes from config.
type apiKeySetter interface{ SetAPIKey(key string) }

// enabledSetter is implemented by providers that mirror the config flag.
type enabledSetter interface{ SetEnabled(bool) }

// LLMProvider is one text-generation backend: free API, local model, or the
// (reserved) paid tier.
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

// Provider returns the provider registered under name (e.g.
// "llama-server") for sidecar control from the caller. It reports false
// when the name is unknown. Read-only: safe for concurrent use.
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
			s.SetAPIKey(e.APIKey)
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

const defaultGeminiLLMModel = "gemini-2.0-flash"

// GeminiProvider calls the Gemini free tier (AI Studio key).
type GeminiProvider struct {
	mu     sync.RWMutex
	apiKey string
	model  string
	http   *http.Client
}

// NewGeminiProvider builds the tier-1 LLM provider. Empty key => Healthy()
// is false and Complete fails fast without touching the network.
func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{
		apiKey: apiKey,
		model:  defaultGeminiLLMModel,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiProvider) Name() string { return "gemini" }

// SetAPIKey updates the key at runtime (called by LLMChain.SetConfig).
func (g *GeminiProvider) SetAPIKey(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apiKey = key
}

func (g *GeminiProvider) Healthy(ctx context.Context) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.apiKey != ""
}

func (g *GeminiProvider) Complete(ctx context.Context, system, prompt string) (string, error) {
	g.mu.RLock()
	key := g.apiKey
	g.mu.RUnlock()
	if key == "" {
		return "", fmt.Errorf("gemini: missing API key")
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/" +
		g.model + ":generateContent?key=" + key
	body := map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
	}
	if system != "" {
		body["system_instruction"] = map[string]any{"parts": []any{map[string]any{"text": system}}}
	}
	data, err := postJSON(ctx, g.http, url, body)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	text, err := parseGeminiText(data)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	return text, nil
}

// parseGeminiText extracts candidates[0].content.parts[0].text.
func parseGeminiText(data []byte) (string, error) {
	var v struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("bad JSON: %w", err)
	}
	if v.Error != nil {
		return "", fmt.Errorf("api error: %s", v.Error.Message)
	}
	if len(v.Candidates) == 0 || len(v.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}
	return v.Candidates[0].Content.Parts[0].Text, nil
}

// ---------------------------------------------------------------------------
// Tier 2: llama-server local ("llama-server")

// LlamaServerProvider talks to a local llama-server (llama.cpp) over its
// OpenAI-compatible /v1/chat/completions endpoint.
type LlamaServerProvider struct {
	baseURL string
	model   string
	http    *http.Client // long timeout: local inference is slow
	probe   *http.Client // short timeout for Healthy()
}

// NewLlamaServerProvider builds the tier-2 LLM provider.
func NewLlamaServerProvider(baseURL, model string) *LlamaServerProvider {
	return &LlamaServerProvider{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		http:    &http.Client{Timeout: 120 * time.Second},
		probe:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (l *LlamaServerProvider) Name() string { return "llama-server" }

// Healthy is true when GET {baseURL}/health returns 200.
func (l *LlamaServerProvider) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := l.probe.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (l *LlamaServerProvider) Complete(ctx context.Context, system, prompt string) (string, error) {
	messages := []any{}
	if system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}
	messages = append(messages, map[string]any{"role": "user", "content": prompt})
	body := map[string]any{
		"model":    l.model,
		"messages": messages,
		"stream":   false,
	}
	data, err := postJSON(ctx, l.http, l.baseURL+"/v1/chat/completions", body)
	if err != nil {
		return "", fmt.Errorf("llama-server: %w", err)
	}
	text, err := parseChatCompletion(data)
	if err != nil {
		return "", fmt.Errorf("llama-server: %w", err)
	}
	return text, nil
}

// parseChatCompletion extracts choices[0].message.content.
func parseChatCompletion(data []byte) (string, error) {
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("bad JSON: %w", err)
	}
	if v.Error != nil {
		return "", fmt.Errorf("api error: %s", v.Error.Message)
	}
	if len(v.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return v.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// Tier 3: paid API ("paid") — placeholder only

// PaidLLMProvider reserves the paid tier in the chain order. It implements
// LLMProvider but never makes a real call: Enabled=false by default and
// Complete always fails with "paid tier disabled".
type PaidLLMProvider struct {
	mu      sync.RWMutex
	Enabled bool
}

// NewPaidLLMProvider builds the disabled-by-default paid placeholder.
func NewPaidLLMProvider() *PaidLLMProvider { return &PaidLLMProvider{} }

func (p *PaidLLMProvider) Name() string { return "paid" }

// SetEnabled mirrors the chain config flag (called by LLMChain.SetConfig).
func (p *PaidLLMProvider) SetEnabled(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Enabled = v
}

func (p *PaidLLMProvider) Healthy(ctx context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Enabled
}

func (p *PaidLLMProvider) Complete(ctx context.Context, system, prompt string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.Enabled {
		return "", fmt.Errorf("paid tier disabled")
	}
	return "", fmt.Errorf("paid tier not implemented (placeholder)")
}

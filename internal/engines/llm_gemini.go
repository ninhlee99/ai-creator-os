package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/freeonly"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type GeminiProvider struct {
	mu      sync.RWMutex
	ring    *tts.KeyRing
	model   string // "" = gmodels.Resolve(gmodels.Text)
	baseURL string // test hook; default is the public Gemini endpoint
	http    *http.Client
}

// NewGeminiProviderKeys builds the tier-1 LLM provider with key rotation.
func NewGeminiProviderKeys(keys []string) *GeminiProvider {
	return &GeminiProvider{
		ring:    tts.NewKeyRing(keys),
		baseURL: "https://generativelanguage.googleapis.com",
		http:    &http.Client{Timeout: 180 * time.Second},
	}
}

// SetBaseURL overrides the API endpoint (tests only).
func (g *GeminiProvider) SetBaseURL(u string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.baseURL = strings.TrimSuffix(u, "/")
}

func (g *GeminiProvider) Name() string { return "gemini" }

// SetAPIKey updates the key at runtime (legacy single-key wrapper, kept for
// backward compatibility; called by LLMChain.SetConfig when the entry has no
// api_keys).
func (g *GeminiProvider) SetAPIKey(key string) { g.SetAPIKeys([]string{key}) }

// SetAPIKeys replaces the key set at runtime (called by LLMChain.SetConfig).
// Applies immediately — in-flight requests finish with their own key.
func (g *GeminiProvider) SetAPIKeys(keys []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ring.SetKeys(keys)
}

// KeyStatus returns the per-key rotation state for the dashboard.
// Full keys are never exposed — only the last 4 characters.
func (g *GeminiProvider) KeyStatus() []tts.KeyStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ring.Status()
}

func (g *GeminiProvider) Healthy(ctx context.Context) bool {
	if !freeonly.GeminiFreeTierConfirmed() {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ring.HasUsable()
}

// ValidateKey performs one minimal request with the key at idx to check
// whether it is valid (dashboard "test" button). An invalid key is marked
// invalid in the ring; a healthy key resets its backoff. This burns a tiny
// amount of that key's quota — it is only called on explicit user action.
func (g *GeminiProvider) ValidateKey(ctx context.Context, idx int) error {
	if !freeonly.GeminiFreeTierConfirmed() {
		return fmt.Errorf("gemini: free-tier key set is not confirmed in Settings → Providers")
	}
	g.mu.RLock()
	key, ok := g.ring.Key(idx)
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("gemini: no key at index %d", idx)
	}
	_, err := g.completeWithKey(ctx, key, "", "ping", true)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil {
		g.ring.ReportSuccess(idx)
		tts.GeminiQuotaCircuit().ObserveSuccess() // nút "test" thành công → quota đã hồi
		return nil
	}
	switch tts.ClassifyKeyError(err) {
	case tts.KeyErrInvalidKey:
		g.ring.ReportInvalid(idx)
		return fmt.Errorf("gemini: key ****%s is invalid: %w", last4(key), err)
	case tts.KeyErrQuota:
		g.ring.ReportSharedQuotaLimit(idx)
		return fmt.Errorf("gemini: key ****%s hit quota/rate-limit: %w", last4(key), err)
	default:
		return fmt.Errorf("gemini: key ****%s test failed: %w", last4(key), err)
	}
}

// last4 is a local alias so the engines package does not reach into the
// tts keyring internals beyond its public API.
func last4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}

func (g *GeminiProvider) Complete(ctx context.Context, system, prompt string) (string, error) {
	if !freeonly.GeminiFreeTierConfirmed() {
		return "", fmt.Errorf("gemini: free-tier key set is not confirmed in Settings → Providers")
	}
	// Đợt S: mạch quota đang mở (hết quota ngày) → fail-fast, không chạm
	// keyring, để chain chuyển sang tier local ngay.
	if qc := tts.GeminiQuotaCircuit(); qc.Open() {
		return "", fmt.Errorf("gemini: quota circuit open (hết quota ngày) — failing over")
	}
	tried := map[int]bool{}
	var lastErr error
	for {
		g.mu.RLock()
		key, idx, ok := g.ring.Next(tried)
		allCooling := !ok && g.ring.Len() > 0 && g.ring.AllCoolingDown()
		keyCount := g.ring.Len()
		g.mu.RUnlock()
		if !ok {
			switch {
			case keyCount == 0:
				return "", fmt.Errorf("gemini: missing API key")
			case allCooling:
				// Every key is cooling down: fail over to the next tier
				// immediately instead of spamming retries.
				// "quota" in the message lets the chain log the reason.
				// Đợt S: đếm lần cạn kiệt liên tiếp cho ngắt mạch quota ngày.
				tts.GeminiQuotaCircuit().ObserveExhausted()
				return "", fmt.Errorf("gemini: all API keys cooling down (quota/rate-limit), failing over")
			case lastErr != nil:
				return "", lastErr
			default:
				return "", fmt.Errorf("gemini: all API keys marked invalid")
			}
		}
		tried[idx] = true
		text, err := g.completeWithKey(ctx, key, system, prompt, false)
		if err == nil {
			g.mu.Lock()
			g.ring.ReportSuccess(idx)
			g.mu.Unlock()
			tts.GeminiQuotaCircuit().ObserveSuccess()
			return text, nil
		}
		lastErr = err
		g.mu.Lock()
		switch tts.ClassifyKeyError(err) {
		case tts.KeyErrQuota:
			g.ring.ReportSharedQuotaLimit(idx)
		case tts.KeyErrInvalidKey:
			g.ring.ReportInvalid(idx)
		default:
			// A transient failure may already have reached Gemini. Do not
			// replay the same prompt through another key.
			g.mu.Unlock()
			return "", err
		}
		g.mu.Unlock()
	}
}

func (g *GeminiProvider) completeWithKey(ctx context.Context, key, system, prompt string, minimal bool) (string, error) {
	g.mu.RLock()
	baseURL, model, httpc := g.baseURL, g.model, g.http
	g.mu.RUnlock()
	auto := model == ""
	if auto {
		model = gmodels.Resolve(gmodels.Text)
	}
	candidates := []string{model, "gemini-3.8-flash", "gemini-3.5-flash", "gemini-3.1-flash-lite", "gemini-3-flash-preview", "gemini-flash-latest"}
	seen := make(map[string]bool)
	var lastErr error
	for _, m := range candidates {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		text, err := g.completeModel(ctx, httpc, baseURL, m, key, system, prompt, minimal)
		if err == nil {
			return text, nil
		}
		lastErr = err
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "503") || strings.Contains(errMsg, "high demand") ||
			strings.Contains(errMsg, "404") || strings.Contains(errMsg, "not found") ||
			strings.Contains(errMsg, "429") || strings.Contains(errMsg, "quota") {
			log.Printf("gemini: model %s lỗi tạm thời (%v) — thử tiếp model dự phòng", m, err)
			continue
		}
		break
	}
	return "", lastErr
}

func (g *GeminiProvider) completeModel(ctx context.Context, httpc *http.Client, baseURL, model, key, system, prompt string, minimal bool) (string, error) {
	url := baseURL + "/v1beta/models/" + model + ":generateContent?key=" + key
	body := map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
	}
	if system != "" {
		body["system_instruction"] = map[string]any{"parts": []any{map[string]any{"text": system}}}
	}
	if minimal {
		body["generationConfig"] = map[string]any{"maxOutputTokens": 1}
	}
	data, err := postJSON(ctx, httpc, url, body)
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

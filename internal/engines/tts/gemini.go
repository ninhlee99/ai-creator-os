package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// persona voice style -> Gemini prebuilt voice (port of GEMINI_VOICES).
var geminiTTSVoices = map[string]string{
	"default":          "Kore",
	"warm-female":      "Aoede", // storyteller: warm, expressive
	"clear-teacher":    "Kore",  // teacher: clear, steady
	"singer":           "Aoede",
	"upbeat-host":      "Puck",
	"energetic-caster": "Puck", // gamer: high energy
	"male":             "Charon",
}

// style keyword -> prompt prefix (Gemini TTS supports style prompting).
var geminiStylePrefix = map[string]string{
	"cheerful":  "Nói một cách vui vẻ, rạng rỡ: ",
	"warm":      "Nói một cách ấm áp, gần gũi: ",
	"suspense":  "Kể một cách hồi hộp, lôi cuốn: ",
	"calm":      "Nói một cách chậm rãi, rõ ràng: ",
	"energetic": "Nói một cách đầy năng lượng, hào hứng: ",
}

// GeminiTTSProvider is the tier-1 TTS provider: Gemini TTS via AI Studio
// generateContent with responseModalities AUDIO. The most expressive option —
// the user chose it as the default, with VieNeu as the offline fallback.
//
// It holds a KeyRing of API keys and rotates round-robin: every request uses
// the next usable key, so N keys multiply the free-tier quota. A key that
// hits 429/quota cools down (60s -> 5m -> 15m); a rejected key is marked
// invalid until the config changes; other errors just try the next key.
// When every key is cooling down the provider fails over to the next chain
// tier immediately instead of spamming retries.
type GeminiTTSProvider struct {
	mu      sync.RWMutex
	ring    *KeyRing
	model   string
	baseURL string // test hook; default is the public Gemini endpoint
	// Style is an optional style keyword (see geminiStylePrefix).
	Style string
	http  *http.Client
}

// NewGeminiTTSProviderKeys builds the tier-1 TTS provider with key rotation.
func NewGeminiTTSProviderKeys(keys []string) *GeminiTTSProvider {
	return &GeminiTTSProvider{
		ring:    NewKeyRing(keys),
		model:   "gemini-2.5-flash-preview-tts",
		baseURL: "https://generativelanguage.googleapis.com",
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// SetBaseURL overrides the API endpoint (tests only).
func (g *GeminiTTSProvider) SetBaseURL(u string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.baseURL = strings.TrimSuffix(u, "/")
}

func (g *GeminiTTSProvider) Name() string { return "gemini" }

// SetAPIKey updates the key at runtime (legacy single-key wrapper, kept for
// backward compatibility; called by TTSChain.SetConfig when the entry has no
// api_keys).
func (g *GeminiTTSProvider) SetAPIKey(key string) { g.SetAPIKeys([]string{key}) }

// SetAPIKeys replaces the key set at runtime (called by TTSChain.SetConfig).
// Applies immediately — in-flight requests finish with their own key.
func (g *GeminiTTSProvider) SetAPIKeys(keys []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ring.SetKeys(keys)
}

// KeyStatus returns the per-key rotation state for the dashboard.
// Full keys are never exposed — only the last 4 characters.
func (g *GeminiTTSProvider) KeyStatus() []KeyStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ring.Status()
}

func (g *GeminiTTSProvider) Healthy(ctx context.Context) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ring.HasUsable()
}

// ValidateKey performs one minimal request with the key at idx to check
// whether it is valid (dashboard "test" button). An invalid key is marked
// invalid in the ring; a healthy key resets its backoff. This burns a tiny
// amount of that key's quota — it is only called on explicit user action.
func (g *GeminiTTSProvider) ValidateKey(ctx context.Context, idx int) error {
	g.mu.RLock()
	key, ok := g.ring.Key(idx)
	g.mu.RUnlock()
	if !ok {
		return fmt.Errorf("gemini: no key at index %d", idx)
	}
	_, err := g.synthesizeWithKey(ctx, key, "Xin chào", "default")
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil {
		g.ring.ReportSuccess(idx)
		return nil
	}
	switch ClassifyKeyError(err) {
	case KeyErrInvalidKey:
		g.ring.ReportInvalid(idx)
		return fmt.Errorf("gemini: key ****%s is invalid: %w", last4(key), err)
	case KeyErrQuota:
		g.ring.ReportRateLimit(idx)
		return fmt.Errorf("gemini: key ****%s hit quota/rate-limit: %w", last4(key), err)
	default:
		return fmt.Errorf("gemini: key ****%s test failed: %w", last4(key), err)
	}
}

func (g *GeminiTTSProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("gemini: empty text")
	}
	tried := map[int]bool{}
	var lastErr error
	for {
		g.mu.RLock()
		key, idx, ok := g.ring.Next(tried)
		allCooling := !ok && g.ring.Len() > 0 && g.ring.AllCoolingDown()
		g.mu.RUnlock()
		if !ok {
			switch {
			case g.keyCount() == 0:
				return nil, fmt.Errorf("gemini: missing API key")
			case allCooling:
				// Every key is cooling down: fail over to the next tier
				// NOW (VieNeu/edge) instead of spamming retries.
				// "quota" in the message lets the chain log the reason.
				return nil, fmt.Errorf("gemini: all API keys cooling down (quota/rate-limit), failing over")
			case lastErr != nil:
				return nil, lastErr
			default:
				return nil, fmt.Errorf("gemini: all API keys marked invalid")
			}
		}
		tried[idx] = true
		wav, err := g.synthesizeWithKey(ctx, key, text, voice)
		if err == nil {
			g.mu.Lock()
			g.ring.ReportSuccess(idx)
			g.mu.Unlock()
			return wav, nil
		}
		lastErr = err
		g.mu.Lock()
		switch ClassifyKeyError(err) {
		case KeyErrQuota:
			g.ring.ReportRateLimit(idx)
		case KeyErrInvalidKey:
			g.ring.ReportInvalid(idx)
		default:
			// network / 5xx / parse: try the next key, no penalty.
		}
		g.mu.Unlock()
	}
}

func (g *GeminiTTSProvider) keyCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ring.Len()
}

func (g *GeminiTTSProvider) synthesizeWithKey(ctx context.Context, key, text, voice string) ([]byte, error) {
	g.mu.RLock()
	baseURL, model, style, httpc := g.baseURL, g.model, g.Style, g.http
	g.mu.RUnlock()
	voiceName := geminiTTSVoices[voice]
	if voiceName == "" {
		voiceName = geminiTTSVoices["default"]
	}
	prompt := geminiStylePrefix[style] + strings.TrimSpace(text)
	url := baseURL + "/v1beta/models/" + model + ":generateContent?key=" + key
	body := map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]any{
				"voiceConfig": map[string]any{
					"prebuiltVoiceConfig": map[string]any{"voiceName": voiceName},
				},
			},
		},
	}
	data, err := postJSON(ctx, httpc, url, body)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	pcm, err := parseGeminiAudio(data)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	return pcmToWav(pcm, 24000), nil
}

// parseGeminiAudio finds the first part with inlineData/inline_data and
// base64-decodes it to raw PCM.
func parseGeminiAudio(data []byte) ([]byte, error) {
	var v struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						Data string `json:"data"`
					} `json:"inlineData"`
					InlineData2 *struct {
						Data string `json:"data"`
					} `json:"inline_data"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("bad JSON: %w", err)
	}
	if v.Error != nil {
		return nil, fmt.Errorf("api error: %s", v.Error.Message)
	}
	if len(v.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}
	for _, part := range v.Candidates[0].Content.Parts {
		var b64 string
		switch {
		case part.InlineData != nil:
			b64 = part.InlineData.Data
		case part.InlineData2 != nil:
			b64 = part.InlineData2.Data
		}
		if b64 == "" {
			continue
		}
		pcm, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("bad base64 audio: %w", err)
		}
		return pcm, nil
	}
	return nil, fmt.Errorf("no audio in response")
}

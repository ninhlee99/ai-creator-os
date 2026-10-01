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
type GeminiTTSProvider struct {
	mu     sync.RWMutex
	apiKey string
	model  string
	// Style is an optional style keyword (see geminiStylePrefix).
	Style string
	http  *http.Client
}

// NewGeminiTTSProvider builds the tier-1 TTS provider.
func NewGeminiTTSProvider(apiKey string) *GeminiTTSProvider {
	return &GeminiTTSProvider{
		apiKey: apiKey,
		model:  "gemini-2.5-flash-preview-tts",
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GeminiTTSProvider) Name() string { return "gemini" }

// SetAPIKey updates the key at runtime (called by TTSChain.SetConfig).
func (g *GeminiTTSProvider) SetAPIKey(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apiKey = key
}

func (g *GeminiTTSProvider) Healthy(ctx context.Context) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.apiKey != ""
}

func (g *GeminiTTSProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	g.mu.RLock()
	key := g.apiKey
	g.mu.RUnlock()
	if key == "" {
		return nil, fmt.Errorf("gemini: missing API key")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("gemini: empty text")
	}
	voiceName := geminiTTSVoices[voice]
	if voiceName == "" {
		voiceName = geminiTTSVoices["default"]
	}
	prompt := geminiStylePrefix[g.Style] + strings.TrimSpace(text)
	url := "https://generativelanguage.googleapis.com/v1beta/models/" +
		g.model + ":generateContent?key=" + key
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
	data, err := postJSON(ctx, g.http, url, body)
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

package studio

import (
	"context"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/freeonly"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ImageRef is one reference image (identity / product lock): a local file
// passed to the image model alongside the prompt.
type ImageRef struct {
	Path string
}

// MediaGen generates production media: still photos and video clips.
type MediaGen interface {
	// GenerateImage renders one image for prompt (+ optional reference
	// images) into outPath.
	GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error
	// GenerateVideo renders a short clip (seconds hint). aspect is
	// "9:16" (1080x1920) or "16:9" (1920x1080) — passed to Veo as
	// aspectRatio; "" means "9:16". firstFrame may be "" (text-to-video)
	// or a local image path (image-to-video, used for identity/product
	// lock).
	GenerateVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string) error
	// Name is the stable provider id.
	Name() string
	// Healthy is a cheap check (keys configured, no quota burned).
	Healthy(ctx context.Context) bool
}

func supportsImageReferences(mg MediaGen) bool {
	provider, ok := mg.(interface{ SupportsImageReferences() bool })
	if !ok {
		return true
	}
	return provider.SupportsImageReferences()
}

// ---------------------------------------------------------------------------
// Gemini implementation
// ---------------------------------------------------------------------------

const (
	geminiBase     = "https://generativelanguage.googleapis.com"
	geminiVeoModel = "veo-3.0-generate-001"
)

// keyBackoffs mirrors the TTS keyring ladder: 60s -> 5m -> 15m.
var keyBackoffs = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

// GeminiMediaGen implements MediaGen over the Gemini API with multi-key
// rotation: keys cool down on 429/quota (progressive backoff) and are
// skipped when rejected as invalid — the same policy as the TTS keyring.
type GeminiMediaGen struct {
	baseURL string
	client  *http.Client

	mu       sync.Mutex
	keys     []string
	coolDown map[int]time.Time
	backoff  map[int]int
	invalid  map[int]bool
	rr       int
}

// NewGeminiMediaGen builds the provider over keys (GEMINI_API_KEYS).
func NewGeminiMediaGen(keys []string) *GeminiMediaGen {
	return &GeminiMediaGen{
		baseURL:  geminiBase,
		client:   &http.Client{Timeout: 130 * time.Second},
		keys:     append([]string{}, keys...),
		coolDown: map[int]time.Time{},
		backoff:  map[int]int{},
		invalid:  map[int]bool{},
	}
}

func (g *GeminiMediaGen) Name() string { return "gemini" }

// SupportsImageReferences is false because every Gemini image model currently
// requires paid billing and Pollinations Free API Classic accepts prompts only.
func (*GeminiMediaGen) SupportsImageReferences() bool { return false }

func (g *GeminiMediaGen) Healthy(_ context.Context) bool {
	if !freeonly.GeminiFreeTierConfirmed() {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.keys {
		if !g.invalid[i] {
			return true
		}
	}
	return false
}

// SetBaseURL overrides the API endpoint (tests).
func (g *GeminiMediaGen) SetBaseURL(u string) { g.baseURL = u }

// KeyCount reports configured keys (dashboard).
func (g *GeminiMediaGen) KeyCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.keys)
}

// SetKeys thay bộ Gemini key lúc đang chạy cho image analysis. Tạo ảnh không
// dùng các key này; cùng bộ key thì giữ nguyên trạng thái cooldown.
func (g *GeminiMediaGen) SetKeys(keys []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if strings.Join(keys, "\n") == strings.Join(g.keys, "\n") {
		return
	}
	g.keys = append([]string{}, keys...)
	g.coolDown, g.backoff, g.invalid, g.rr = map[int]time.Time{}, map[int]int{}, map[int]bool{}, 0
}

// pickKey returns the next usable key index (round-robin, skipping
// cooling-down and invalid keys).
func (g *GeminiMediaGen) pickKey() (int, string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for n := 0; n < len(g.keys); n++ {
		g.rr = (g.rr + 1) % len(g.keys)
		i := g.rr
		if g.invalid[i] {
			continue
		}
		if until, ok := g.coolDown[i]; ok && now.Before(until) {
			continue
		}
		return i, g.keys[i], true
	}
	return 0, "", false
}

// noteResult applies the keyring policy to the outcome of a call.
func (g *GeminiMediaGen) noteResult(idx int, err error) {
	if err == nil {
		g.mu.Lock()
		delete(g.coolDown, idx)
		g.backoff[idx] = 0
		g.mu.Unlock()
		return
	}
	switch tts.ClassifyKeyError(err) {
	case tts.KeyErrQuota:
		g.mu.Lock()
		lvl := g.backoff[idx]
		if lvl >= len(keyBackoffs) {
			lvl = len(keyBackoffs) - 1
		}
		g.coolDown[idx] = time.Now().Add(keyBackoffs[lvl])
		g.backoff[idx] = lvl + 1
		g.mu.Unlock()
	case tts.KeyErrInvalidKey:
		g.mu.Lock()
		g.invalid[idx] = true
		g.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------

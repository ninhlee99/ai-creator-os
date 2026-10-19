package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
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

func (g *GeminiMediaGen) Healthy(_ context.Context) bool {
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

// SetKeys thay bộ key lúc đang chạy (key nhập ở Cài đặt áp dụng ngay cho
// tạo ảnh — trước chỉ đọc biến môi trường lúc khởi động). Cùng bộ key thì
// giữ nguyên trạng thái cooldown.
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
// image generation
// ---------------------------------------------------------------------------

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig map[string]any  `json:"generationConfig,omitempty"`
}

type geminiGenResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func mimeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func loadRef(path string) (geminiPart, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return geminiPart{}, fmt.Errorf("read ref %s: %w", path, err)
	}
	if len(b) > 12<<20 {
		return geminiPart{}, fmt.Errorf("ref %s too large (%d bytes)", path, len(b))
	}
	return geminiPart{InlineData: &geminiInlineData{
		MimeType: mimeOf(path),
		Data:     base64.StdEncoding.EncodeToString(b),
	}}, nil
}

// GenerateImage renders one image via the Gemini image-generation model.
// Reference images (identity/product lock) are passed as inlineData parts.
func (g *GeminiMediaGen) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("empty prompt")
	}
	if len(g.keys) == 0 {
		return fmt.Errorf("gemini: no keys configured")
	}
	var lastErr error
	if !tts.GeminiQuotaCircuit().Open() {
		parts := []geminiPart{{Text: prompt}}
		for _, r := range refs {
			p, err := loadRef(r.Path)
			if err != nil {
				return err
			}
			parts = append(parts, p)
		}
		req := geminiGenRequest{
			Contents: []geminiContent{{Parts: parts}},
			GenerationConfig: map[string]any{
				"responseModalities": []string{"TEXT", "IMAGE"},
			},
		}
		for attempt := 0; attempt < len(g.keys)+1; attempt++ {
			idx, key, ok := g.pickKey()
			if !ok {
				break
			}
			img, err := g.generateImageWithKey(ctx, key, req)
			g.noteResult(idx, err)
			if err == nil {
				if dir := filepath.Dir(outPath); dir != "" {
					_ = os.MkdirAll(dir, 0o755)
				}
				if err := os.WriteFile(outPath, img, 0o644); err != nil {
					return fmt.Errorf("write image: %w", err)
				}
				return nil
			}
			lastErr = err
			// Rotate to next key on invalid key or quota/rate limit error.
			// Only stop early on non-key errors (e.g. malformed prompt).
			if tts.ClassifyKeyError(err) == tts.KeyErrOther {
				break
			}
		}
	} else {
		lastErr = fmt.Errorf("quota circuit open (hết quota ngày)")
	}
	// Tier-2 Image Generation: Pollinations AI (free photorealistic 9:16 vertical engine)
	log.Printf("mediagen: Gemini image API keys unavailable (%v) — attempting Pollinations AI visual generation", lastErr)
	pctx, pcancel := context.WithTimeout(ctx, 35*time.Second)
	perr := generatePollinationsImage(pctx, prompt, outPath)
	pcancel()
	if perr == nil {
		log.Printf("mediagen: ✓ Generated AI visual via Pollinations: %s", filepath.Base(outPath))
		return nil
	}
	log.Printf("mediagen: Pollinations fallback failed (%v) — falling back to synthesized cinematic scene", perr)

	// Tier-3: Synthesized cinematic scene visual with local model/scenery photo compositing
	hashIdx := int(crc32.ChecksumIEEE([]byte(prompt)))
	if hashIdx < 0 {
		hashIdx = -hashIdx
	}
	if gerr := GenerateCinematicStoryScene(hashIdx, 100, prompt, outPath); gerr == nil {
		log.Printf("mediagen: ✓ Generated synthesized cinematic visual (sceneIdx %d): %s", hashIdx, filepath.Base(outPath))
		return nil
	}
	return fmt.Errorf("generate image: %w", lastErr)
}

func generatePollinationsImage(ctx context.Context, prompt string, outPath string) error {
	cleanPrompt := strings.TrimSpace(prompt)
	if cleanPrompt == "" {
		return fmt.Errorf("empty prompt")
	}
	if len(cleanPrompt) > 1200 {
		cleanPrompt = cleanPrompt[:1200]
	}
	hashIdx := int(crc32.ChecksumIEEE([]byte(cleanPrompt))) & 0x7FFFFFFF
	encoded := url.QueryEscape(cleanPrompt)
	reqURL := fmt.Sprintf("https://image.pollinations.ai/prompt/%s?width=1080&height=1920&nologo=true&seed=%d&model=flux", encoded, hashIdx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if len(data) < 2000 || bytes.HasPrefix(data, []byte("{")) {
		return fmt.Errorf("invalid image data (len %d)", len(data))
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(outPath, data, 0o644)
}

func (g *GeminiMediaGen) generateImageWithKey(ctx context.Context, key string, req geminiGenRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	model := gmodels.Resolve(gmodels.Image)
	img, err := g.generateImageModel(ctx, model, key, body)
	if err != nil && gmodels.IsModelGone(err) {
		// Model ảnh bị Google tắt/đổi tên: dò model còn sống, thử lại 1 lần.
		if nm, derr := gmodels.Rediscover(ctx, g.client, g.baseURL, key, gmodels.Image); derr == nil {
			log.Printf("gemini image: model %s không còn — chuyển sang %s", model, nm)
			return g.generateImageModel(ctx, nm, key, body)
		} else {
			log.Printf("gemini image: model %s không còn nhưng dò model mới thất bại: %v", model, derr)
		}
	}
	return img, err
}

func (g *GeminiMediaGen) generateImageModel(ctx context.Context, model, key string, body []byte) ([]byte, error) {
	url := g.baseURL + "/v1beta/models/" + model + ":generateContent?key=" + key
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, trunc(string(data), 300))
	}
	return parseImageResponse(data)
}

// parseImageResponse extracts the first inline image from a generateContent
// response.
func parseImageResponse(data []byte) ([]byte, error) {
	var r geminiGenResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("bad JSON: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("%s: %s", r.Error.Status, r.Error.Message)
	}
	for _, c := range r.Candidates {
		for _, p := range c.Content.Parts {
			if p.InlineData != nil && strings.HasPrefix(p.InlineData.MimeType, "image/") {
				img, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
				if err != nil {
					return nil, fmt.Errorf("bad image data: %w", err)
				}
				if len(img) == 0 {
					continue
				}
				return img, nil
			}
		}
	}
	return nil, fmt.Errorf("no image in response")
}

// ---------------------------------------------------------------------------
// video generation (Veo)
// ---------------------------------------------------------------------------

type veoInstance struct {
	Prompt string `json:"prompt"`
	Image  *struct {
		BytesBase64Encoded string `json:"bytesBase64Encoded"`
		MimeType           string `json:"mimeType"`
	} `json:"image,omitempty"`
}

type veoPredictRequest struct {
	Instances  []veoInstance  `json:"instances"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// veoMaxSeconds is the longest clip Veo 3 renders in one call (~8s).
// Requesting more is silently accepted by the API but the returned clip is
// still ~8s, so we clamp up front and log the real duration (P0-7).
const veoMaxSeconds = 8

// clampVeoSeconds bounds a requested scene duration to what Veo 3 actually
// produces: below 4s the API rejects, above 8s it silently returns ~8s.
func clampVeoSeconds(seconds int) int {
	if seconds < 4 {
		return 8
	}
	if seconds > veoMaxSeconds {
		return veoMaxSeconds
	}
	return seconds
}

// veoAspect normalizes a job aspect to a Veo API aspectRatio value.
// Veo only accepts "9:16"/"16:9"; anything else falls back to vertical.
func veoAspect(aspect string) string {
	if aspect == "16:9" {
		return "16:9"
	}
	return "9:16"
}

// GenerateVideo renders a clip with Veo 3 (image-to-video when firstFrame is
// set). Veo requires a billing-enabled Google Cloud project; without it the
// API returns 400/403 and the caller should fall back to the photo-list
// format (GenerateImage only).
func (g *GeminiMediaGen) GenerateVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("empty prompt")
	}
	// Đợt S: mạch quota đang mở (hết quota ngày) → không đốt API call.
	if tts.GeminiQuotaCircuit().Open() {
		return fmt.Errorf("generate video: quota circuit open (hết quota ngày) — thử lại sau 0h")
	}
	seconds = clampVeoSeconds(seconds)
	inst := veoInstance{Prompt: prompt}
	if firstFrame != "" {
		b, err := os.ReadFile(firstFrame)
		if err != nil {
			return fmt.Errorf("read first frame: %w", err)
		}
		inst.Image = &struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		}{BytesBase64Encoded: base64.StdEncoding.EncodeToString(b), MimeType: mimeOf(firstFrame)}
	}
	req := veoPredictRequest{
		Instances: []veoInstance{inst},
		Parameters: map[string]any{
			"aspectRatio":     veoAspect(aspect),
			"durationSeconds": seconds,
		},
	}
	var lastErr error
	for attempt := 0; attempt < len(g.keys)+1; attempt++ {
		idx, key, ok := g.pickKey()
		if !ok {
			break
		}
		err := g.generateVideoWithKey(ctx, key, req, outPath)
		g.noteResult(idx, err)
		if err == nil {
			return nil
		}
		lastErr = err
		if tts.ClassifyKeyError(err) == tts.KeyErrOther {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable Gemini API key")
	}
	return fmt.Errorf("generate video: %w", lastErr)
}

func (g *GeminiMediaGen) generateVideoWithKey(ctx context.Context, key string, req veoPredictRequest, outPath string) error {
	vc := NewVeoClient(g.baseURL)
	prompt := ""
	firstFrame := ""
	if len(req.Instances) > 0 {
		prompt = req.Instances[0].Prompt
	}
	seconds := 8
	aspect := "9:16"
	if p, ok := req.Parameters["durationSeconds"].(int); ok && p > 0 {
		seconds = p
	}
	if a, ok := req.Parameters["aspectRatio"].(string); ok && a != "" {
		aspect = a
	}
	return vc.RenderVeoVideo(ctx, key, prompt, firstFrame, seconds, aspect, outPath, nil)
}

// postJSON posts body and extracts the "name" field of the response.
func (g *GeminiMediaGen) postJSON(ctx context.Context, url string, body []byte, what string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, trunc(string(data), 300))
	}
	var r struct {
		Name  string `json:"name"`
		Error *struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("bad %s JSON: %w", what, err)
	}
	if r.Error != nil {
		return "", fmt.Errorf("%s: %s", r.Error.Status, r.Error.Message)
	}
	if r.Name == "" {
		return "", fmt.Errorf("no %s name in response", what)
	}
	return r.Name, nil
}

func (g *GeminiMediaGen) downloadFile(ctx context.Context, key, uri, outPath string) error {
	if !strings.Contains(uri, "key=") {
		sep := "?"
		if strings.Contains(uri, "?") {
			sep = "&"
		}
		uri += sep + "key=" + key
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return err
	}
	dl := &http.Client{Timeout: 10 * time.Minute}
	resp, err := dl.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 400<<20))
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

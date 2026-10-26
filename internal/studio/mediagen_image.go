package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/freeonly"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

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

// GenerateImage uses Pollinations Free API Classic for prompts without refs.
// Gemini image models require paid billing; reference generation therefore
// remains unavailable under the free-only policy.
func (g *GeminiMediaGen) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("empty prompt")
	}
	if len(g.keys) == 0 {
		return fmt.Errorf("gemini: no keys configured")
	}
	if len(refs) > 0 {
		return fmt.Errorf("generate image with references: Gemini image models have no free tier; blocked by U-04")
	}
	// Free API Classic requires no account and cannot charge the Gemini project.
	log.Printf("mediagen: generating image via Pollinations Free API Classic")
	pctx, pcancel := context.WithTimeout(ctx, 35*time.Second)
	perr := generatePollinationsImage(pctx, prompt, outPath)
	pcancel()
	if perr == nil {
		log.Printf("mediagen: generated AI visual via Pollinations: %s", filepath.Base(outPath))
		return nil
	}
	return fmt.Errorf("generate image via Pollinations Free API Classic: %w", perr)
}

// AnalyzeImages asks Gemini's text model to inspect ordered image references.
// Used for viral source breakdown; image order is preserved in the request.
func (g *GeminiMediaGen) AnalyzeImages(ctx context.Context, prompt string, refs []ImageRef) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("empty analysis prompt")
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("no reference images supplied")
	}
	if len(g.keys) == 0 {
		return "", fmt.Errorf("gemini: no keys configured")
	}
	if !freeonly.GeminiFreeTierConfirmed() {
		return "", fmt.Errorf("analyze images: Gemini free-tier key set is not confirmed in Settings → Providers")
	}
	if tts.GeminiQuotaCircuit().Open() {
		return "", fmt.Errorf("analyze images: quota circuit open")
	}
	parts := []geminiPart{{Text: prompt}}
	for _, ref := range refs {
		part, err := loadRef(ref.Path)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	body, err := json.Marshal(geminiGenRequest{Contents: []geminiContent{{Parts: parts}}})
	if err != nil {
		return "", fmt.Errorf("marshal image analysis: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt < len(g.keys)+1; attempt++ {
		idx, key, ok := g.pickKey()
		if !ok {
			break
		}
		text, err := g.analyzeImagesWithKey(ctx, key, body)
		g.noteResult(idx, err)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if tts.ClassifyKeyError(err) == tts.KeyErrOther {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable Gemini API key")
	}
	return "", fmt.Errorf("analyze reference images: %w", lastErr)
}

func (g *GeminiMediaGen) analyzeImagesWithKey(ctx context.Context, key string, body []byte) (string, error) {
	model := gmodels.Resolve(gmodels.Text)
	text, err := g.generateTextModel(ctx, model, key, body)
	if err != nil && gmodels.IsModelGone(err) {
		if next, discoverErr := gmodels.Rediscover(ctx, g.client, g.baseURL, key, gmodels.Text); discoverErr == nil {
			return g.generateTextModel(ctx, next, key, body)
		}
	}
	return text, err
}

func (g *GeminiMediaGen) generateTextModel(ctx context.Context, model, key string, body []byte) (string, error) {
	apiURL := g.baseURL + "/v1beta/models/" + model + ":generateContent?key=" + key
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, trunc(string(data), 300))
	}
	var result geminiGenResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("bad Gemini analysis response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("%s: %s", result.Error.Status, result.Error.Message)
	}
	var text strings.Builder
	for _, candidate := range result.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				if text.Len() > 0 {
					text.WriteByte('\n')
				}
				text.WriteString(part.Text)
			}
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("Gemini analysis response had no text")
	}
	return text.String(), nil
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
	tmpRaw := outPath + ".raw.png"
	if err := os.WriteFile(tmpRaw, data, 0o644); err != nil {
		return err
	}
	defer os.Remove(tmpRaw)

	// Tự động cắt sạch 100% logo chân trang (36px watermark) và tối ưu độ phân giải 1080x1920
	vf := "crop=in_w:in_h-36:0:0,scale=1080:1920"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpRaw, "-vf", vf, outPath)
	if err := cmd.Run(); err == nil && fileExists(outPath) {
		return nil
	}
	return os.WriteFile(outPath, data, 0o644)
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

package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

// GenerateVideo is disabled because Veo requires a billing-enabled project.
func (g *GeminiMediaGen) GenerateVideo(_ context.Context, prompt, _ string, _ int, _ string, _ string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("empty prompt")
	}
	return fmt.Errorf("generate video: Google Veo requires a billing-enabled project; blocked by MiuNova free-only policy")
}

func (g *GeminiMediaGen) generateVideoWithKey(ctx context.Context, key string, req veoPredictRequest, firstFrame, outPath string) error {
	vc := NewVeoClient(g.baseURL)
	prompt := ""
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

package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
)

// VeoCandidateModels lists official Google Veo generative video model identifiers in priority order.
var VeoCandidateModels = []string{
	"veo-3.1-generate-preview",
	"veo-3.0-generate-preview",
	"veo-2.0-generate-001",
	"veo-3.0-generate-001",
	"veo-2.0-generate-preview",
}

// GetVeoModels returns the prioritized list of Veo models to attempt, respecting user overrides.
func GetVeoModels() []string {
	custom := strings.TrimSpace(os.Getenv("GEMINI_VEO_MODEL"))
	if custom == "" {
		custom = strings.TrimSpace(os.Getenv("GEMINI_VIDEO_MODEL"))
	}
	if custom != "" {
		return append([]string{custom}, VeoCandidateModels...)
	}
	resolved := gmodels.Resolve(gmodels.Video)
	if resolved != "" && resolved != VeoCandidateModels[0] {
		return append([]string{resolved}, VeoCandidateModels...)
	}
	return VeoCandidateModels
}

// ActiveVeoModel returns the currently preferred Veo model identifier.
func ActiveVeoModel() string {
	models := GetVeoModels()
	if len(models) > 0 {
		return models[0]
	}
	return gmodels.Resolve(gmodels.Video)
}

// VeoClient handles deep integration with Google Veo 3 / Veo 3.1 video generation API.
type VeoClient struct {
	baseURL string
	client  *http.Client
}

// NewVeoClient creates a new client for Google Veo video generation.
func NewVeoClient(baseURL string) *VeoClient {
	if baseURL == "" {
		baseURL = geminiBase
	}
	return &VeoClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// RenderVeoVideo executes an async text-to-video or image-to-video generation job on Google Veo 3,
// polling until completion and downloading the resulting high-definition MP4.
func (vc *VeoClient) RenderVeoVideo(ctx context.Context, key string, prompt, firstFrame string, seconds int, aspect, outPath string, logFn func(string)) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("veo: prompt cannot be empty")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("veo: Gemini API key is missing")
	}

	seconds = clampVeoSeconds(seconds)
	inst := veoInstance{Prompt: prompt}
	if firstFrame != "" && fileExists(firstFrame) {
		b, err := os.ReadFile(firstFrame)
		if err != nil {
			return fmt.Errorf("veo: read first frame: %w", err)
		}
		inst.Image = &struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		}{
			BytesBase64Encoded: base64.StdEncoding.EncodeToString(b),
			MimeType:           mimeOf(firstFrame),
		}
	}

	req := veoPredictRequest{
		Instances: []veoInstance{inst},
		Parameters: map[string]any{
			"aspectRatio":     veoAspect(aspect),
			"durationSeconds": seconds,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("veo: marshal request: %w", err)
	}

	models := GetVeoModels()
	var lastErr error

	for _, model := range models {
		if logFn != nil {
			logFn(fmt.Sprintf("Google Veo 3: Đang gửi lệnh sinh video tới model '%s' (%ds, khổ %s)...", model, seconds, veoAspect(aspect)))
		}

		url := fmt.Sprintf("%s/v1beta/models/%s:predictLongRunning?key=%s", vc.baseURL, model, key)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", key)

		resp, err := vc.client.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		respData, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("veo model %s returned status %d: %s", model, resp.StatusCode, string(respData))
			// If model not found (404), try the next candidate model
			if resp.StatusCode == http.StatusNotFound {
				continue
			}
			return lastErr
		}

		var opResp struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(respData, &opResp); err != nil || opResp.Name == "" {
			lastErr = fmt.Errorf("veo: response missing operation name: %s", string(respData))
			continue
		}

		if logFn != nil {
			logFn(fmt.Sprintf("Google Veo 3: Tác vụ đã khởi tạo thành công (Operation: %s). Đang kết xuất video...", opResp.Name))
		}

		// Poll the long-running operation
		return vc.pollOperation(ctx, key, opResp.Name, outPath, logFn)
	}

	if lastErr != nil {
		return fmt.Errorf("veo render failed across all candidate models: %w", lastErr)
	}
	return fmt.Errorf("veo: no candidate models succeeded")
}

func (vc *VeoClient) pollOperation(ctx context.Context, key, opName, outPath string, logFn func(string)) error {
	pollClient := &http.Client{Timeout: 60 * time.Second}
	deadline := time.Now().Add(15 * time.Minute)
	pollInterval := 10 * time.Second
	pollCount := 0

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("veo: quá thời gian chờ kết xuất video (timeout 15 phút)")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		pollCount++
		opURL := fmt.Sprintf("%s/v1beta/%s?key=%s", vc.baseURL, opName, key)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, opURL, nil)
		if err != nil {
			continue
		}
		httpReq.Header.Set("x-goog-api-key", key)

		resp, err := pollClient.Do(httpReq)
		if err != nil {
			continue // transient network error; keep polling
		}

		data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()

		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var op struct {
			Done  bool `json:"done"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
			Response *struct {
				GenerateVideoResponse *struct {
					GeneratedSamples []struct {
						Video struct {
							URI string `json:"uri"`
						} `json:"video"`
					} `json:"generatedSamples"`
				} `json:"generateVideoResponse"`
			} `json:"response,omitempty"`
		}

		if err := json.Unmarshal(data, &op); err != nil {
			continue
		}

		if op.Error != nil {
			return fmt.Errorf("veo error (%d): %s", op.Error.Code, op.Error.Message)
		}

		if !op.Done {
			if logFn != nil && pollCount%3 == 0 {
				logFn(fmt.Sprintf("Google Veo 3: Đang tính toán chuyển động khung hình AI (đã chờ %ds)...", pollCount*10))
			}
			continue
		}

		if op.Response == nil || op.Response.GenerateVideoResponse == nil {
			return fmt.Errorf("veo: hoàn tất nhưng không có dữ liệu phản hồi video")
		}

		samples := op.Response.GenerateVideoResponse.GeneratedSamples
		if len(samples) == 0 || samples[0].Video.URI == "" {
			return fmt.Errorf("veo: không tìm thấy liên kết video đầu ra")
		}

		uri := samples[0].Video.URI
		if logFn != nil {
			logFn(fmt.Sprintf("Google Veo 3: Video đã tạo thành công! Đang tải file MP4 chất lượng cao..."))
		}

		return vc.downloadVideoFile(ctx, key, uri, outPath)
	}
}

func (vc *VeoClient) downloadVideoFile(ctx context.Context, key, uri, outPath string) error {
	dlURL := uri
	if !strings.Contains(dlURL, "key=") && key != "" {
		if strings.Contains(dlURL, "?") {
			dlURL += "&key=" + key
		} else {
			dlURL += "?key=" + key
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	if key != "" {
		req.Header.Set("x-goog-api-key", key)
	}

	dlClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("download video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download video status %d", resp.StatusCode)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	return nil
}

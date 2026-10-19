package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MuseAIClient handles integration with Muse AI video creation and hosting services.
type MuseAIClient struct {
	baseURL string
	apiKey  string
	session string
	client  *http.Client
}

// NewMuseAIClient creates a new Muse AI client instance with optional custom API key.
func NewMuseAIClient(apiKey, session string) *MuseAIClient {
	if apiKey == "" {
		apiKey = os.Getenv("MUSE_AI_API_KEY")
	}
	if session == "" {
		session = os.Getenv("MUSE_AI_SESSION")
	}
	return &MuseAIClient{
		baseURL: "https://muse.ai/api",
		apiKey:  strings.TrimSpace(apiKey),
		session: strings.TrimSpace(session),
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

// Healthy checks whether Muse AI credentials are configured.
func (mc *MuseAIClient) Healthy() bool {
	return mc.apiKey != "" || mc.session != ""
}

// RenderMuseAIVideo generates or processes a motion video clip through Muse AI.
func (mc *MuseAIClient) RenderMuseAIVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string, logFn func(string)) error {
	if !mc.Healthy() {
		return fmt.Errorf("muse.ai: chưa cấu hình MUSE_AI_API_KEY hoặc phiên đăng nhập Muse AI trong Cài đặt")
	}

	if logFn != nil {
		logFn("Muse AI: Đang kết nối tới máy chủ Muse AI Video Studio...")
	}

	// 1. If a keyframe image is provided, upload it to Muse AI
	var uploadedFileID string
	if firstFrame != "" && fileExists(firstFrame) {
		if logFn != nil {
			logFn(fmt.Sprintf("Muse AI: Đang tải ảnh keyframe '%s' lên máy chủ Muse AI...", filepath.Base(firstFrame)))
		}
		fileID, err := mc.uploadFile(ctx, firstFrame)
		if err != nil {
			if logFn != nil {
				logFn(fmt.Sprintf("Muse AI upload cảnh báo (%v), chuyển sang chế độ tạo từ mô tả văn bản...", err))
			}
		} else {
			uploadedFileID = fileID
		}
	}

	// 2. Request video generation
	if logFn != nil {
		logFn(fmt.Sprintf("Muse AI: Đang khởi tạo render video chuyển động (%ds, khổ %s)...", seconds, aspect))
	}

	reqPayload := map[string]any{
		"prompt":       prompt,
		"duration":     seconds,
		"aspect_ratio": aspect,
	}
	if uploadedFileID != "" {
		reqPayload["reference_file_id"] = uploadedFileID
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("muse.ai: marshal request: %w", err)
	}

	reqURL := mc.baseURL + "/videos"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	mc.setAuthHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := mc.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("muse.ai request: %w", err)
	}
	defer resp.Body.Close()

	bodyData, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("muse.ai API error (status %d): %s", resp.StatusCode, string(bodyData))
	}

	var res struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		VideoURL string `json:"video_url"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(bodyData, &res); err != nil {
		return fmt.Errorf("muse.ai unmarshal response: %w", err)
	}

	videoURL := res.VideoURL
	if videoURL == "" {
		videoURL = res.URL
	}

	if videoURL != "" && strings.HasPrefix(videoURL, "http") {
		if logFn != nil {
			logFn("Muse AI: Video đã tạo thành công! Đang tải file MP4...")
		}
		return downloadRemoteMedia(ctx, videoURL, outPath)
	}

	return fmt.Errorf("muse.ai: không nhận được URL video hợp lệ từ phản hồi")
}

func (mc *MuseAIClient) uploadFile(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	reqURL := mc.baseURL + "/files/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return "", err
	}
	mc.setAuthHeaders(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := mc.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	resBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("upload status %d: %s", resp.StatusCode, string(resBytes))
	}

	var uploadRes struct {
		ID  string `json:"id"`
		FID string `json:"file_id"`
	}
	_ = json.Unmarshal(resBytes, &uploadRes)
	if uploadRes.ID != "" {
		return uploadRes.ID, nil
	}
	return uploadRes.FID, nil
}

func (mc *MuseAIClient) setAuthHeaders(req *http.Request) {
	if mc.apiKey != "" {
		req.Header.Set("Key", mc.apiKey)
		req.Header.Set("Authorization", "Bearer "+mc.apiKey)
	}
	if mc.session != "" {
		req.Header.Set("Cookie", mc.session)
	}
	req.Header.Set("User-Agent", "AICreatorOS/2.0")
}

func downloadRemoteMedia(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

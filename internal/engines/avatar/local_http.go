//go:build parked

package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type healthResp struct {
	Status  string `json:"status"`
	Model   string `json:"model"`
	Device  string `json:"device"`
	Version string `json:"version"`
}

type renderReq struct {
	AvatarID          string            `json:"avatar_id"`
	ReferenceImageB64 string            `json:"reference_image_b64"`
	AudioWavB64       string            `json:"audio_wav_b64"`
	EmotionCues       []TimedEmotionCue `json:"emotion_cues"`
	Seed              int64             `json:"seed"`
	IdentityLock      string            `json:"identity_lock"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	FPS               int               `json:"fps"`
}

type renderResp struct {
	VideoMP4B64  string  `json:"video_mp4_b64"`
	DurationSec  float64 `json:"duration_sec"`
	Frames       int     `json:"frames"`
	IdentityLock string  `json:"identity_lock"`
}

type openReq struct {
	AvatarID          string `json:"avatar_id"`
	ReferenceImageB64 string `json:"reference_image_b64"`
	Seed              int64  `json:"seed"`
	IdentityLock      string `json:"identity_lock"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	FPS               int    `json:"fps"`
}

type openResp struct {
	SessionID string `json:"session_id"`
}

type pushReq struct {
	SessionID string `json:"session_id"`
	PCMB64    string `json:"pcm_b64"`
}

func (p *LocalAvatarProvider) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp, out)
}

func (p *LocalAvatarProvider) post(ctx context.Context, path string, body any, out any) error {
	return p.postWith(ctx, p.client, path, body, out)
}

// postRender is post() against /render with the long-timeout client (see
// the renderClient field). Falls back to the shared client when the
// provider was built as a struct literal (tests).
func (p *LocalAvatarProvider) postRender(ctx context.Context, body any, out any) error {
	c := p.renderClient
	if c == nil {
		c = p.client
	}
	return p.postWith(ctx, c, "/render", body, out)
}

func (p *LocalAvatarProvider) postWith(ctx context.Context, client *http.Client, path string, body any, out any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp, out)
}

func decodeJSON(resp *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("bad JSON: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

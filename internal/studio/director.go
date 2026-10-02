package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Director chuyên nghiệp (yêu cầu Ninh 2026-10-01):
//   - Video affiliate 30s: list ảnh 5–10 ảnh.
//   - Video từ 1 phút: shot list điện ảnh như có cameraman/photographer/
//     editor thật — cỡ cảnh, chuyển động máy, ống kính, ánh sáng, story arc.
// (Pipeline phim ngắn đã park — xem film_director.go, build tag parked.)

// ---------------------------------------------------------------------------
// shot điện ảnh
// ---------------------------------------------------------------------------

// ProShot là một shot quay theo ngôn ngữ điện ảnh.
type ProShot struct {
	Index         int    `json:"index"`
	Purpose       string `json:"purpose"`     // hook | build | payoff …
	ShotSize      string `json:"shot_size"`   // ECU, CU, MCU, MS, WS, EWS, drone …
	CameraMove    string `json:"camera_move"` // dolly-in, tracking, handheld, static …
	LensLight     string `json:"lens_light"`  // 35mm f/1.8, golden hour …
	Seconds       int    `json:"seconds"`
	Action        string `json:"action"`         // diễn xuất/hành động (tiếng Việt)
	ImagePrompt   string `json:"image_prompt"`   // prompt ảnh (tiếng Anh)
	VideoPrompt   string `json:"video_prompt"`   // prompt video (tiếng Anh)
	TrailerWorthy bool   `json:"trailer_worthy"` // shot đắt giá → cắt trailer 9:16
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func parseDirectorJSON(text string, v any) error {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "```"))
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("director JSON: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 60s affiliate: shot list điện ảnh
// ---------------------------------------------------------------------------

// WriteProShotList viết shot list điện ảnh cho video affiliate dài (>= 60s):
// story arc hook -> build -> payoff, mỗi shot có cỡ cảnh, chuyển động máy,
// ống kính/ánh sáng như cameraman thật. Không chữ, không voiceover.
func WriteProShotList(ctx context.Context, llm LLM, productName, niche string, seconds int) ([]ProShot, error) {
	n := seconds / 7
	if n < 6 {
		n = 6
	}
	if n > 10 {
		n = 10
	}
	sys := "Bạn là đạo diễn quảng cáo TikTok Việt Nam đẳng cấp quốc tế. Chỉ trả lời JSON thuần."
	prompt, err := directorPrompt("affiliate_shotlist.txt", promptData{
		Topic: productName, Niche: niche, Seconds: seconds, N: n,
	})
	if err != nil {
		return nil, err
	}
	text, err := llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("director: %w", err)
	}
	var plan struct {
		Shots []ProShot `json:"shots"`
	}
	if err := parseDirectorJSON(text, &plan); err != nil {
		return nil, err
	}
	var out []ProShot
	for _, sh := range plan.Shots {
		if strings.TrimSpace(sh.ImagePrompt) == "" {
			continue
		}
		if sh.Seconds < 3 {
			sh.Seconds = 5
		}
		out = append(out, sh)
	}
	if len(out) < 4 {
		return nil, fmt.Errorf("director: chỉ có %d shot, cần >= 4", len(out))
	}
	return out, nil
}

package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// GenerateViralScript writes a source-grounded storyboard. It refuses to create
// content when the source video or channel character evidence is missing.
func (s *Studio) GenerateViralScript(ctx context.Context, p ViralParams) (*ViralScript, error) {
	duration := p.TargetDuration
	if fileExists(p.SampleVideo) {
		measured := probeDuration(ctx, p.SampleVideo)
		if measured <= 0 {
			return nil, fmt.Errorf("không đọc được thời lượng video nguồn")
		}
		duration = measured
	}
	if duration <= 0 {
		return nil, fmt.Errorf("cần thời lượng video nguồn chính xác")
	}
	p.TargetDuration = duration

	if p.ModelPhoto == "" {
		for _, photo := range p.ModelPhotos {
			if fileExists(photo) {
				p.ModelPhoto = photo
				break
			}
		}
	}
	if !fileExists(p.ModelPhoto) {
		return nil, fmt.Errorf("cần ảnh nhân vật đã chọn của kênh để lập storyboard tái hiện")
	}
	if len(p.ReferenceBeats) == 0 || strings.TrimSpace(p.ReferenceSummary) == "" {
		summary, beats, err := s.analyzeViralReference(ctx, p.SampleVideo, duration)
		if err != nil {
			return nil, err
		}
		p.ReferenceSummary = summary
		p.ReferenceBeats = beats
	}
	if err := validateViralReferenceBeats(p.ReferenceBeats, duration); err != nil {
		return nil, err
	}
	if s.llm == nil || !s.llm.Healthy(ctx) {
		return nil, fmt.Errorf("không có LLM khả dụng để chuyển phân tích video nguồn thành storyboard")
	}

	beats, err := json.MarshalIndent(p.ReferenceBeats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("không mã hóa được beat video nguồn: %w", err)
	}
	prompt, err := directorPrompt("viral_recreate.txt", promptData{
		Topic:            p.Topic,
		Niche:            p.Account,
		Seconds:          int(math.Round(duration)),
		Duration:         formatViralDuration(duration),
		ReferenceSummary: strings.TrimSpace(p.ReferenceSummary),
		ReferenceBeats:   string(beats),
	})
	if err != nil {
		return nil, err
	}
	response, err := s.llm.Complete(ctx, "Bạn là đạo diễn tái hiện video theo nguồn đã phân tích. Chỉ trả JSON thuần, không thêm nội dung.", prompt)
	if err != nil {
		return nil, fmt.Errorf("viết storyboard tái hiện nguồn: %w", err)
	}

	var script ViralScript
	if err := json.Unmarshal([]byte(extractJSON(response)), &script); err != nil {
		return nil, fmt.Errorf("storyboard trả JSON không hợp lệ: %w", err)
	}
	if strings.TrimSpace(script.Title) == "" {
		return nil, fmt.Errorf("storyboard thiếu tiêu đề mô tả nguồn")
	}
	if err := validateViralReferenceScenes(script.Scenes, duration); err != nil {
		return nil, err
	}

	// The available source evidence is visual-only. Never infer audio, text,
	// marketing copy, or engagement assets from sampled frames.
	script.Hook3s = ""
	script.CallToAction = ""
	script.Hashtags = nil
	for i := range script.Scenes {
		script.Scenes[i].VoiceText = ""
		script.Scenes[i].VisualHook = ""
	}
	script.SourcePerformance = p.SourcePerformance
	return &script, nil
}

func formatViralDuration(seconds float64) string {
	formatted := strconv.FormatFloat(seconds, 'f', 3, 64)
	return strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

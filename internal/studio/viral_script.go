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
	performanceEvidence, err := json.Marshal(p.SourcePerformance)
	if err != nil {
		return nil, fmt.Errorf("không mã hóa được chỉ số Radar nguồn: %w", err)
	}
	prompt, err := directorPrompt("viral_recreate.txt", promptData{
		Topic:             p.Topic,
		Niche:             p.Account,
		Seconds:           int(math.Round(duration)),
		Duration:          formatViralDuration(duration),
		ReferenceSummary:  strings.TrimSpace(p.ReferenceSummary),
		ReferenceBeats:    string(beats),
		SourcePerformance: string(performanceEvidence),
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
	script.Analysis = sanitizeViralSourceAnalysis(script.Analysis, p.ReferenceBeats)

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

func sanitizeViralSourceAnalysis(analysis ViralSourceAnalysis, beats []ViralReferenceBeat) ViralSourceAnalysis {
	var clean ViralSourceAnalysis
	allowedFactors := map[string]bool{
		"opening": true, "pacing": true, "movement": true,
		"camera": true, "editing": true, "visual_payoff": true,
	}
	for _, factor := range analysis.Factors {
		name := strings.ToLower(strings.TrimSpace(factor.Factor))
		observation := cleanViralAnalysisText(factor.Observation, 220)
		hypothesis := cleanViralAnalysisText(factor.Hypothesis, 260)
		evidenceTimes := matchedViralEvidenceTimes(factor.EvidenceTimesSec, beats)
		if !allowedFactors[name] || observation == "" || hypothesis == "" || len(evidenceTimes) == 0 {
			continue
		}
		hypothesisLead := strings.ToLower(hypothesis)
		if !strings.HasPrefix(hypothesisLead, "có thể") && !strings.HasPrefix(hypothesisLead, "giả thuyết") {
			hypothesis = "Giả thuyết: " + hypothesis
		}
		confidence := strings.ToLower(strings.TrimSpace(factor.Confidence))
		if confidence == "high" {
			confidence = "medium"
		}
		if confidence != "medium" {
			confidence = "low"
		}
		clean.Factors = append(clean.Factors, ViralSourceFactor{
			Factor: name, Observation: observation, EvidenceTimesSec: evidenceTimes,
			Hypothesis: hypothesis, Confidence: confidence,
		})
		if len(clean.Factors) == 4 {
			break
		}
	}
	allowedMetrics := map[string]bool{
		"views_per_hour": true, "engagement_rate": true, "share_count": true,
		"comment_count": true, "like_count": true,
	}
	for _, test := range analysis.NextTests {
		change := cleanViralAnalysisText(test.Change, 220)
		metric := strings.ToLower(strings.TrimSpace(test.Metric))
		evidenceTimes := matchedViralEvidenceTimes(test.EvidenceTimesSec, beats)
		if change == "" || !allowedMetrics[metric] || len(evidenceTimes) == 0 {
			continue
		}
		clean.NextTests = append(clean.NextTests, ViralNextTest{
			Change: change, Metric: metric, EvidenceTimesSec: evidenceTimes,
		})
		if len(clean.NextTests) == 3 {
			break
		}
	}
	return clean
}

func matchedViralEvidenceTimes(candidates []float64, beats []ViralReferenceBeat) []float64 {
	matched := make([]float64, 0, min(len(candidates), 3))
	seen := make(map[float64]bool)
	for _, candidate := range candidates {
		if math.IsNaN(candidate) || math.IsInf(candidate, 0) {
			continue
		}
		for _, beat := range beats {
			if math.Abs(candidate-beat.TimeSec) > 0.01 || seen[beat.TimeSec] {
				continue
			}
			matched = append(matched, beat.TimeSec)
			seen[beat.TimeSec] = true
			break
		}
		if len(matched) == 3 {
			break
		}
	}
	return matched
}

func cleanViralAnalysisText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return value
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

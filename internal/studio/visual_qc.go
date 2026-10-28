package studio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// VisualQCReport records only visual checks actually performed on sampled
// frames. It contains no predicted views, retention, or conversion metrics.
type VisualQCReport struct {
	Decision                 string                   `json:"decision"`
	Summary                  string                   `json:"summary"`
	ProductMatch             string                   `json:"product_match,omitempty"`
	ProductUsage             string                   `json:"product_usage,omitempty"`
	ProductVisibility        string                   `json:"product_visibility,omitempty"`
	CharacterIdentity        string                   `json:"character_identity,omitempty"`
	Motion                   string                   `json:"motion,omitempty"`
	ShotVariety              string                   `json:"shot_variety,omitempty"`
	Sharpness                string                   `json:"sharpness,omitempty"`
	Composition              string                   `json:"composition,omitempty"`
	EmotionalArc             string                   `json:"emotional_arc,omitempty"`
	Continuity               string                   `json:"continuity,omitempty"`
	SceneOrder               string                   `json:"scene_order,omitempty"`
	ActionSequence           string                   `json:"action_sequence,omitempty"`
	CameraPattern            string                   `json:"camera_pattern,omitempty"`
	FullCharacterReplacement string                   `json:"full_character_replacement,omitempty"`
	Issues                   []string                 `json:"issues,omitempty"`
	TemporalMotion           []TemporalMotionEvidence `json:"temporal_motion,omitempty"`
	FrameEvidence            []VisualQCFrameEvidence  `json:"frame_evidence,omitempty"`
	ViralFrameEvidence       []ViralQCFrameEvidence   `json:"viral_frame_evidence,omitempty"`
}

// VisualQCFrameEvidence makes the model cite what it saw in each ordered
// sample instead of returning only one unsupported overall verdict.
type VisualQCFrameEvidence struct {
	Frame             int    `json:"frame"`
	Shot              int    `json:"shot"`
	ProductVisibility string `json:"product_visibility"`
	ProductUse        string `json:"product_use"`
	CharacterIdentity string `json:"character_identity"`
	Focus             string `json:"focus"`
	Composition       string `json:"composition"`
	EmotionMatch      string `json:"emotion_match"`
	Continuity        string `json:"continuity"`
	Evidence          string `json:"evidence"`
}

type ViralQCFrameEvidence struct {
	Frame                    int    `json:"frame"`
	Segment                  int    `json:"segment"`
	FullCharacterReplacement string `json:"full_character_replacement"`
	CharacterIdentity        string `json:"character_identity"`
	ActionMatch              string `json:"action_match"`
	Focus                    string `json:"focus"`
	Composition              string `json:"composition"`
	Continuity               string `json:"continuity"`
	Evidence                 string `json:"evidence"`
}

type visualImageAnalyzer interface {
	AnalyzeImages(context.Context, string, []ImageRef) (string, error)
}

type sampledVideoFrame struct {
	Ref     ImageRef
	TimeSec float64
	Group   int
}

func sampleVideoForVisualQC(ctx context.Context, videoPath, outDir, prefix string, count int) ([]sampledVideoFrame, error) {
	duration := probeDuration(ctx, videoPath)
	if duration <= 0 {
		return nil, fmt.Errorf("không đọc được thời lượng video để QC")
	}
	if count < 4 {
		count = 4
	}
	if count > 64 {
		count = 64
	}
	times := make([]float64, count)
	groups := make([]int, count)
	for i := 0; i < count; i++ {
		times[i] = duration * (float64(i) + 0.5) / float64(count)
		groups[i] = i + 1
	}
	return sampleVideoAtTimes(ctx, videoPath, outDir, prefix, times, groups)
}

func sampleVideoAtTimes(ctx context.Context, videoPath, outDir, prefix string, times []float64, groups []int) ([]sampledVideoFrame, error) {
	duration := probeDuration(ctx, videoPath)
	if duration <= 0 {
		return nil, fmt.Errorf("không đọc được thời lượng video để QC")
	}
	if len(times) == 0 || len(times) != len(groups) {
		return nil, fmt.Errorf("thiếu timeline QC hợp lệ")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("tạo thư mục frame QC: %w", err)
	}
	frames := make([]sampledVideoFrame, 0, len(times))
	for i, requestedAt := range times {
		at := math.Max(0, math.Min(duration-0.05, requestedAt))
		path := filepath.Join(outDir, fmt.Sprintf("%s-%02d.jpg", prefix, i+1))
		if err := ffmpegRun(ctx, "-ss", fmt.Sprintf("%.3f", at), "-i", videoPath,
			"-vf", "scale=640:-2", "-frames:v", "1", "-q:v", "7", path); err != nil {
			continue
		}
		if st, err := os.Stat(path); err != nil || st.Size() < 100 {
			continue
		}
		frames = append(frames, sampledVideoFrame{Ref: ImageRef{Path: path}, TimeSec: at, Group: groups[i]})
	}
	if len(frames) != len(times) {
		return nil, fmt.Errorf("trích được %d/%d frame QC; thiếu frame ở ít nhất một shot/segment", len(frames), len(times))
	}
	return frames, nil
}

func describeSampledFrames(label string, frames []sampledVideoFrame) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s frames theo thời gian:", label)
	for i, frame := range frames {
		fmt.Fprintf(&b, " %d=%.2fs(group %d)", i+1, frame.TimeSec, frame.Group)
	}
	return b.String()
}

func (s *Studio) analyzeVisualQC(ctx context.Context, prompt string, refs []ImageRef) (VisualQCReport, error) {
	var report VisualQCReport
	var raw string
	var err error
	if analyzer, ok := s.mg.(visualImageAnalyzer); ok && s.mg != nil && s.mg.Healthy(ctx) {
		raw, err = analyzer.AnalyzeImages(ctx, prompt, refs)
	}
	if raw == "" || err != nil {
		paths := make([]string, 0, len(refs))
		for _, ref := range refs {
			paths = append(paths, ref.Path)
		}
		var webErr error
		raw, webErr = s.AnalyzeImagesWithWebAI(ctx, prompt, paths)
		if webErr != nil {
			if err != nil {
				return report, fmt.Errorf("cả Media Engine (%v) và Web AI (%v) đều không phân tích được ảnh QC", err, webErr)
			}
			return report, fmt.Errorf("Web AI không phân tích được ảnh QC: %w", webErr)
		}
	}
	if err := parseDirectorJSON(raw, &report); err != nil {
		return report, fmt.Errorf("QC trả JSON không hợp lệ: %w", err)
	}
	if strings.TrimSpace(report.Summary) == "" {
		return report, fmt.Errorf("QC thiếu summary và bằng chứng")
	}
	return report, nil
}

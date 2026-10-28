package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func (s *Studio) analyzeViralReference(ctx context.Context, videoPath string, duration float64) (string, []ViralReferenceBeat, error) {
	if !fileExists(videoPath) {
		return "", nil, fmt.Errorf("cần video nguồn để quan sát thứ tự cảnh và chuyển động")
	}
	if s.mg == nil || !s.mg.Healthy(ctx) {
		return "", nil, fmt.Errorf("media engine chưa sẵn sàng để phân tích video nguồn")
	}
	analyzer, ok := s.mg.(visualImageAnalyzer)
	if !ok {
		return "", nil, fmt.Errorf("media engine chưa hỗ trợ phân tích frame video nguồn")
	}
	workDir := filepath.Join(s.workRoot, "viral-reference-"+newID())
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("tạo thư mục phân tích video nguồn: %w", err)
	}
	defer os.RemoveAll(workDir)

	frames, err := sampleViralReferenceFrames(ctx, videoPath, workDir, duration)
	if err != nil {
		return "", nil, err
	}
	var prompt strings.Builder
	prompt.WriteString("Analyze these ordered samples from one source video. Images are point samples, not continuous footage. Describe only visible evidence; do not infer unseen choreography, speech, sound, text, or intent. Group observations into chronological beats. Give each beat a source time_sec and concise framing, subject_action, body_movement, camera_movement, edit_transition, energy, and uncertainty. Mark motion between samples uncertain when frames do not prove it.\nReturn valid JSON only with double-quoted keys:\n{\"summary\": \"string\", \"beats\": [{\"time_sec\": 0.0, \"framing\": \"string\", \"subject_action\": \"string\", \"body_movement\": \"string\", \"camera_movement\": \"string\", \"edit_transition\": \"string\", \"energy\": \"string\", \"uncertainty\": \"string\"}]}\n")
	for i, frame := range frames {
		fmt.Fprintf(&prompt, "Image %d shows source time %.3f seconds.\n", i+1, frame.TimeSec)
	}
	refs := make([]ImageRef, 0, len(frames))
	for _, frame := range frames {
		refs = append(refs, ImageRef{Path: frame.Path})
	}
	raw, err := analyzer.AnalyzeImages(ctx, prompt.String(), refs)
	if err != nil {
		return "", nil, fmt.Errorf("phân tích frame video nguồn: %w", err)
	}
	return parseViralReferenceAnalysis(raw, duration)
}

func parseViralReferenceAnalysis(raw string, duration float64) (string, []ViralReferenceBeat, error) {
	var result map[string]json.RawMessage
	cleaned := cleanJSON(raw)
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn trả JSON không hợp lệ: %w", err)
	}
	var summary string
	var beats []ViralReferenceBeat
	if err := json.Unmarshal(result["summary"], &summary); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn thiếu summary hợp lệ: %w", err)
	}
	if err := json.Unmarshal(result["beats"], &beats); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn thiếu beats hợp lệ: %w", err)
	}
	if strings.TrimSpace(summary) == "" {
		summary = "Quan sát các frame nguồn theo thứ tự thời gian"
	}
	for i := range beats {
		beats[i].Framing = strings.TrimSpace(beats[i].Framing)
		beats[i].SubjectAction = strings.TrimSpace(beats[i].SubjectAction)
		beats[i].BodyMovement = strings.TrimSpace(beats[i].BodyMovement)
		beats[i].CameraMovement = strings.TrimSpace(beats[i].CameraMovement)
		beats[i].EditTransition = strings.TrimSpace(beats[i].EditTransition)
		beats[i].Energy = strings.TrimSpace(beats[i].Energy)
		beats[i].Uncertainty = strings.TrimSpace(beats[i].Uncertainty)
	}
	if err := validateViralReferenceBeats(beats, duration); err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(summary), beats, nil
}

func validateViralReferenceBeats(beats []ViralReferenceBeat, duration float64) error {
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return fmt.Errorf("thời lượng video nguồn không hợp lệ")
	}
	if len(beats) == 0 {
		return fmt.Errorf("thiếu beat quan sát từ video nguồn")
	}
	lastTime := -1.0
	for i, beat := range beats {
		if math.IsNaN(beat.TimeSec) || math.IsInf(beat.TimeSec, 0) || beat.TimeSec < 0 || beat.TimeSec > duration || (i > 0 && beat.TimeSec <= lastTime) {
			return fmt.Errorf("beat nguồn %d có timestamp thiếu, ngoài video hoặc sai thứ tự", i+1)
		}
		if strings.TrimSpace(beat.Framing+beat.SubjectAction+beat.BodyMovement) == "" {
			return fmt.Errorf("beat nguồn %d thiếu mô tả hình ảnh quan sát được", i+1)
		}
		lastTime = beat.TimeSec
	}
	return nil
}

func validateViralReferenceScenes(scenes []ViralScene, target float64) error {
	if math.IsNaN(target) || math.IsInf(target, 0) || target <= 0 {
		return fmt.Errorf("thời lượng video nguồn không hợp lệ")
	}
	if len(scenes) == 0 {
		return fmt.Errorf("storyboard nguồn không có cảnh")
	}
	cursor := 0.0
	for i := range scenes {
		scene := &scenes[i]
		if strings.TrimSpace(scene.ImagePrompt) == "" || strings.TrimSpace(scene.VideoPrompt) == "" {
			return fmt.Errorf("storyboard cảnh %d thiếu keyframe hoặc chuyển động theo nguồn", i+1)
		}
		if math.IsNaN(scene.SourceStartSec) || math.IsInf(scene.SourceStartSec, 0) || math.IsNaN(scene.SourceEndSec) || math.IsInf(scene.SourceEndSec, 0) || scene.SourceStartSec < 0 || scene.SourceEndSec > target || math.Abs(scene.SourceStartSec-cursor) > 0.05 {
			return fmt.Errorf("storyboard cảnh %d tạo khoảng trống hoặc chồng lấn với timeline nguồn", i+1)
		}
		if scene.SourceEndSec <= scene.SourceStartSec {
			return fmt.Errorf("storyboard cảnh %d có khoảng timestamp không hợp lệ", i+1)
		}
		sceneDuration := scene.SourceEndSec - scene.SourceStartSec
		if math.IsNaN(scene.DurationSec) || math.IsInf(scene.DurationSec, 0) || scene.DurationSec <= 0 || math.Abs(scene.DurationSec-sceneDuration) > 0.05 {
			return fmt.Errorf("storyboard cảnh %d khai duration không khớp khoảng nguồn", i+1)
		}
		scene.Index = i + 1
		scene.Phase = "source_beat"
		scene.DurationSec = sceneDuration
		cursor = scene.SourceEndSec
	}
	if math.Abs(cursor-target) > 0.05 {
		return fmt.Errorf("timeline storyboard kết thúc ở %.3f giây, nguồn dài %.3f giây", cursor, target)
	}
	return nil
}

type viralSampleFrame struct {
	Path    string
	TimeSec float64
}

func isViralVideoFile(path string) bool {
	if path == "" || !fileExists(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi":
		return true
	default:
		return false
	}
}

func sampleViralReferenceFrames(ctx context.Context, videoPath, outDir string, duration float64) ([]viralSampleFrame, error) {
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return nil, fmt.Errorf("không đọc được thời lượng video")
	}
	count := int(math.Ceil(duration * 2)) // two ordered source samples per second
	if count < 8 {
		count = 8
	}
	if count > 120 {
		count = 120
	}
	frames := make([]viralSampleFrame, 0, count)
	for i := 0; i < count; i++ {
		at := duration * (float64(i) + 0.5) / float64(count)
		path := filepath.Join(outDir, fmt.Sprintf("viral-reference-%02d.jpg", i+1))
		if err := ffmpegRun(ctx, "-ss", fmt.Sprintf("%.3f", at), "-i", videoPath, "-vf", "scale=640:-2", "-frames:v", "1", "-q:v", "7", path); err != nil || !fileExists(path) {
			continue
		}
		frames = append(frames, viralSampleFrame{Path: path, TimeSec: at})
	}
	if len(frames) < 4 {
		return nil, fmt.Errorf("chỉ trích được %d frame; cần ít nhất 4", len(frames))
	}
	return frames, nil
}

// AnalyzeViralOriginal reads technical metadata. Motion, mood, and pacing
// remain unknown until an image/video analyzer measures them.
func AnalyzeViralOriginal(ctx context.Context, samplePath, outDir string) *ViralOriginalProfile {
	prof := &ViralOriginalProfile{
		Path:     samplePath,
		Duration: probeDuration(ctx, samplePath),
		HasAudio: ProbeHasAudio(ctx, samplePath),
	}

	w, h := ProbeDims(ctx, samplePath)
	prof.Width = w
	prof.Height = h

	// Trích xuất âm thanh gốc nếu video có audio stream
	if prof.HasAudio && outDir != "" {
		audioFile := filepath.Join(outDir, "viral_orig_audio.m4a")
		if err := ffmpegRun(ctx, "-i", samplePath, "-vn", "-c:a", "aac", "-b:a", "192k", audioFile); err == nil && fileExists(audioFile) {
			prof.AudioExtracted = audioFile
		}
	}

	return prof
}

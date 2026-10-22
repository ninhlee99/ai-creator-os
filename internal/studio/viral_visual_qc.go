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

func (s *Studio) evaluateViralVideo(ctx context.Context, videoPath, workDir string, p ViralParams, modelPhotos []string) (VisualQCReport, error) {
	var empty VisualQCReport
	modelPhotos = validCharacterPhotos(modelPhotos, 4)
	if !isViralVideoFile(p.SampleVideo) || len(modelPhotos) == 0 {
		return empty, fmt.Errorf("QC viral cần video nguồn và ảnh nhân vật kênh")
	}
	sourceDuration := probeDuration(ctx, p.SampleVideo)
	times, segmentIDs, err := viralSegmentSampleTimeline(sourceDuration)
	if err != nil {
		return empty, err
	}
	sourceFrames, err := sampleVideoAtTimes(ctx, p.SampleVideo, filepath.Join(workDir, "qc-viral-source"), "source", times, segmentIDs)
	if err != nil {
		return empty, fmt.Errorf("trích frame nguồn: %w", err)
	}
	outputFrames, err := sampleVideoAtTimes(ctx, videoPath, filepath.Join(workDir, "qc-viral-output"), "output", times, segmentIDs)
	if err != nil {
		return empty, fmt.Errorf("trích frame đầu ra: %w", err)
	}
	refs := make([]ImageRef, 0, len(modelPhotos)+len(sourceFrames)+len(outputFrames))
	for _, photo := range modelPhotos {
		refs = append(refs, ImageRef{Path: photo})
	}
	for _, frame := range sourceFrames {
		refs = append(refs, frame.Ref)
	}
	for _, frame := range outputFrames {
		refs = append(refs, frame.Ref)
	}
	identityEnd := len(modelPhotos)
	sourceEnd := identityEnd + len(sourceFrames)
	prompt := fmt.Sprintf(
		"Compare ordered channel-character references and two video frame sequences. Images 1-%d are the same channel character from different reference angles. Images %d-%d are source frames in time order. Remaining %d images are output frames at matching timestamps, in time order. Topic: %q. %s %s\n"+
			"Judge whether the entire main person was replaced with the channel character across face, hair, and visible body identity; face-only replacement is failure. The full-body look may adapt to source pose, but output must not retain original actor identity. Compare source and output frame-by-frame: preserve scene order, body choreography/action, timing, camera movement, framing, and cuts. Judge visible evidence only. Still samples cannot prove continuous motion.\n"+
			"Each edit segment has five ordered source and output samples across the segment at matching timestamps. Judge every sampled action independently; do not infer between samples.\n"+
			"Return JSON only with fields: decision (pass|fail|uncertain), summary, character_identity (match|mismatch|uncertain), full_character_replacement (complete|partial|face_only|not_replaced|uncertain), scene_order (match|mismatch|uncertain), action_sequence (match|partial|mismatch|uncertain), camera_pattern (match|partial|mismatch|uncertain), motion (visible|still|uncertain), issues (array), viral_frame_evidence (one item for every output frame in order: frame integer, segment integer, full_character_replacement complete|partial|face_only|not_replaced|uncertain, action_match match|partial|mismatch|uncertain, evidence short visible comparison). Include all %d output frames and preserve segment numbers. Set decision=pass only when full_character_replacement=complete, character_identity=match, scene_order=match, action_sequence=match, camera_pattern=match, motion=visible, and every viral_frame_evidence item says complete replacement and matching action. No numeric quality score.",
		identityEnd, identityEnd+1, sourceEnd, len(outputFrames), p.Topic,
		describeSampledFrames("Source", sourceFrames), describeSampledFrames("Output", outputFrames), len(outputFrames))
	report, err := s.analyzeVisualQC(ctx, prompt, refs)
	if err != nil {
		return report, err
	}
	if !viralQCFieldsPass(report, len(outputFrames), len(segmentIDs)/viralQCFramesPerSegment) {
		return report, fmt.Errorf("QC viral chưa chứng minh thay toàn nhân vật hoặc giữ đúng cảnh/choreography: %s", report.Summary)
	}
	return report, nil
}

func viralSegmentSampleTimeline(duration float64) ([]float64, []int, error) {
	if duration < 3 {
		return nil, nil, fmt.Errorf("QC viral cần video nguồn dài ít nhất 3s")
	}
	const maxSegmentSeconds = 10.0
	segmentCount := int(math.Ceil(duration / maxSegmentSeconds))
	segmentDuration := duration / float64(segmentCount)
	if segmentDuration < 3 || segmentDuration > maxSegmentSeconds {
		return nil, nil, fmt.Errorf("không thể tạo timeline QC theo segment 3-10s cho video %.2fs", duration)
	}
	var times []float64
	var segments []int
	for i := 0; i < segmentCount; i++ {
		start := float64(i) * segmentDuration
		for sample := 1; sample <= viralQCFramesPerSegment; sample++ {
			fraction := float64(sample) / float64(viralQCFramesPerSegment+1)
			times = append(times, start+segmentDuration*fraction)
			segments = append(segments, i+1)
		}
	}
	return times, segments, nil
}

const viralQCFramesPerSegment = 5

func (s *Studio) runViralVisualGate(ctx context.Context, jobID, videoPath, workDir string, p ViralParams, modelPhotos []string) error {
	s.appendLog(jobID, "VISUAL_QC_START: đang so sánh chuỗi frame nguồn và đầu ra, kiểm tra toàn nhân vật")
	report, err := s.evaluateViralVideo(ctx, videoPath, workDir, p, modelPhotos)
	if writeErr := writeVisualQCReport(filepath.Join(workDir, "visual-qc.json"), report); writeErr != nil {
		s.appendLog(jobID, "Không lưu được báo cáo visual QC: "+writeErr.Error())
	}
	if err != nil {
		s.appendLog(jobID, "VISUAL_QC_FAIL: "+err.Error())
		return err
	}
	s.appendLog(jobID, "VISUAL_QC_PASS: "+report.Summary)
	return nil
}

func viralQCFieldsPass(r VisualQCReport, expectedFrames, expectedSegments int) bool {
	if !strings.EqualFold(r.Decision, "pass") || len(r.Issues) != 0 ||
		!strings.EqualFold(r.CharacterIdentity, "match") ||
		!strings.EqualFold(r.FullCharacterReplacement, "complete") ||
		!strings.EqualFold(r.SceneOrder, "match") ||
		!strings.EqualFold(r.ActionSequence, "match") ||
		!strings.EqualFold(r.CameraPattern, "match") ||
		!strings.EqualFold(r.Motion, "visible") {
		return false
	}
	if expectedSegments < 1 || expectedFrames != expectedSegments*viralQCFramesPerSegment || len(r.ViralFrameEvidence) != expectedFrames {
		return false
	}
	for i, evidence := range r.ViralFrameEvidence {
		if evidence.Frame != i+1 ||
			evidence.Segment != i/viralQCFramesPerSegment+1 ||
			!strings.EqualFold(evidence.FullCharacterReplacement, "complete") ||
			!strings.EqualFold(evidence.ActionMatch, "match") ||
			strings.TrimSpace(evidence.Evidence) == "" {
			return false
		}
	}
	return true
}

func writeVisualQCReport(path string, report VisualQCReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

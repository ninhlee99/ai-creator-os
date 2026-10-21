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
	ActionMatch              string `json:"action_match"`
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
	analyzer, ok := s.mg.(visualImageAnalyzer)
	if !ok {
		return report, fmt.Errorf("media engine không hỗ trợ phân tích hình ảnh để QC")
	}
	raw, err := analyzer.AnalyzeImages(ctx, prompt, refs)
	if err != nil {
		return report, fmt.Errorf("phân tích frame QC: %w", err)
	}
	if err := parseDirectorJSON(raw, &report); err != nil {
		return report, fmt.Errorf("QC trả JSON không hợp lệ: %w", err)
	}
	if strings.TrimSpace(report.Summary) == "" {
		return report, fmt.Errorf("QC thiếu summary và bằng chứng")
	}
	return report, nil
}

func (s *Studio) evaluateAffiliateVideo(ctx context.Context, videoPath, workDir string, p AffiliateParams, profile AffiliateProductProfile, shots []ProShot) (VisualQCReport, error) {
	var empty VisualQCReport
	if !fileExists(p.ProductPhoto) || !fileExists(p.ModelPhoto) {
		return empty, fmt.Errorf("QC affiliate cần ảnh sản phẩm và ảnh nhân vật gốc")
	}
	if len(shots) == 0 {
		return empty, fmt.Errorf("QC affiliate thiếu shot list để kiểm tra coverage từng cảnh")
	}
	times, shotGroups := affiliateShotSampleTimeline(shots)
	frames, err := sampleVideoAtTimes(ctx, videoPath, filepath.Join(workDir, "qc-affiliate"), "output", times, shotGroups)
	if err != nil {
		return empty, err
	}
	productRefs := affiliateProductReferences(p)
	modelRefs := affiliateModelReferences(p, 4)
	refs := append([]ImageRef(nil), productRefs...)
	refs = append(refs, modelRefs...)
	for _, frame := range frames {
		refs = append(refs, frame.Ref)
	}
	identityStart := len(productRefs) + 1
	identityEnd := identityStart + len(modelRefs) - 1
	outputStart := len(productRefs) + len(modelRefs) + 1
	prompt := fmt.Sprintf(
		"Evaluate finished affiliate video against ordered listing and channel identity references. Images 1-%d are listing angles of the same exact product. Images %d-%d are the same channel character from different angles. Images %d onward are ordered output frames. Product: %q. Category: %q. Required use: %q. Focus: %q. Campaign wardrobe: %q. Campaign setting: %q. Shot emotional beats and framing: %s.\n"+
			"Inspect each output frame independently. Check exact item/category and visible details, required product_use_mode for that shot, correct physical use, product visibility, subject focus/sharpness, clean commercial composition, channel identity, the planned emotional beat, coherent outfit/location/light, and visible movement across the samples. Do not assume an item is worn/used because it is nearby. Apparel is worn, footwear on feet, bags carried/worn, jewelry worn, cosmetics applied, home/tech items operated. Do not infer unsupported product claims.\n"+
			"Three ordered frames represent each shot (early, middle, late); frame numbers are 1-%d and shot numbers are 1-%d. %s. Return JSON only: decision (pass|fail|uncertain), summary, product_match (match|mismatch|uncertain), product_usage (correct|not_shown|incorrect|uncertain), product_visibility (clear|weak|absent), character_identity (match|mismatch|uncertain), motion (visible|still|uncertain), shot_variety (varied|repetitive|uncertain), sharpness (sharp|soft|blurred|uncertain), composition (professional|cluttered|weak|uncertain), emotional_arc (coherent|flat|uncertain), continuity (consistent|inconsistent|uncertain), issues (blocking issue array), frame_evidence (one item per output frame: frame integer, shot integer, product_visibility clear|weak|absent, product_use correct|not_shown|incorrect|uncertain, character_identity match|mismatch|not_visible|uncertain, focus sharp|soft|blurred|uncertain, composition professional|cluttered|weak|uncertain, emotion_match match|partial|mismatch|not_applicable|uncertain, continuity match|mismatch|not_applicable|uncertain, evidence short visible description). For a planned macro/ECU frame without a face, identity and emotion may be not_visible/not_applicable; otherwise assess them. Pass only if all three frames in every shot show the exact product clearly and show it being worn or used in the specified product_use_mode, no sampled product use is incorrect, every frame is sharp and professionally composed, each non-macro shot has identity and emotion evidence, continuity holds, all overall fields pass, and evidence covers every frame. Samples do not prove every video frame. No numeric score.",
		len(productRefs), identityStart, identityEnd, outputStart, p.ProductName, profile.Category,
		profile.UseAction, profile.Focus, shots[0].WardrobeStyling, shots[0].SceneSetting,
		describeAffiliateShots(shots), len(frames), len(shots), describeSampledFrames("Output", frames))
	report, err := s.analyzeVisualQC(ctx, prompt, refs)
	if err != nil {
		return report, err
	}
	if !affiliateQCFieldsPass(report, len(frames), shots) {
		return report, fmt.Errorf("QC affiliate chưa đạt: %s", report.Summary)
	}
	return report, nil
}

func affiliateShotSampleTimeline(shots []ProShot) ([]float64, []int) {
	var times []float64
	var groups []int
	cursor := 0.0
	for i, shot := range shots {
		duration := float64(shot.Seconds)
		times = append(times, cursor+duration*0.2, cursor+duration*0.5, cursor+duration*0.8)
		groups = append(groups, i+1, i+1, i+1)
		cursor += duration
	}
	return times, groups
}

func describeAffiliateShots(shots []ProShot) string {
	var b strings.Builder
	for _, shot := range shots {
		fmt.Fprintf(&b, " shot %d=%s/%s, use_mode=%q, emotion=%q, action=%q, product=%q, focus=%q;", shot.Index, shot.Purpose, shot.ShotSize, shot.ProductUseMode, shot.EmotionBeat, shot.Action, shot.ProductInteraction, shot.ProductFocus)
	}
	return strings.TrimSpace(b.String())
}

func (s *Studio) runAffiliateVisualGate(ctx context.Context, jobID, videoPath, workDir string, p AffiliateParams, profile AffiliateProductProfile, shots []ProShot) error {
	s.appendLog(jobID, "MOTION_QC_START: đo thay đổi frame liên tiếp trong từng shot")
	motion, motionErr := measureAffiliateTemporalMotion(ctx, videoPath, shots)
	report := VisualQCReport{TemporalMotion: motion}
	if motionErr == nil && !affiliateTemporalMotionPass(motion, len(shots)) {
		motionErr = fmt.Errorf("có shot thiếu chuyển động liên tục trong frame mẫu")
	}
	if motionErr != nil {
		if writeErr := writeVisualQCReport(filepath.Join(workDir, "visual-qc.json"), report); writeErr != nil {
			s.appendLog(jobID, "Không lưu được báo cáo visual QC: "+writeErr.Error())
		}
		s.appendLog(jobID, "MOTION_QC_FAIL: "+motionErr.Error())
		return motionErr
	}
	s.appendLog(jobID, "MOTION_QC_PASS: mọi shot có thay đổi frame đủ ngưỡng; kết quả này không tự chứng minh hành động dùng sản phẩm")
	s.appendLog(jobID, "VISUAL_QC_START: so 3 frame mỗi shot với ảnh sản phẩm/nhân vật; kiểm độ nét, bố cục, cảm xúc và continuity")
	report, err := s.evaluateAffiliateVideo(ctx, videoPath, workDir, p, profile, shots)
	report.TemporalMotion = motion
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

func affiliateQCFieldsPass(r VisualQCReport, expectedFrames int, shots []ProShot) bool {
	expectedShots := len(shots)
	if !strings.EqualFold(r.Decision, "pass") ||
		len(r.Issues) != 0 ||
		!strings.EqualFold(r.ProductMatch, "match") ||
		!strings.EqualFold(r.ProductUsage, "correct") ||
		!strings.EqualFold(r.ProductVisibility, "clear") ||
		!strings.EqualFold(r.CharacterIdentity, "match") ||
		!strings.EqualFold(r.Motion, "visible") ||
		!strings.EqualFold(r.ShotVariety, "varied") ||
		!strings.EqualFold(r.Sharpness, "sharp") ||
		!strings.EqualFold(r.Composition, "professional") ||
		!strings.EqualFold(r.EmotionalArc, "coherent") ||
		!strings.EqualFold(r.Continuity, "consistent") {
		return false
	}
	if expectedShots < 4 || expectedFrames != expectedShots*3 || len(r.FrameEvidence) != expectedFrames {
		return false
	}
	clearAndUsed := make([]int, expectedShots)
	shotHasIdentity := make([]bool, expectedShots)
	shotHasEmotion := make([]bool, expectedShots)
	shotIsMacro := make([]bool, expectedShots)
	for i, shot := range shots {
		shotIsMacro[i] = shot.Purpose == "macro" || strings.EqualFold(shot.ShotSize, "ECU")
	}
	for i, frame := range r.FrameEvidence {
		expectedShot := i/3 + 1
		if frame.Frame != i+1 || frame.Shot != expectedShot || strings.TrimSpace(frame.Evidence) == "" {
			return false
		}
		if strings.EqualFold(frame.ProductVisibility, "clear") && strings.EqualFold(frame.ProductUse, "correct") {
			clearAndUsed[expectedShot-1]++
		}
		if strings.EqualFold(frame.ProductUse, "incorrect") ||
			!strings.EqualFold(frame.Focus, "sharp") ||
			!strings.EqualFold(frame.Composition, "professional") {
			return false
		}
		if strings.EqualFold(frame.CharacterIdentity, "mismatch") || strings.EqualFold(frame.EmotionMatch, "mismatch") || strings.EqualFold(frame.Continuity, "mismatch") {
			return false
		}
		if strings.EqualFold(frame.CharacterIdentity, "match") {
			shotHasIdentity[expectedShot-1] = true
		}
		if strings.EqualFold(frame.EmotionMatch, "match") {
			shotHasEmotion[expectedShot-1] = true
		}
	}
	for i := range shots {
		if clearAndUsed[i] < 3 {
			return false
		}
		if !shotIsMacro[i] && (!shotHasIdentity[i] || !shotHasEmotion[i]) {
			return false
		}
	}
	return true
}

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

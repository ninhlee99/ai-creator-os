package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

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

package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

func (s *Studio) runAffiliateWorkflow(ctx context.Context, jobID, workDir string, p AffiliateParams, productProfile AffiliateProductProfile, shots []ProShot, museCreditsAcknowledged bool) (string, error) {
	musicPath, err := downloadTikTokRadarAudio(ctx, p.SourceMusicVideoURL, workDir)
	if err != nil {
		return "", fmt.Errorf("không tải được sound Radar: %w", err)
	}
	store, err := s.GetFlowStore()
	if err != nil {
		return "", err
	}
	graph, err := store.GetGraph("preset_affiliate_showcase")
	if err != nil {
		return "", fmt.Errorf("không tải được preset Affiliate Workflow: %w", err)
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if node.Params == nil {
			node.Params = map[string]any{}
		}
		switch node.ID {
		case "char":
			node.Params["account_id"] = p.AccountID
			node.Params["model_path"] = p.ModelPhoto
			node.Params["model_paths"] = append([]string{p.ModelPhoto}, p.ModelPhotos...)
			node.Params["model_name"] = strings.TrimSuffix(filepath.Base(p.ModelPhoto), filepath.Ext(p.ModelPhoto))
		case "product":
			productID := ""
			if p.ProductID > 0 {
				productID = fmt.Sprint(p.ProductID)
			}
			node.Params["product_id"] = productID
			node.Params["product_image"] = p.ProductPhoto
			node.Params["product_images"] = append([]string(nil), p.ExtraPhotos...)
			node.Params["product_title"] = p.ProductName
			node.Params["affiliate_url"] = p.AffLink
		case "music":
			node.Params["audio_path"] = musicPath
			node.Params["audio_title"] = p.SourceMusicTitle
		case "export":
			node.Params["title"] = p.ProductName
		case "outfit":
			node.Params["outfit_name"] = p.ProductName
		}
	}
	shotInputs := make([]any, 0, len(shots))
	for i, shot := range shots {
		item := map[string]any{
			"duration_seconds": float64(shot.Seconds),
			"variables": map[string]any{
				"duration_seconds":    shot.Seconds,
				"product_name":        p.ProductName,
				"product_use_mode":    shot.ProductUseMode,
				"product_focus":       shot.ProductFocus,
				"action":              shot.Action,
				"product_interaction": shot.ProductInteraction,
				"framing":             shot.ShotSize,
				"camera_move":         shot.CameraMove,
				"lens_light":          shot.LensLight,
				"emotion_beat":        shot.EmotionBeat,
				"wardrobe_styling":    shot.WardrobeStyling,
				"scene_setting":       shot.SceneSetting,
				"motion_direction":    shot.VideoPrompt,
				"product_category":    productProfile.Category,
				"product_description": strings.Join(productProfile.VisibleAttributes, ", "),
				"product_use_action":  productProfile.UseAction,
				"product_notes":       strings.Join(productProfile.OperatorNotes, ", "),
			},
		}
		if i < len(p.Clips) && strings.TrimSpace(p.Clips[i]) != "" {
			item["segment_video"] = p.Clips[i]
		}
		shotInputs = append(shotInputs, item)
	}
	inputs := map[string]any{
		"shots":      shotInputs,
		"duration":   float64(p.Seconds),
		"resolution": p.OutputResolution,
	}
	if strings.TrimSpace(p.VoiceoverPath) != "" {
		if !fileExists(p.VoiceoverPath) || !isStudioAudioFile(p.VoiceoverPath) {
			return "", fmt.Errorf("voiceover không phải file audio hợp lệ")
		}
		inputs["voice_audio"] = p.VoiceoverPath
	}
	runID := "affiliate-" + safeAffiliateRunID(jobID)
	run, err := s.ExecuteFlowGraphWithInputs(ctx, runID, graph, inputs, flow.ExecutionServices{MuseCreditsAcknowledged: museCreditsAcknowledged})
	if err != nil {
		return "", fmt.Errorf("Workflow API Affiliate: %w", err)
	}
	if run == nil || run.Status != "completed" {
		status := "không có trạng thái"
		if run != nil {
			status = run.Status
		}
		return "", fmt.Errorf("Workflow API Affiliate kết thúc với trạng thái %s", status)
	}
	workflowVideo, ok := run.Outputs["video"].(string)
	if !ok || !fileExists(workflowVideo) || !isViralVideoFile(workflowVideo) {
		return "", fmt.Errorf("Workflow API Affiliate không trả video output hợp lệ")
	}
	if err := verifyVideoDuration(ctx, workflowVideo, float64(p.Seconds)); err != nil {
		return "", fmt.Errorf("video Affiliate sai thời lượng: %w", err)
	}
	if !ProbeHasAudio(ctx, workflowVideo) {
		return "", fmt.Errorf("video Affiliate không có audio sau khi chạy workflow")
	}
	if p.OutputResolution != "" {
		width, height := ProbeDims(ctx, workflowVideo)
		wantWidth, wantHeight, _ := VideoOutputDims(p.AspectRatio, p.OutputResolution)
		if width != wantWidth || height != wantHeight {
			return "", fmt.Errorf("video Affiliate sai độ phân giải: nhận %dx%d, cần %dx%d", width, height, wantWidth, wantHeight)
		}
	}
	finalPath := filepath.Join(s.outDir, "studio-"+safeAffiliateRunID(jobID)+".mp4")
	_ = os.Remove(finalPath)
	if err := copyStudioFile(workflowVideo, finalPath); err != nil {
		return "", fmt.Errorf("chép video Affiliate thành phẩm: %w", err)
	}
	if eval, evalErr := s.EvaluateVideoQuality(ctx, finalPath, workDir, nil); evalErr == nil && eval != nil {
		s.appendLog(jobID, fmt.Sprintf("VIDEO_QUALITY_QC: Điểm %d/100 (%s) · %s · %dx%d · %.1fs",
			eval.OverallScore, eval.Status, eval.Verdict, eval.Width, eval.Height, eval.DurationSec))
	}
	return finalPath, nil
}

func safeAffiliateRunID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "job"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, id)
}

func isStudioAudioFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".aac", ".wav", ".m4a", ".ogg", ".flac":
		return true
	default:
		return false
	}
}

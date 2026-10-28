package flow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIPromptCraft(t *testing.T) {
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Log:     func(string) {},
	}

	node := &FlowNode{
		ID:   "craft_1",
		Type: "ai_prompt_craft",
		Params: map[string]any{
			"template":     "Video of {{character}} showing {{product}}. Action: {{context}}.",
			"style_preset": "commercial_ad",
		},
	}

	inputs := map[string]any{
		"character_text": "Linh (22 tuổi)",
		"product_text":   "Son kem lì Maybelline",
		"context_text":   "Thoa son nhẹ nhàng lên môi trước gương",
		"variables": map[string]any{
			"lighting": "warm studio glow",
		},
	}

	res, err := executeAIPromptCraft(ctx, node, inputs)
	if err != nil {
		t.Fatalf("executeAIPromptCraft failed: %v", err)
	}

	promptOut, ok := res["prompt_out"].(string)
	if !ok || promptOut == "" {
		t.Fatalf("expected valid prompt_out, got %v", res["prompt_out"])
	}

	if !strings.Contains(promptOut, "Linh (22 tuổi)") || !strings.Contains(promptOut, "Son kem lì Maybelline") {
		t.Errorf("prompt_out missing interpolated values: %s", promptOut)
	}

	if !strings.Contains(promptOut, "commercial product showcase") {
		t.Errorf("prompt_out missing commercial_ad style preset: %s", promptOut)
	}

	// Test override
	node.Params["prompt_override"] = "Custom prompt override directly"
	res2, err := executeAIPromptCraft(ctx, node, inputs)
	if err != nil {
		t.Fatalf("override test failed: %v", err)
	}
	if !strings.Contains(res2["prompt_out"].(string), "Custom prompt override directly") {
		t.Errorf("override failed, got: %s", res2["prompt_out"])
	}
}

func TestAIVideoGenerate(t *testing.T) {
	workDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: workDir,
		Log:     func(string) {},
		EngineCall: func(c context.Context, platform, prompt string, images []string, aspect, projectID, accountID, outPath string) (string, string, error) {
			_ = writeDummyVideo(&ExecutionContext{WorkDir: workDir}, outPath, 4.0)
			return "proj_1", "acc_1", nil
		},
		FFmpegRunner: func(c context.Context, args ...string) error {
			// Mock frame extractor for thumbnail
			for i, arg := range args {
				if arg == "-vframes" && i+2 < len(args) {
					outImg := args[len(args)-1]
					_ = writeDummyImage(outImg, 720, 1280, "THUMB")
					return nil
				}
			}
			return nil
		},
	}

	node := &FlowNode{
		ID:   "video_gen_1",
		Type: "ai_video_generate",
		Params: map[string]any{
			"provider_mode": "web_ai",
			"provider":      "google_flow",
			"model":         "veo-3.1-fast",
			"aspect_ratio":  "9:16",
		},
	}

	inputs := map[string]any{
		"prompt_in": "A stunning dance in neon lights",
	}

	res, err := executeAIVideoGenerate(ctx, node, inputs)
	if err != nil {
		t.Fatalf("executeAIVideoGenerate failed: %v", err)
	}

	videoOut, ok := res["video_out"].(string)
	if !ok || !fileExists(videoOut) {
		t.Fatalf("expected existing video_out file, got %v", videoOut)
	}

	meta, ok := res["meta_out"].(map[string]any)
	if !ok || meta["provider"] != "google_flow" {
		t.Fatalf("expected valid meta_out, got %v", meta)
	}
}

func TestAIImageGenerate(t *testing.T) {
	workDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: workDir,
		Log:     func(string) {},
		GenerateImage: func(c context.Context, prompt string, imagePaths []string, outPath string) error {
			return writeDummyImage(outPath, 1024, 1024, "AI_IMAGE")
		},
	}

	node := &FlowNode{
		ID:   "img_gen_1",
		Type: "ai_image_generate",
		Params: map[string]any{
			"provider_mode": "api_key",
			"provider":      "openai",
			"model":         "dall-e-3",
			"aspect_ratio":  "1:1",
			"custom_prompt": "Beautiful cosmetics product packaging on marble surface",
		},
	}

	res, err := executeAIImageGenerate(ctx, node, map[string]any{})
	if err != nil {
		t.Fatalf("executeAIImageGenerate failed: %v", err)
	}

	imgOut, ok := res["image_out"].(string)
	if !ok || !fileExists(imgOut) {
		t.Fatalf("expected existing image_out file, got %v", imgOut)
	}
}

func TestAIVisionAnalyzer(t *testing.T) {
	workDir := t.TempDir()
	dummyImg := filepath.Join(workDir, "test_prod.jpg")
	_ = writeDummyImage(dummyImg, 800, 800, "PRODUCT")

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: workDir,
		Log:     func(string) {},
		AnalyzeImages: func(c context.Context, prompt string, imagePaths []string) (string, error) {
			return "Sản phẩm là chai serum thủy tinh màu hổ phách, nắp nhỏ giọt cao cấp, nhãn vàng đồng sang trọng.", nil
		},
	}

	node := &FlowNode{
		ID:   "vision_1",
		Type: "ai_vision_analyzer",
		Params: map[string]any{
			"provider_mode": "web_ai",
			"provider":      "gemini",
			"task_type":     "extract_product",
		},
	}

	inputs := map[string]any{
		"image_in": dummyImg,
	}

	res, err := executeAIVisionAnalyzer(ctx, node, inputs)
	if err != nil {
		t.Fatalf("executeAIVisionAnalyzer failed: %v", err)
	}

	analysis, ok := res["analysis_text"].(string)
	if !ok || analysis == "" {
		t.Fatalf("expected analysis_text, got %v", res["analysis_text"])
	}

	optPrompt, ok := res["optimized_prompt"].(string)
	if !ok || optPrompt == "" {
		t.Fatalf("expected optimized_prompt, got %v", res["optimized_prompt"])
	}
}

func TestAITextLLM(t *testing.T) {
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Log:     func(string) {},
		GenerateText: func(c context.Context, systemPrompt, prompt string) (string, error) {
			return `{"hook": "Bí quyết dưỡng da chuẩn Hàn", "call_to_action": "Bấm vào giỏ hàng ngay"}`, nil
		},
	}

	node := &FlowNode{
		ID:   "text_llm_1",
		Type: "ai_text_llm",
		Params: map[string]any{
			"provider_mode": "api_key",
			"provider":      "chatgpt",
			"model":         "gpt-4o",
			"custom_prompt": "Viết hook bán hàng affiliate cho serum",
		},
	}

	res, err := executeAITextLLM(ctx, node, map[string]any{})
	if err != nil {
		t.Fatalf("executeAITextLLM failed: %v", err)
	}

	textOut, ok := res["text_out"].(string)
	if !ok || textOut == "" {
		t.Fatalf("expected text_out, got %v", res["text_out"])
	}

	jsonMap, ok := res["json_out"].(map[string]any)
	if !ok || jsonMap["hook"] != "Bí quyết dưỡng da chuẩn Hàn" {
		t.Fatalf("expected parsed json_out, got %v", jsonMap)
	}
}

func TestVideoQCEvaluator(t *testing.T) {
	workDir := t.TempDir()
	dummyVid := filepath.Join(workDir, "qc_test.mp4")
	_ = writeDummyVideo(&ExecutionContext{WorkDir: workDir}, dummyVid, 5.0)

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: workDir,
		Log:     func(string) {},
		EvaluateVideo: func(c context.Context, videoPath, dir string) (map[string]any, error) {
			return map[string]any{
				"overall_score": 92,
				"verdict":       "PASSED",
				"status":        "passed",
				"summary":       "Video sắc nét, âm thanh đồng bộ tuyệt vời.",
			}, nil
		},
	}

	node := &FlowNode{
		ID:   "qc_1",
		Type: "video_qc_evaluator",
		Params: map[string]any{
			"min_score":   70,
			"strict_mode": true,
		},
	}

	inputs := map[string]any{
		"video_in": dummyVid,
	}

	res, err := executeVideoQCEvaluator(ctx, node, inputs)
	if err != nil {
		t.Fatalf("executeVideoQCEvaluator failed: %v", err)
	}

	if res["overall_score"] != 92 {
		t.Errorf("expected score 92, got %v", res["overall_score"])
	}
	if res["verdict"] != "PASSED" {
		t.Errorf("expected verdict PASSED, got %v", res["verdict"])
	}
	if res["video_out"] != dummyVid {
		t.Errorf("expected video_out passthrough, got %v", res["video_out"])
	}

	// Test strict mode failure
	node.Params["min_score"] = 95
	_, err = executeVideoQCEvaluator(ctx, node, inputs)
	if err == nil {
		t.Errorf("expected strict mode failure when score < min_score")
	}
}

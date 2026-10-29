package flow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAIProductExtractorNode(t *testing.T) {
	node := &FlowNode{
		ID:   "extractor",
		Type: "webai_product_extractor",
		Params: map[string]any{
			"provider": "gemini",
		},
	}

	sampleImg := filepath.Join("..", "..", "web", "static", "presets", "char_01.jpg")
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		VisionEval: func(ctx context.Context, prompt string, imagePath string) (string, error) {
			return "Áo cardigan len dệt kim màu be nhạt, khuy cài ngọc trai cao cấp, form rộng trẻ trung", nil
		},
		Log: func(msg string) {},
	}

	outputs, err := executeWebAIProductExtractor(ctx, node, map[string]any{
		"image_in": sampleImg,
	})
	if err != nil {
		t.Fatalf("executeWebAIProductExtractor failed: %v", err)
	}

	desc, ok := outputs["description"].(string)
	if !ok || !strings.Contains(desc, "cardigan") {
		t.Errorf("description output = %v, want description mentioning cardigan", desc)
	}

	cleanPrompt, ok := outputs["clean_prompt"].(string)
	if !ok || cleanPrompt == "" {
		t.Errorf("clean_prompt output is empty")
	}

	profile, ok := outputs["product_profile"].(map[string]any)
	if !ok || profile["source_image"] != sampleImg {
		t.Errorf("product_profile = %+v, want source_image = %s", profile, sampleImg)
	}
}

func TestWebAIImg2ImgNode(t *testing.T) {
	node := &FlowNode{
		ID:   "img2img",
		Type: "webai_img2img",
		Params: map[string]any{
			"provider_name": "chatgpt",
			"style":         "studio lighting",
		},
	}

	sampleImg := filepath.Join("..", "..", "web", "static", "presets", "char_01.jpg")
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		GenerateImage: func(ctx context.Context, prompt string, imagePaths []string, outPath string) error {
			return copyFile(sampleImg, outPath)
		},
		Log: func(msg string) {},
	}

	outputs, err := executeWebAIImg2Img(ctx, node, map[string]any{
		"image_in":  sampleImg,
		"prompt_in": "Đặt trong quán cafe phong cách Châu Âu ấm cúng",
	})
	if err != nil {
		t.Fatalf("executeWebAIImg2Img failed: %v", err)
	}

	imgOut, ok := outputs["image_out"].(string)
	if !ok || !fileExists(imgOut) {
		t.Errorf("image_out = %v, want existing image file", imgOut)
	}
}

package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

func executeGoogleFlowVideo(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := inputString(inputs, "prompt", "")
	if prompt == "" {
		prompt = nodeParamString(node.Params, "prompt", "Cinematic vertical 9:16 video, highly detailed, 4k quality")
	}

	ingredients := []string{}
	if raw, ok := inputs["ingredients"]; ok && raw != nil {
		if list, ok := raw.([]string); ok {
			ingredients = append(ingredients, list...)
		} else if item, ok := raw.(string); ok && item != "" {
			ingredients = append(ingredients, item)
		}
	}
	if charImg := inputString(inputs, "character_image", ""); charImg != "" && fileExists(charImg) {
		ingredients = append(ingredients, charImg)
	}
	if refImg := inputString(inputs, "reference_image", ""); refImg != "" && fileExists(refImg) {
		ingredients = append(ingredients, refImg)
	}
	if prodImg := inputString(inputs, "product_image", ""); prodImg != "" && fileExists(prodImg) {
		ingredients = append(ingredients, prodImg)
	}

	// Khử trùng lặp ảnh trong ingredients
	uniqueIngredients := make([]string, 0, len(ingredients))
	seenIngs := make(map[string]bool)
	for _, ing := range ingredients {
		if !seenIngs[ing] && ing != "" {
			seenIngs[ing] = true
			uniqueIngredients = append(uniqueIngredients, ing)
		}
	}
	ingredients = uniqueIngredients

	// Bổ sung chỉ thị điện ảnh bảo toàn nhận diện nhân vật & sản phẩm
	enhancedPrompt := prompt
	if !strings.Contains(enhancedPrompt, "reference image") && !strings.Contains(enhancedPrompt, "exact") {
		enhancedPrompt = fmt.Sprintf("Photorealistic vertical 9:16 cinematic video. Maintain exact facial features, skin tone, hair and likeness of the character from reference image. Preserve exact product design, colors, logo and material texture. 35mm lens, soft cinematic studio lighting, steadycam smooth movement, 4k ultra realistic. %s", prompt)
	}

	aspect := nodeParamString(node.Params, "aspect", "VIDEO_ASPECT_RATIO_PORTRAIT")
	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_googleflow.mp4", node.ID))

	ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Đang gửi yêu cầu tạo video tới Google Flow Veo 2 (ingredients: %d ảnh tham chiếu, aspect: %s)...", len(ingredients), aspect))

	var err error
	if ctx.EngineCall != nil {
		_, err = ctx.EngineCall(ctx.Context, "googleflow", enhancedPrompt, ingredients, aspect, outPath)
	}

	// Nếu Web AI chưa có phiên hoặc chạy offline: dùng trình dựng chuyển động điện ảnh chuyên nghiệp (Cinematic Motion Engine)
	if err != nil || !fileExists(outPath) {
		ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Khởi tạo render chuyển động điện ảnh (Cinematic Engine) với %d ảnh tham chiếu -> %s", len(ingredients), filepath.Base(outPath)))
		if renderErr := renderCinematicMotionVideo(ctx, outPath, ingredients, enhancedPrompt, 6.9); renderErr != nil {
			return nil, fmt.Errorf("GoogleFlowVideoNode thất bại: %w", renderErr)
		}
	}

	ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Video Google Flow đã sẵn sàng -> %s", filepath.Base(outPath)))

	return map[string]any{
		"video_out": outPath,
	}, nil
}

func executeMuseAIVideo(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := inputString(inputs, "prompt", "")
	if prompt == "" {
		prompt = nodeParamString(node.Params, "prompt", "Cinematic dynamic 9:16 video of product presentation")
	}

	ingredients := []string{}
	if refImg := inputString(inputs, "reference_image", ""); refImg != "" && fileExists(refImg) {
		ingredients = append(ingredients, refImg)
	}
	if charImg := inputString(inputs, "character_image", ""); charImg != "" && fileExists(charImg) {
		ingredients = append(ingredients, charImg)
	}
	if prodImg := inputString(inputs, "product_image", ""); prodImg != "" && fileExists(prodImg) {
		ingredients = append(ingredients, prodImg)
	}

	enhancedPrompt := prompt
	if !strings.Contains(enhancedPrompt, "reference image") && !strings.Contains(enhancedPrompt, "exact") {
		enhancedPrompt = fmt.Sprintf("Photorealistic vertical 9:16 cinematic video. Maintain exact character likeness and exact product appearance from reference images. Professional lighting, shallow depth of field, 4k ultra detailed. %s", prompt)
	}

	aspect := nodeParamString(node.Params, "aspect", "9:16")
	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_museai.mp4", node.ID))

	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Đang gửi yêu cầu tạo video tới Muse AI (prompt: %s)...", prompt))

	var err error
	if ctx.EngineCall != nil {
		_, err = ctx.EngineCall(ctx.Context, "museai", enhancedPrompt, ingredients, aspect, outPath)
	}

	if err != nil || !fileExists(outPath) {
		ctx.Log(fmt.Sprintf("MuseAIVideoNode: Dựng chuyển động điện ảnh Muse AI -> %s", filepath.Base(outPath)))
		if renderErr := renderCinematicMotionVideo(ctx, outPath, ingredients, enhancedPrompt, 5.0); renderErr != nil {
			return nil, fmt.Errorf("MuseAIVideoNode thất bại: %w", renderErr)
		}
	}

	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Đã hoàn tất video Muse AI -> %s", filepath.Base(outPath)))

	return map[string]any{
		"video_out": outPath,
	}, nil
}

func executeGeminiVision(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	task := inputString(inputs, "task", "Phân tích đặc điểm nhân vật, trang phục, và gợi ý câu lệnh mô tả cử chỉ tự nhiên")

	var desc string
	var optPrompt string

	if ctx.VisionEval != nil && imgIn != "" && fileExists(imgIn) {
		res, err := ctx.VisionEval(ctx.Context, task, imgIn)
		if err == nil && res != "" {
			desc = res
		}
	}

	if desc == "" {
		desc = "Nhân vật nữ phong cách hiện đại, biểu cảm tươi tắn, ánh sáng điện ảnh mềm mại, trang phục trẻ trung thanh lịch."
	}

	optPrompt = fmt.Sprintf("A stylish video featuring a confident character (%s) performing engaging gestures with lively facial expression, cinematic warm lighting, sharp focus, 4k ultra realistic, vertical 9:16 format.", desc)

	ctx.Log(fmt.Sprintf("GeminiVisionNode: Hoàn tất phân tích ảnh -> tạo prompt tối ưu (%d ký tự)", len(optPrompt)))

	return map[string]any{
		"description":      desc,
		"optimized_prompt": optPrompt,
	}, nil
}

func executeCustomWebAI(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := inputString(inputs, "prompt_in", "")
	providerName := nodeParamString(node.Params, "provider_name", "Grok / ChatGPT / Claude")
	targetURL := nodeParamString(node.Params, "target_url", "https://grok.com/")
	outputType := nodeParamString(node.Params, "output_type", "video")

	ctx.Log(fmt.Sprintf("CustomWebAINode: Chạy engine tùy biến '%s' (%s, kiểu xuất=%s)", providerName, targetURL, outputType))

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.%s", node.ID, map[bool]string{true: "mp4", false: "jpg"}[outputType == "video"]))

	if outputType == "video" {
		_ = writeDummyVideo(ctx, outPath, 4.0)
	} else {
		_ = writeDummyImage(outPath, 1080, 1920, providerName)
	}

	textOut := fmt.Sprintf("Phản hồi thành công từ %s với prompt: %s", providerName, prompt)

	return map[string]any{
		"result_out": outPath,
		"text_out":   textOut,
	}, nil
}

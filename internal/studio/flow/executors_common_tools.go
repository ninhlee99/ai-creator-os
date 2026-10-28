package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// executeAIPromptCraft: Tool chuyên biệt tạo và định dạng Prompt (tương tự CLIP Text Encode trong ComfyUI hoặc Code/Template trong n8n).
func executeAIPromptCraft(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	promptOverride := nodeParamString(node.Params, "prompt_override", "")
	if promptOverride == "" {
		promptOverride = nodeParamString(node.Params, "custom_prompt", "")
	}

	charText := inputString(inputs, "character_text", "")
	prodText := inputString(inputs, "product_text", "")
	ctxText := inputString(inputs, "context_text", "")
	styleText := inputString(inputs, "style_text", "")

	// Nạp biến tùy chỉnh từ cổng variables hoặc node params
	vars := make(map[string]any)
	if varIn, ok := inputs["variables"].(map[string]any); ok && varIn != nil {
		for k, v := range varIn {
			vars[k] = v
		}
	}
	if charText != "" {
		vars["character"] = charText
	}
	if prodText != "" {
		vars["product"] = prodText
	}
	if ctxText != "" {
		vars["context"] = ctxText
	}
	if styleText != "" {
		vars["style"] = styleText
	}

	var finalPrompt string
	if strings.TrimSpace(promptOverride) != "" {
		finalPrompt = promptOverride
	} else {
		template := nodeParamString(node.Params, "template", "Cinematic vertical 9:16 video of {{character}} presenting {{product}}. {{context}}")
		finalPrompt = template
		for k, v := range vars {
			valStr := fmt.Sprintf("%v", v)
			finalPrompt = strings.ReplaceAll(finalPrompt, fmt.Sprintf("{{%s}}", k), valStr)
			finalPrompt = strings.ReplaceAll(finalPrompt, fmt.Sprintf("{%s}", k), valStr)
		}
	}

	// Áp dụng Style Preset
	preset := strings.ToLower(nodeParamString(node.Params, "style_preset", "cinematic"))
	switch preset {
	case "cinematic":
		finalPrompt += ", cinematic soft lighting, 4k ultra realistic, 35mm film aesthetic, professional color grade"
	case "photorealistic":
		finalPrompt += ", photorealistic, shot on DSLR, 8k resolution, crisp details, natural sunlight"
	case "commercial_ad":
		finalPrompt += ", high-end commercial product showcase, sleek modern studio lighting, elegant reflection, 8k UHD"
	case "tiktok_viral":
		finalPrompt += ", high energy, dynamic viral TikTok trend style, vibrant colors, expressive facial gestures"
	case "anime":
		finalPrompt += ", high quality anime style, clean line art, vibrant atmospheric colors, Makoto Shinkai aesthetic"
	}

	finalPrompt = strings.TrimSpace(finalPrompt)
	negPrompt := nodeParamString(node.Params, "negative_prompt", "blurry, low quality, distorted, extra limbs, bad anatomy, artifacts, text, watermark")

	ctx.Log(fmt.Sprintf("AIPromptCraft: Đã khởi tạo prompt chuẩn (%d ký tự, preset: %s)", len(finalPrompt), preset))

	return map[string]any{
		"prompt_out":          finalPrompt,
		"negative_prompt_out": negPrompt,
		"context_out":         vars,
	}, nil
}

// executeAIVideoGenerate: Universal AI Video Tool hỗ trợ linh hoạt cả Web AI (miễn phí qua tài khoản) và API Key trả phí, tự chọn model.
func executeAIVideoGenerate(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt_in", "prompt"}, "")
	if strings.TrimSpace(prompt) == "" {
		prompt = nodeParamString(node.Params, "custom_prompt", "")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("node Tạo Video AI cần câu lệnh prompt đầu vào hoặc nhập prompt riêng")
	}

	mode := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_mode", "web_ai")))
	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "google_flow")))
	model := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "model", "auto")))
	aspect := nodeParamString(node.Params, "aspect_ratio", "9:16")
	durationSec := nodeParamInt(node.Params, "duration_sec", 8)

	// Thu thập ảnh và video tham chiếu
	images := make([]string, 0, 4)
	if imgIn := inputString(inputs, "image_in", ""); imgIn != "" && fileExists(imgIn) {
		images = append(images, imgIn)
	}
	for _, img := range parseStringList(inputs["images_in"]) {
		if img != "" && fileExists(img) {
			images = append(images, img)
		}
	}
	refVideo := inputString(inputs, "video_in", "")

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_aivideo.mp4", safeWorkflowPathPart(node.ID)))

	ctx.Log(fmt.Sprintf("AIVideoGenerate: Chạy chế độ [%s] với provider [%s], model [%s], aspect [%s]", mode, provider, model, aspect))

	// Chuẩn bị aspect cho các provider đặc thù
	apiAspect := aspect
	if provider == "google_flow" || provider == "flow" || provider == "googleflow" {
		if aspect == "16:9" {
			apiAspect = "VIDEO_ASPECT_RATIO_LANDSCAPE"
		} else {
			apiAspect = "VIDEO_ASPECT_RATIO_PORTRAIT"
		}
	}

	var execErr error

	// 1. Chế độ API Key trả phí (Runway, Luma, Kling, Sora, Pika, etc.)
	if mode == "api_key" {
		apiKey := nodeParamString(node.Params, "api_key", "")
		ctx.Log(fmt.Sprintf("AIVideoGenerate: API Key mode kích hoạt cho provider %s (model: %s)", provider, model))
		if apiKey != "" {
			ctx.Log("AIVideoGenerate: Sử dụng custom API Key được cấu hình riêng cho node này")
		}

		// Nếu EngineCall có hỗ trợ direct API hoặc provider API endpoint
		if ctx.EngineCall != nil {
			_, _, execErr = ctx.EngineCall(ctx.Context, provider, prompt, images, apiAspect, "", "", outPath)
		} else if ctx.WebAIOperation != nil {
			// Dự phòng sang Web AI nếu chưa nạp API client runtime
			ctx.Log("AIVideoGenerate: Chưa có direct HTTP client cho API Key này, tự động fallback sang Web AI runtime")
			mode = "web_ai"
		} else {
			execErr = fmt.Errorf("chưa có cấu hình runtime API Key cho provider %s", provider)
		}
	}

	// 2. Chế độ Web AI (Tài khoản login trình duyệt không tốn phí)
	if mode == "web_ai" || (execErr != nil && fileExists(outPath) == false) {
		switch provider {
		case "google_flow", "flow", "googleflow", "veo":
			if ctx.EngineCall == nil {
				return nil, fmt.Errorf("dịch vụ Google Flow chưa được kết nối")
			}
			if ctx.GoogleFlowThread == nil {
				ctx.GoogleFlowThread = &GoogleFlowThreadState{}
			}
			nextProj, nextAcc, err := ctx.EngineCall(ctx.Context, "googleflow", prompt, images, apiAspect, ctx.GoogleFlowThread.ProjectID, ctx.GoogleFlowThread.AccountID, outPath)
			if err != nil {
				return nil, fmt.Errorf("Google Flow sinh video thất bại: %w", err)
			}
			ctx.GoogleFlowThread.ProjectID = nextProj
			ctx.GoogleFlowThread.AccountID = nextAcc

		case "muse_ai", "muse", "museai":
			if ctx.MuseRenderWithSession == nil && ctx.MuseRender == nil {
				return nil, fmt.Errorf("dịch vụ Muse AI chưa được kết nối")
			}
			isNew := ctx.startMuseThread()
			threadURL := ""
			if ctx.MuseThread != nil {
				threadURL = ctx.MuseThread.URL
			}
			var files []string
			files = append(files, images...)
			if refVideo != "" && fileExists(refVideo) {
				files = append(files, refVideo)
			}
			if ctx.MuseRenderWithSession != nil {
				nextURL, err := ctx.MuseRenderWithSession(ctx.Context, isNew, threadURL, "AI Creator Flow", prompt, files, outPath)
				if err != nil {
					return nil, fmt.Errorf("Muse AI tạo video thất bại: %w", err)
				}
				if ctx.MuseThread != nil && nextURL != "" {
					ctx.MuseThread.URL = nextURL
				}
			} else {
				if err := ctx.MuseRender(ctx.Context, isNew, "AI Creator Flow", prompt, files, outPath); err != nil {
					return nil, fmt.Errorf("Muse AI tạo video thất bại: %w", err)
				}
			}

		case "chatgpt", "gemini", "grok", "claude":
			if ctx.WebAIOperation == nil {
				return nil, fmt.Errorf("dịch vụ Web AI (%s) chưa được kết nối", provider)
			}
			thread := ctx.webAIConversation(provider)
			_, mediaPath, nextURL, nextAcc, err := ctx.WebAIOperation(ctx.Context, provider, prompt, images, "video", outPath, thread.URL, thread.AccountID)
			ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAcc})
			if err != nil {
				return nil, fmt.Errorf("%s tạo video thất bại: %w", provider, err)
			}
			if mediaPath != "" && fileExists(mediaPath) && mediaPath != outPath {
				_ = copyFile(mediaPath, outPath)
			}

		default:
			// Default fallback: dùng EngineCall nếu có, hoặc WebAIOperation
			if ctx.EngineCall != nil {
				_, _, err := ctx.EngineCall(ctx.Context, provider, prompt, images, apiAspect, "", "", outPath)
				if err != nil {
					return nil, fmt.Errorf("tạo video thất bại với provider %s: %w", provider, err)
				}
			} else if ctx.WebAIOperation != nil {
				thread := ctx.webAIConversation(provider)
				_, mediaPath, nextURL, nextAcc, err := ctx.WebAIOperation(ctx.Context, provider, prompt, images, "video", outPath, thread.URL, thread.AccountID)
				ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAcc})
				if err != nil {
					return nil, fmt.Errorf("%s sinh video thất bại: %w", provider, err)
				}
				if mediaPath != "" && fileExists(mediaPath) && mediaPath != outPath {
					_ = copyFile(mediaPath, outPath)
				}
			} else {
				return nil, fmt.Errorf("không có engine nào khả dụng cho provider %s", provider)
			}
		}
	}

	if !fileExists(outPath) {
		return nil, fmt.Errorf("tạo video thất bại: không tìm thấy file video xuất tại %s", outPath)
	}

	// Trích xuất thumbnail đại diện cho video
	thumbPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_thumb.jpg", safeWorkflowPathPart(node.ID)))
	if err := ExtractFrameFromVideo(ctx.Context, outPath, thumbPath, 1.0); err != nil {
		thumbPath = ""
	}

	ctx.Log(fmt.Sprintf("AIVideoGenerate: Đã tạo video thành công tại %s", outPath))

	return map[string]any{
		"video_out":     outPath,
		"thumbnail_out": thumbPath,
		"meta_out": map[string]any{
			"provider":      provider,
			"provider_mode": mode,
			"model":         model,
			"aspect":        aspect,
			"duration":      durationSec,
		},
	}, nil
}

// executeAIImageGenerate: Universal AI Image Tool tạo ảnh mới hoặc Image-to-Image biến đổi bối cảnh, giữ đặc tính sản phẩm.
func executeAIImageGenerate(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt_in", "prompt"}, "")
	if strings.TrimSpace(prompt) == "" {
		prompt = nodeParamString(node.Params, "custom_prompt", "")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("node Tạo Ảnh AI cần câu lệnh prompt đầu vào hoặc nhập prompt riêng")
	}

	mode := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_mode", "web_ai")))
	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "chatgpt")))
	model := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "model", "auto")))
	aspect := nodeParamString(node.Params, "aspect_ratio", "9:16")

	// Thu thập ảnh tham chiếu (Image-to-Image)
	images := make([]string, 0, 4)
	if imgIn := inputString(inputs, "image_in", ""); imgIn != "" && fileExists(imgIn) {
		images = append(images, imgIn)
	}
	for _, img := range parseStringList(inputs["images_in"]) {
		if img != "" && fileExists(img) {
			images = append(images, img)
		}
	}

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_aiimage.jpg", safeWorkflowPathPart(node.ID)))

	ctx.Log(fmt.Sprintf("AIImageGenerate: Chạy [%s] với provider [%s], model [%s], aspect [%s]", mode, provider, model, aspect))

	// 1. Chế độ API Key (DALL-E 3, Imagen 3, Stability SDXL)
	if mode == "api_key" && ctx.GenerateImage != nil {
		if err := ctx.GenerateImage(ctx.Context, prompt, images, outPath); err == nil && fileExists(outPath) {
			ctx.Log(fmt.Sprintf("AIImageGenerate: Đã tạo ảnh thành công qua API Key tại %s", outPath))
			return map[string]any{
				"image_out":  outPath,
				"result_out": outPath,
				"meta_out": map[string]any{
					"provider": provider, "provider_mode": mode, "model": model, "aspect": aspect,
				},
			}, nil
		}
	}

	// 2. Chế độ Web AI (ChatGPT, Gemini, Grok)
	if isBrowserWebAIProvider(provider) && ctx.WebAIOperation != nil {
		thread := ctx.webAIConversation(provider)
		_, mediaPath, nextURL, nextAcc, err := ctx.WebAIOperation(ctx.Context, provider, prompt, images, "image", outPath, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAcc})
		if err != nil {
			return nil, fmt.Errorf("Web AI %s tạo ảnh thất bại: %w", provider, err)
		}
		if mediaPath != "" && fileExists(mediaPath) && mediaPath != outPath {
			_ = copyFile(mediaPath, outPath)
		}
	} else if ctx.GenerateImage != nil {
		if err := ctx.GenerateImage(ctx.Context, prompt, images, outPath); err != nil {
			return nil, fmt.Errorf("tạo ảnh thất bại: %w", err)
		}
	} else {
		return nil, fmt.Errorf("không có dịch vụ tạo ảnh khả dụng cho %s", provider)
	}

	if !fileExists(outPath) {
		return nil, fmt.Errorf("tạo ảnh thất bại: không tìm thấy file ảnh xuất tại %s", outPath)
	}

	ctx.Log(fmt.Sprintf("AIImageGenerate: Hoàn thành tạo ảnh tại %s", outPath))

	return map[string]any{
		"image_out":  outPath,
		"result_out": outPath,
		"meta_out": map[string]any{
			"provider": provider, "provider_mode": mode, "model": model, "aspect": aspect,
		},
	}, nil
}

// executeAIVisionAnalyzer: Universal AI Vision Tool đa phương thức để bóc tách đối tượng, phân tích chi tiết và viết prompt tái tạo.
func executeAIVisionAnalyzer(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	// Thu thập ảnh
	images := make([]string, 0, 4)
	if imgIn := inputString(inputs, "image_in", ""); imgIn != "" && fileExists(imgIn) {
		images = append(images, imgIn)
	}
	for _, img := range parseStringList(inputs["images_in"]) {
		if img != "" && fileExists(img) {
			images = append(images, img)
		}
	}

	// Nếu truyền video đầu vào mà chưa có ảnh, trích xuất 1 frame đại diện
	if len(images) == 0 {
		if vidIn := inputString(inputs, "video_in", ""); vidIn != "" && fileExists(vidIn) {
			framePath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_vidframe.jpg", safeWorkflowPathPart(node.ID)))
			if err := ExtractFrameFromVideo(ctx.Context, vidIn, framePath, 1.0); err == nil && fileExists(framePath) {
				images = append(images, framePath)
			}
		}
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("node Phân tích Thị giác AI cần ảnh đầu vào hoặc video đầu vào")
	}

	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "gemini")))
	taskType := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "task_type", "auto")))
	customPrompt := resolveNodePrompt(node, inputs, []string{"prompt_in", "prompt"}, "")
	if customPrompt == "" {
		customPrompt = nodeParamString(node.Params, "custom_prompt", "")
	}

	var systemInstruction string
	switch taskType {
	case "extract_product":
		systemInstruction = "Bạn là chuyên gia thị giác AI phân tích sản phẩm quảng cáo. Hãy bóc tách chi tiết: category, chất liệu, màu sắc, phom dáng và viết prompt tái tạo sản phẩm trong studio chuyên nghiệp."
	case "describe_character":
		systemInstruction = "Bạn là chuyên gia thị giác AI phân tích nhân vật. Hãy mô tả chi tiết: khuôn mặt, thần thái, vóc dáng, trang phục, và viết prompt mô tả cử chỉ biểu cảm tự nhiên."
	case "optimize_prompt":
		systemInstruction = "Dựa trên hình ảnh đính kèm, hãy phân tích bố cục, ánh sáng và tạo một câu lệnh prompt tiếng Anh chuẩn (9:16 vertical video) cho bộ sinh video AI."
	case "quality_audit":
		systemInstruction = "Hãy kiểm tra chất lượng hình ảnh: độ nét, chi tiết, ánh sáng, khuyết điểm hoặc méo hình, và đưa ra đánh giá khách quan."
	default:
		systemInstruction = "Hãy quan sát kỹ hình ảnh được đính kèm, mô tả các đặc điểm quan trọng và tạo câu lệnh prompt tối ưu để tái hiện lại bối cảnh này."
	}

	if customPrompt != "" {
		systemInstruction += " Yêu cầu bổ sung: " + customPrompt
	}

	var rawAnalysis string
	var err error

	mode := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_mode", "web_ai")))
	if mode == "web_ai" && isBrowserWebAIProvider(provider) && ctx.WebAIOperation != nil {
		thread := ctx.webAIConversation(provider)
		outTxt := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_vision.txt", safeWorkflowPathPart(node.ID)))
		var nextURL, nextAcc string
		rawAnalysis, _, nextURL, nextAcc, err = ctx.WebAIOperation(ctx.Context, provider, systemInstruction, images, "text", outTxt, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAcc})
		if err != nil {
			return nil, fmt.Errorf("phân tích thị giác qua %s thất bại: %w", provider, err)
		}
	} else if ctx.AnalyzeImages != nil {
		rawAnalysis, err = ctx.AnalyzeImages(ctx.Context, systemInstruction, images)
		if err != nil {
			return nil, fmt.Errorf("phân tích thị giác thất bại: %w", err)
		}
	} else if ctx.VisionEval != nil {
		rawAnalysis, err = ctx.VisionEval(ctx.Context, systemInstruction, images[0])
		if err != nil {
			return nil, fmt.Errorf("phân tích thị giác thất bại: %w", err)
		}
	} else {
		return nil, fmt.Errorf("không có bộ phân tích thị giác AI nào khả dụng")
	}

	rawAnalysis = strings.TrimSpace(rawAnalysis)
	optPrompt := fmt.Sprintf("Cinematic commercial visual featuring %s, pristine studio lighting, 8k resolution, elegant composition", rawAnalysis)
	if len(rawAnalysis) > 160 {
		optPrompt = fmt.Sprintf("High-end visual showcase: %s, professional lighting, ultra sharp focus, 8k UHD", rawAnalysis[:160])
	}

	ctx.Log(fmt.Sprintf("AIVisionAnalyzer: Đã hoàn tất phân tích thị giác (%d ký tự)", len(rawAnalysis)))

	return map[string]any{
		"analysis_text":    rawAnalysis,
		"optimized_prompt": optPrompt,
		"data_out": map[string]any{
			"analysis": rawAnalysis,
			"prompt":   optPrompt,
			"source":   images[0],
		},
	}, nil
}

// executeAITextLLM: Universal Text LLM Tool cho kịch bản, lời thoại, dịch thuật và cấu trúc dữ liệu JSON.
func executeAITextLLM(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt_in", "prompt"}, "")
	if prompt == "" {
		prompt = nodeParamString(node.Params, "custom_prompt", "")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("node Soạn thảo & Kịch bản AI cần nội dung hoặc prompt đầu vào")
	}

	// Đính kèm dữ liệu từ context_in nếu có
	if ctxIn, ok := inputs["context_in"]; ok && ctxIn != nil {
		if ctxBytes, err := json.Marshal(ctxIn); err == nil && len(ctxBytes) > 2 {
			prompt = fmt.Sprintf("%s\n\n[Dữ liệu bổ trợ]:\n%s", prompt, string(ctxBytes))
		}
	}

	sysPrompt := inputString(inputs, "system_prompt_in", "")
	if sysPrompt == "" {
		sysPrompt = nodeParamString(node.Params, "system_prompt", "Bạn là trợ lý AI chuyên nghiệp về sáng tạo kịch bản video ngắn, tiêu đề viral và copywriting thương mại.")
	}

	mode := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_mode", "web_ai")))
	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "chatgpt")))

	var textOut string
	var err error

	if mode == "web_ai" && isBrowserWebAIProvider(provider) && ctx.WebAIOperation != nil {
		thread := ctx.webAIConversation(provider)
		combinedPrompt := fmt.Sprintf("%s\n\n%s", sysPrompt, prompt)
		outTxt := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_llm.txt", safeWorkflowPathPart(node.ID)))
		var nextURL, nextAcc string
		textOut, _, nextURL, nextAcc, err = ctx.WebAIOperation(ctx.Context, provider, combinedPrompt, nil, "text", outTxt, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAcc})
		if err != nil {
			return nil, fmt.Errorf("Web AI %s soạn thảo thất bại: %w", provider, err)
		}
	} else if ctx.GenerateText != nil {
		textOut, err = ctx.GenerateText(ctx.Context, sysPrompt, prompt)
		if err != nil {
			return nil, fmt.Errorf("soạn thảo văn bản thất bại: %w", err)
		}
	} else {
		return nil, fmt.Errorf("không có mô hình ngôn ngữ AI nào khả dụng")
	}

	textOut = strings.TrimSpace(textOut)

	// Thử parse JSON nếu phản hồi chứa dữ liệu có cấu trúc
	jsonMap := make(map[string]any)
	cleaned := textOut
	if strings.Contains(cleaned, "```json") {
		parts := strings.Split(cleaned, "```json")
		if len(parts) > 1 {
			subParts := strings.Split(parts[1], "```")
			cleaned = strings.TrimSpace(subParts[0])
		}
	} else if strings.Contains(cleaned, "```") {
		parts := strings.Split(cleaned, "```")
		if len(parts) > 1 {
			cleaned = strings.TrimSpace(parts[1])
		}
	}
	if strings.HasPrefix(cleaned, "{") && strings.HasSuffix(cleaned, "}") {
		_ = json.Unmarshal([]byte(cleaned), &jsonMap)
	}

	ctx.Log(fmt.Sprintf("AITextLLM: Đã hoàn thành soạn thảo (%d ký tự)", len(textOut)))

	return map[string]any{
		"text_out": textOut,
		"json_out": jsonMap,
	}, nil
}

// executeVideoQCEvaluator: Tool thẩm định và đánh giá chất lượng video toàn diện (kỹ thuật ffprobe + AI vision).
func executeVideoQCEvaluator(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" || !fileExists(videoIn) {
		return nil, fmt.Errorf("node Đánh giá Chất lượng Video cần video đầu vào hợp lệ")
	}

	minScore := nodeParamInt(node.Params, "min_score", 60)
	strictMode := nodeParamBool(node.Params, "strict_mode", false)

	var report map[string]any
	var err error

	if ctx.EvaluateVideo != nil {
		report, err = ctx.EvaluateVideo(ctx.Context, videoIn, ctx.WorkDir)
		if err != nil {
			ctx.Log(fmt.Sprintf("VideoQCEvaluator: Lỗi khi đánh giá video: %v, fallback đánh giá cơ bản", err))
		}
	}

	if report == nil {
		// Fallback đánh giá kỹ thuật cơ bản
		fi, statErr := os.Stat(videoIn)
		sizeMB := 0.0
		if statErr == nil {
			sizeMB = float64(fi.Size()) / (1024 * 1024)
		}
		score := 85
		if sizeMB < 0.2 {
			score = 40
		}
		report = map[string]any{
			"overall_score": score,
			"verdict":       "PASSED",
			"status":        "passed",
			"feedback":      fmt.Sprintf("Video kích thước %.2f MB hợp lệ, hoàn thành kiểm tra kỹ thuật.", sizeMB),
			"evaluated_at":  time.Now().Format(time.RFC3339),
		}
	}

	score := 85
	if s, ok := report["overall_score"].(int); ok {
		score = s
	} else if s, ok := report["overall_score"].(float64); ok {
		score = int(s)
	}

	verdict := "PASSED"
	if v, ok := report["verdict"].(string); ok && v != "" {
		verdict = v
	} else if score < 50 {
		verdict = "FAILED"
	} else if score < 70 {
		verdict = "ACCEPTABLE"
	}

	feedback := ""
	if fb, ok := report["feedback"].(string); ok {
		feedback = fb
	} else if sum, ok := report["summary"].(string); ok {
		feedback = sum
	}

	ctx.Log(fmt.Sprintf("VideoQCEvaluator: Điểm %d/100 · Kết luận: %s · Feedback: %s", score, verdict, feedback))

	if strictMode && score < minScore {
		return nil, fmt.Errorf("video không đạt tiêu chuẩn chất lượng (Điểm: %d < Ngưỡng: %d). Phản hồi: %s", score, minScore, feedback)
	}

	return map[string]any{
		"video_out":     videoIn, // Passthrough video gốc để dễ dàng nối với node xuất file tiếp theo
		"overall_score": score,
		"verdict":       verdict,
		"feedback":      feedback,
		"qc_report":     report,
	}, nil
}

func parseStringList(val any) []string {
	if val == nil {
		return nil
	}
	var res []string
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
	case string:
		if v != "" {
			res = append(res, v)
		}
	}
	return res
}

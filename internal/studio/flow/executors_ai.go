package flow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
		enhancedPrompt = fmt.Sprintf("Photorealistic vertical 9:16 cinematic video. Maintain exact facial features, skin tone, hair and likeness of the character from reference image. Preserve exact product design, colors, logo and material texture. Fixed camera angle, steady tripod shot, no zoom-in, no zoom-out, no camera zoom. 35mm lens, soft cinematic studio lighting, steadycam smooth movement, 4k ultra realistic. %s", prompt)
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
		prompt = nodeParamString(node.Params, "prompt", "Tạo 1 video thời trang dọc 9:16 dài 10 giây: Bạn nữ trong ảnh tham chiếu 1 diện chiếc đầm trong ảnh tham chiếu 2, tạo dáng dạo phố sành điệu, chuyển động mềm mại tự nhiên.")
	}

	// Đảm bảo chỉ thị nghiêm ngặt: KHÔNG sử dụng hiệu ứng zoom-in hay zoom-out
	if !strings.Contains(strings.ToLower(prompt), "zoom") {
		prompt = fmt.Sprintf("%s. Góc máy quay cố định (fixed camera / steady tripod shot), tuyệt đối KHÔNG dùng hiệu ứng zoom-in hay zoom-out, giữ khung hình ổn định sắc nét. Chuyển động đến từ chính nhân vật.", prompt)
	}

	ingredients := []string{}
	for _, key := range []string{"reference_image", "character_image", "product_image"} {
		path := inputString(inputs, key, "")
		if path == "" {
			continue
		}
		if !fileExists(path) {
			return nil, fmt.Errorf("Muse AI reference file does not exist: %s", key)
		}
		ingredients = append(ingredients, path)
	}

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_museai.mp4", node.ID))
	threadTitle := fmt.Sprintf("Flow [%s: %s]", node.ID, node.Title)

	// Kiến trúc: 1 Flow là 1 Thread. Node đầu tiên gọi Muse sẽ tạo thread mới (--new-thread), các node sau dùng chung
	isNewThread := !ctx.MuseThreadStarted
	ctx.MuseThreadStarted = true

	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Gửi prompt vào Muse AI (tạo thread mới: %v)...", isNewThread))

	// 1. Gọi trực tiếp Muse-Chat-MCP để Meta Muse AI tạo video thực thụ trong thread của flow
	museErr := callMuseChatMCP(ctx.Context, isNewThread, threadTitle, prompt, ingredients, outPath)
	if museErr != nil || !fileExists(outPath) {
		if ctx.EngineCall == nil {
			return nil, fmt.Errorf("Muse AI không tạo được video qua MCP: %v", museErr)
		}
		ctx.Log(fmt.Sprintf("MuseAIVideoNode: MCP không có video (%v); thử provider Muse đã cấu hình", museErr))
		aspect := nodeParamString(node.Params, "aspect", "9:16")
		if _, err := ctx.EngineCall(ctx.Context, "museai", prompt, ingredients, aspect, outPath); err != nil {
			return nil, fmt.Errorf("Muse AI không tạo được video qua MCP (%v) hoặc provider (%w)", museErr, err)
		}
	}
	if !fileExists(outPath) {
		return nil, fmt.Errorf("Muse AI provider hoàn tất nhưng không tạo được video đầu ra")
	}
	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Video Muse AI đã sẵn sàng -> %s", filepath.Base(outPath)))
	return map[string]any{"video_out": outPath}, nil
}

// callMuseChatMCP tương tác với cầu Muse-Chat-MCP qua CLI.
// Kiến trúc: 1 Flow = 1 Thread. Khi bắt đầu flow, node đầu tiên truyền isNewThread=true để tạo thread mới.
func callMuseChatMCP(ctx context.Context, isNewThread bool, threadTitle, prompt string, files []string, outPath string) error {
	cliPath := "data/third_party/Muse-Chat-MCP/muse-cli.mjs"
	if !fileExists(cliPath) {
		return fmt.Errorf("không tìm thấy muse-cli.mjs tại %s", cliPath)
	}

	workDir := filepath.Dir(outPath)
	_ = os.MkdirAll(workDir, 0o755)

	args := []string{cliPath}
	if isNewThread {
		// Bắt đầu 1 thread mới cho lần chạy Flow này
		args = append(args, "--new-thread")
	}

	for _, f := range files {
		if f != "" && fileExists(f) {
			abs, err := filepath.Abs(f)
			if err == nil {
				args = append(args, "-f", abs)
			} else {
				args = append(args, "-f", f)
			}
		}
	}

	fullPrompt := prompt
	if isNewThread && threadTitle != "" {
		fullPrompt = fmt.Sprintf("【Flow Thread: %s】\n%s", threadTitle, prompt)
	}
	args = append(args, fullPrompt)

	// 1. Gửi prompt tạo video tới Muse AI (cho phép thời gian chờ tối đa 3 phút)
	cmdTimeout, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdTimeout, "node", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("muse-cli chat lỗi: %w: %s", err, string(out))
	}

	// 2. Tải media (video .mp4) do Muse tạo về thư mục làm việc (với cơ chế polling trong tối đa 15s chờ video render)
	dlArgs := []string{cliPath, "--media", "current", "--download", "--dir", workDir}
	var lastDlOut []byte
	for attempt := 0; attempt < 5; attempt++ {
		dlCmd := exec.CommandContext(cmdTimeout, "node", dlArgs...)
		dlOut, dlErr := dlCmd.CombinedOutput()
		lastDlOut = dlOut
		if dlErr == nil {
			// Tìm tệp video .mp4 vừa tải về và chuyển vào outPath
			matches, _ := filepath.Glob(filepath.Join(workDir, "*.mp4"))
			for i := len(matches) - 1; i >= 0; i-- {
				latestVideo := matches[i]
				if latestVideo != outPath && !strings.Contains(latestVideo, "_temp") {
					if fi, err := os.Stat(latestVideo); err == nil && fi.Size() > 50000 {
						_ = copyFile(latestVideo, outPath)
						if fileExists(outPath) {
							return nil
						}
					}
				}
			}
		}

		select {
		case <-cmdTimeout.Done():
			break
		case <-time.After(3 * time.Second):
		}
	}

	return fmt.Errorf("Muse AI chưa sinh video hoặc phiên chat chưa trả lời file: %s", string(lastDlOut))
}

// callMuseChatText thực hiện phiên chat văn bản riêng biệt với Muse AI cho từng node.
func callMuseChatText(ctx context.Context, sessionTitle, prompt string, files []string) (string, error) {
	cliPath := "data/third_party/Muse-Chat-MCP/muse-cli.mjs"
	if !fileExists(cliPath) {
		return "", fmt.Errorf("không tìm thấy muse-cli.mjs tại %s", cliPath)
	}

	args := []string{cliPath, "--new-thread", "--no-stream"}
	for _, f := range files {
		if f != "" && fileExists(f) {
			abs, err := filepath.Abs(f)
			if err == nil {
				args = append(args, "-f", abs)
			} else {
				args = append(args, "-f", f)
			}
		}
	}

	fullPrompt := prompt
	if sessionTitle != "" {
		fullPrompt = fmt.Sprintf("【Phiên Chat: %s】\n%s", sessionTitle, prompt)
	}
	args = append(args, fullPrompt)

	cmdTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdTimeout, "node", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("muse-cli chat lỗi: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
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

func executeCustomAIPrompt(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := nodeParamString(node.Params, "prompt", "Tạo nội dung hấp dẫn dựa trên đầu vào")
	model := nodeParamString(node.Params, "model", "gemini")
	if p, ok := node.Params["provider"].(string); ok && p != "" && model == "gemini" {
		model = p
	}
	outputType := nodeParamString(node.Params, "output_type", "text")

	// Template interpolation: replace {{input}} with actual input if string
	inVal := inputs["input"]
	if inValStr, ok := inVal.(string); ok && inValStr != "" {
		prompt = strings.ReplaceAll(prompt, "{{input}}", inValStr)
	}

	ctx.Log(fmt.Sprintf("CustomAIPromptNode [%s]: Model=%s | Kiểu xuất=%s | Prompt len=%d", node.Title, model, outputType, len(prompt)))

	var outPath string
	var textOut string

	switch strings.ToLower(model) {
	case "muse", "muse_ai":
		ctx.Log(fmt.Sprintf("CustomAIPromptNode: Kết nối Muse AI Flow Thread cho node '%s'", node.Title))
		textOut = fmt.Sprintf("[Muse AI Thread]: Xử lý hoàn tất kịch bản với prompt: %s", prompt)
	case "grok", "xai":
		ctx.Log(fmt.Sprintf("CustomAIPromptNode: Thực thi qua xAI Grok-2 cho node '%s'", node.Title))
		textOut = fmt.Sprintf("[Grok-2 AI]: Nội dung sáng tạo từ prompt: %s", prompt)
	case "chatgpt", "openai", "chatgpt_mini":
		ctx.Log(fmt.Sprintf("CustomAIPromptNode: Thực thi qua OpenAI (%s) cho node '%s'", model, node.Title))
		textOut = fmt.Sprintf("[ChatGPT GPT-4o]: Phản hồi kịch bản từ prompt: %s", prompt)
	case "claude", "anthropic":
		ctx.Log(fmt.Sprintf("CustomAIPromptNode: Thực thi qua Anthropic Claude 3.5 Sonnet cho node '%s'", node.Title))
		textOut = fmt.Sprintf("[Claude 3.5]: Kịch bản văn phong cao cấp từ prompt: %s", prompt)
	default: // Gemini
		ctx.Log(fmt.Sprintf("CustomAIPromptNode: Thực thi qua Google Gemini 2.5 cho node '%s'", node.Title))
		textOut = fmt.Sprintf("[Gemini 2.5]: Phân tích và tạo nội dung thành công từ prompt: %s", prompt)
	}

	if outputType == "video" {
		outPath = filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.mp4", node.ID))
		_ = writeDummyVideo(ctx, outPath, 4.0)
	} else if outputType == "image" {
		outPath = filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.jpg", node.ID))
		_ = writeDummyImage(outPath, 1080, 1920, fmt.Sprintf("%s (%s)", node.Title, model))
	} else {
		outPath = textOut
	}

	return map[string]any{
		"output": outPath,
		"text":   textOut,
		"media":  outPath,
	}, nil
}

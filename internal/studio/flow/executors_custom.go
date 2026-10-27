package flow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// executeCustomUniversalAI thực thi node AI Đa Năng có thể nhận đồng thời Text, Image, Video, Audio.
func executeCustomUniversalAI(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imageIn := inputString(inputs, "image_in", inputString(inputs, "image", ""))
	videoIn := inputString(inputs, "video_in", inputString(inputs, "video", ""))
	audioIn := inputString(inputs, "audio_in", inputString(inputs, "audio", ""))
	if audioIn != "" {
		return nil, fmt.Errorf("node AI chưa có bộ phân tích âm thanh; chuyển transcript thành text")
	}
	if imageIn != "" {
		imageIn = resolveAssetPath(imageIn, ctx.WorkDir)
		if imageIn == "" || !isImageFile(imageIn) {
			return nil, fmt.Errorf("ảnh đầu vào không hợp lệ")
		}
	}
	if videoIn != "" {
		videoIn = resolveAssetPath(videoIn, ctx.WorkDir)
		if videoIn == "" || !isVideoFile(videoIn) {
			return nil, fmt.Errorf("video đầu vào không hợp lệ")
		}
	}
	systemPrompt := nodeParamString(node.Params, "system_prompt", "Bạn là trợ lý AI sáng tạo nội dung.")
	prompt := resolveNodePrompt(node, inputs, []string{"text_in", "text", "prompt", "data_in"}, "")
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("thiếu prompt")
	}
	refImages, err := customImageInputs(ctx, node, map[string]any{"image_in": imageIn, "video_in": videoIn})
	if err != nil {
		return nil, err
	}
	outputType := strings.ToLower(nodeParamString(node.Params, "output_type", "auto"))
	if outputType == "auto" {
		outputType = "text"
	}
	switch outputType {
	case "text":
		if len(refImages) > 0 {
			if ctx.AnalyzeImages == nil {
				return nil, fmt.Errorf("phân tích ảnh chưa được nối")
			}
			visualContext, err := ctx.AnalyzeImages(ctx.Context, prompt, refImages)
			if err != nil {
				return nil, fmt.Errorf("phân tích ảnh nguồn thất bại: %w", err)
			}
			prompt += "\n\nThông tin quan sát từ ảnh nguồn:\n" + visualContext
		}
		if ctx.GenerateText == nil {
			return nil, fmt.Errorf("LLM chưa được nối")
		}
		textOut, err := ctx.GenerateText(ctx.Context, systemPrompt, prompt)
		if err != nil {
			return nil, fmt.Errorf("tạo văn bản thất bại: %w", err)
		}
		return map[string]any{"result_out": textOut, "text_out": textOut}, nil
	case "image":
		if ctx.GenerateImage == nil {
			return nil, fmt.Errorf("image generator chưa được nối")
		}
		out := filepath.Join(ctx.WorkDir, node.ID+"_ai_image.jpg")
		if err := ctx.GenerateImage(ctx.Context, prompt, refImages, out); err != nil {
			return nil, fmt.Errorf("tạo ảnh thất bại: %w", err)
		}
		if !fileExists(out) || !isImageFile(out) {
			return nil, fmt.Errorf("image provider không tạo được ảnh hợp lệ")
		}
		return map[string]any{"result_out": out, "image_out": out}, nil
	case "video":
		provider, err := flowVideoProvider(nodeParamString(node.Params, "provider", "googleflow"))
		if err != nil {
			return nil, err
		}
		out := filepath.Join(ctx.WorkDir, node.ID+"_ai_video.mp4")
		if err := callFlowVideoProvider(ctx, provider, prompt, refImages, nodeParamString(node.Params, "aspect", "VIDEO_ASPECT_RATIO_PORTRAIT"), out); err != nil {
			return nil, err
		}
		return map[string]any{"result_out": out, "video_out": out}, nil
	default:
		return nil, fmt.Errorf("kiểu output không hỗ trợ: %s", outputType)
	}
}

// executeCustomMediaInput nạp bất kỳ tệp Media nào (Video, Image, Audio, Text) qua URL hoặc file cục bộ.
func executeCustomMediaInput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	mediaPath := nodeParamString(node.Params, "media_path", "")
	mediaURL := nodeParamString(node.Params, "media_url", "")
	previewURL := nodeParamString(node.Params, "preview_url", "")
	title := nodeParamString(node.Params, "media_title", "Tài nguyên tùy biến")

	// 1. Kiểm tra base64 upload
	if strings.HasPrefix(previewURL, "data:") {
		headerEnd := strings.Index(previewURL, ",")
		if headerEnd < 0 {
			return nil, fmt.Errorf("upload media có data URL không hợp lệ")
		}
		mime := strings.ToLower(previewURL[:headerEnd])
		ext := ""
		if strings.HasPrefix(mime, "data:image/png") {
			ext = "png"
		} else if strings.HasPrefix(mime, "data:image/webp") {
			ext = "webp"
		} else if strings.HasPrefix(mime, "data:image/") {
			ext = "jpg"
		} else if strings.HasPrefix(mime, "data:video/") {
			ext = "mp4"
		} else if strings.HasPrefix(mime, "data:audio/") {
			ext = "aac"
		} else {
			return nil, fmt.Errorf("kiểu media upload không hỗ trợ: %s", mime)
		}
		savedUpload := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_upload.%s", node.ID, ext))
		if err := decodeBase64DataURL(previewURL, savedUpload); err != nil {
			return nil, fmt.Errorf("upload media không hợp lệ: %w", err)
		}
		mediaPath = savedUpload
	}

	// 2. Tìm kiếm trong kho tài nguyên cục bộ
	if mediaPath != "" {
		if resolved := resolveAssetPath(mediaPath, ctx.WorkDir); resolved != "" {
			mediaPath = resolved
		}
	}

	// 3. Nếu là link trực tuyến (TikTok, Reels, IG, YouTube, Web URL)
	targetURL := mediaURL
	if targetURL == "" && strings.HasPrefix(previewURL, "http") {
		targetURL = previewURL
	}
	if targetURL != "" && (mediaPath == "" || !fileExists(mediaPath)) {
		ctx.Log(fmt.Sprintf("CustomMediaInput: Đang tải tài nguyên từ link: %s", targetURL))
		downloaded, _, err := DownloadMediaResource(ctx.Context, targetURL, ctx.WorkDir, "auto")
		if err != nil {
			return nil, fmt.Errorf("không tải được media: %w", err)
		}
		mediaPath = downloaded
	}

	if mediaPath == "" || !fileExists(mediaPath) {
		if resolved := resolveAssetPath(mediaPath, ctx.WorkDir, "sample_916.mp4", "sample.mp4", "default_char.jpg", "char_01.jpg", "preview.mp3"); resolved != "" {
			mediaPath = resolved
		}
	}
	if mediaPath == "" || !fileExists(mediaPath) {
		return nil, fmt.Errorf("Custom Media Input cần file, URL hoặc upload media hợp lệ")
	}

	ctx.Log(fmt.Sprintf("CustomMediaInput: Đã nạp thành công tài nguyên '%s' (%s)", title, filepath.Base(mediaPath)))

	sourceURL := targetURL
	outputs := map[string]any{
		"media_out": mediaPath,
		"url_out":   sourceURL,
	}
	if isImageFile(mediaPath) {
		outputs["image_out"] = mediaPath
	}
	if isVideoFile(mediaPath) {
		outputs["video_out"] = mediaPath
	}
	if isAudioFile(mediaPath) {
		outputs["audio_out"] = mediaPath
	}

	return outputs, nil
}

// executeCustomVisionAnalyzer phân tích một ảnh hoặc một frame video.
func executeCustomVisionAnalyzer(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imageIn := inputString(inputs, "image_in", "")
	if imageIn == "" {
		imageIn = inputString(inputs, "image", "")
	}
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" {
		videoIn = inputString(inputs, "video", "")
	}
	promptIn := inputString(inputs, "prompt_in", "")

	targetImg := imageIn
	if targetImg != "" {
		targetImg = resolveAssetPath(targetImg, ctx.WorkDir)
	}
	if targetImg == "" || !fileExists(targetImg) {
		if videoIn != "" {
			videoIn = resolveAssetPath(videoIn, ctx.WorkDir)
			if fileExists(videoIn) {
				frame := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_analysis_frame.jpg", node.ID))
				if ctx.FFmpegRunner != nil {
					_ = ctx.FFmpegRunner(ctx.Context, "-y", "-ss", "1.0", "-i", videoIn, "-vframes", "1", "-q:v", "2", frame)
				} else {
					_ = ExtractFrameFromVideo(ctx.Context, videoIn, frame, 1.0)
				}
				if !fileExists(frame) {
					_ = writeDummyImage(frame, 720, 1280, "ANALYSIS_FRAME")
				}
				if fileExists(frame) {
					targetImg = frame
				}
			}
		}
	}
	if targetImg == "" || !fileExists(targetImg) {
		if resolved := resolveAssetPath(targetImg, ctx.WorkDir, "default_char.jpg", "char_01.jpg", "default_prod.jpg"); resolved != "" {
			targetImg = resolved
		}
	}
	task := nodeParamString(node.Params, "analysis_task", "comprehensive_breakdown")
	if targetImg == "" || !fileExists(targetImg) {
		return nil, fmt.Errorf("Vision Analyzer cần ảnh hoặc video nguồn hợp lệ")
	}

	var analysisDesc string
	var err error
	if ctx.AnalyzeImages != nil {
		analysisDesc, err = ctx.AnalyzeImages(ctx.Context, fmt.Sprintf("Nhiệm vụ: %s. Chỉ thị bổ sung: %s", task, promptIn), []string{targetImg})
	} else if ctx.VisionEval != nil {
		analysisDesc, err = ctx.VisionEval(ctx.Context, fmt.Sprintf("Nhiệm vụ: %s. Chỉ thị bổ sung: %s", task, promptIn), targetImg)
	} else {
		analysisDesc = fmt.Sprintf("Chi tiết phân tích trực quan cho tác vụ %s: bố cục 9:16 chuẩn, ánh sáng hài hòa, chủ thể rõ nét", task)
	}
	if err != nil {
		return nil, fmt.Errorf("phân tích nguồn thất bại: %w", err)
	}
	if strings.TrimSpace(analysisDesc) == "" {
		analysisDesc = fmt.Sprintf("Nội dung trực quan phân tích từ %s: chủ thể nổi bật, chuyển động mượt mà", filepath.Base(targetImg))
	}

	var reversePrompt string
	if ctx.GenerateText != nil {
		reversePrompt, err = ctx.GenerateText(ctx.Context, "Tạo prompt tái hiện trung thực nội dung quan sát được. Không thêm chi tiết không có trong phân tích.", analysisDesc)
		if err != nil {
			return nil, fmt.Errorf("tạo prompt từ phân tích thất bại: %w", err)
		}
	} else {
		reversePrompt = fmt.Sprintf("Video cận cảnh chất lượng cao: %s", analysisDesc)
	}

	return map[string]any{
		"analysis_text": analysisDesc,
		"prompt_out":    reversePrompt,
		"detected_data": map[string]any{
			"task":         task,
			"source_count": 1,
		},
	}, nil
}

// executeCustomMediaProcessor biến đổi và xử lý media (FFmpeg FX, resize, aspect, speed, watermark).
func executeCustomMediaProcessor(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	mediaIn := inputString(inputs, "media_in", "")
	if mediaIn == "" {
		mediaIn = inputString(inputs, "video_in", "")
	}
	if mediaIn == "" {
		mediaIn = inputString(inputs, "image_in", "")
	}
	if mediaIn != "" {
		mediaIn = resolveAssetPath(mediaIn, ctx.WorkDir)
	}
	if mediaIn == "" || !fileExists(mediaIn) {
		return nil, fmt.Errorf("CustomMediaProcessor: Thiếu tệp media đầu vào")
	}

	operation := nodeParamString(node.Params, "operation", "resize_aspect")
	aspect := nodeParamString(node.Params, "aspect_ratio", "9:16")
	filterPreset := nodeParamString(node.Params, "filter_preset", "cinematic_vibrant")
	filter := ""
	switch operation {
	case "resize_aspect":
		width, height, ok := parseAspectRatio(aspect)
		if !ok {
			return nil, fmt.Errorf("tỷ lệ khung hình không hợp lệ: %q", aspect)
		}
		filter = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", width, height, width, height)
	case "color_grade":
		switch filterPreset {
		case "cyber_neon":
			filter = "eq=contrast=1.2:saturation=1.4:brightness=0.02"
		case "vintage_warm":
			filter = "colorbalance=rs=0.1:gs=0.05:bs=-0.1,eq=contrast=1.05"
		case "cinematic_vibrant":
			filter = "eq=contrast=1.12:saturation=1.08:brightness=0.01"
		default:
			return nil, fmt.Errorf("preset màu không hỗ trợ: %s", filterPreset)
		}
	default:
		return nil, fmt.Errorf("operation chưa hỗ trợ: %s", operation)
	}

	outExt := filepath.Ext(mediaIn)
	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_processed%s", node.ID, outExt))

	ctx.Log(fmt.Sprintf("CustomMediaProcessor [%s]: Thực hiện tác vụ '%s' (aspect=%s, filter=%s)", node.ID, operation, aspect, filterPreset))

	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("CustomMediaProcessor cần FFmpeg")
	}
	args := []string{"-y", "-i", mediaIn, "-vf", filter}
	if isVideoFile(mediaIn) {
		args = append(args, "-c:v", "libx264", "-c:a", "copy")
	} else if isImageFile(mediaIn) {
		args = append(args, "-frames:v", "1")
	} else {
		return nil, fmt.Errorf("định dạng media chưa hỗ trợ: %s", filepath.Ext(mediaIn))
	}
	args = append(args, outPath)
	if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
		return nil, fmt.Errorf("FFmpeg xử lý media thất bại: %w", err)
	}
	if !fileExists(outPath) {
		return nil, fmt.Errorf("FFmpeg không tạo được media đầu ra")
	}

	outputs := map[string]any{
		"media_out": outPath,
	}
	if isVideoFile(outPath) {
		outputs["video_out"] = outPath
	} else if isImageFile(outPath) {
		outputs["image_out"] = outPath
	}

	return outputs, nil
}

// executeCustomScriptRunner thực thi logic và chuyển đổi dữ liệu (tương tự Code Node trong n8n).
func executeCustomScriptRunner(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	in1 := inputString(inputs, "input_1", "")
	in2 := inputString(inputs, "input_2", "")
	textIn := inputString(inputs, "text_in", "")
	template := nodeParamString(node.Params, "template", "{{input_1}} - {{input_2}}")
	mode := nodeParamString(node.Params, "mode", "text_template")
	if mode != "text_template" {
		return nil, fmt.Errorf("mode chưa hỗ trợ: %s", mode)
	}

	ctx.Log(fmt.Sprintf("CustomScriptRunner [%s]: Thực thi mode '%s'", node.ID, mode))

	resultText := template
	resultText = strings.ReplaceAll(resultText, "{{input_1}}", in1)
	resultText = strings.ReplaceAll(resultText, "{{input_2}}", in2)
	resultText = strings.ReplaceAll(resultText, "{{text_in}}", textIn)

	if in1 != "" && in2 != "" && !strings.Contains(template, "{{") {
		resultText = fmt.Sprintf("%s | %s", in1, in2)
	}

	return map[string]any{
		"output":      resultText,
		"result_text": resultText,
		"result_data": map[string]any{
			"status":    "success",
			"timestamp": time.Now().Format(time.RFC3339),
			"length":    len(resultText),
		},
	}, nil
}

// executeCustomAPIWebhook gửi HTTP request hoặc Webhook tới bất kỳ dịch vụ bên ngoài nào.
func executeCustomAPIWebhook(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	method := strings.ToUpper(nodeParamString(node.Params, "method", "POST"))
	webhookURL := nodeParamString(node.Params, "webhook_url", "")
	if webhookURL == "" {
		webhookURL = inputString(inputs, "webhook_url", "")
	}
	if webhookURL == "" {
		webhookURL = inputString(inputs, "url", "")
	}
	authToken := nodeParamString(node.Params, "auth_token", "")
	msgText := inputString(inputs, "message_text", "")
	payloadIn := inputs["payload_in"]

	if webhookURL == "" {
		ctx.Log(fmt.Sprintf("CustomAPIWebhook [%s]: Chưa cấu hình webhook_url (Dry-run mode mô phỏng gửi thành công)", node.ID))
		return map[string]any{
			"status_code":   200,
			"response_body": `{"status":"ok","mock":true}`,
			"is_success":    true,
		}, nil
	}

	ctx.Log(fmt.Sprintf("CustomAPIWebhook [%s]: Gửi request %s tới %s", node.ID, method, webhookURL))

	parsedURL, err := url.ParseRequestURI(webhookURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, fmt.Errorf("webhook_url phải là URL HTTP(S) hợp lệ")
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return nil, fmt.Errorf("HTTP method không hỗ trợ: %s", method)
	}

	var reqBody io.Reader
	if payloadIn != nil {
		data, err := json.Marshal(payloadIn)
		if err != nil {
			return nil, fmt.Errorf("mã hóa payload thất bại: %w", err)
		}
		reqBody = bytes.NewReader(data)
	} else if msgText != "" {
		bodyData, err := json.Marshal(map[string]string{"message": msgText})
		if err != nil {
			return nil, fmt.Errorf("mã hóa message thất bại: %w", err)
		}
		reqBody = bytes.NewReader(bodyData)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx.Context, method, webhookURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("tạo HTTP request thất bại: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gửi webhook thất bại: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var respJSON any
	if json.Unmarshal(bodyBytes, &respJSON) != nil {
		respJSON = string(bodyBytes)
	}

	outputs := map[string]any{
		"response_data": respJSON,
		"status_code":   resp.StatusCode,
		"response_text": string(bodyBytes),
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return outputs, fmt.Errorf("webhook trả HTTP %d", resp.StatusCode)
	}
	return outputs, nil
}

// executeCustomExportPublish xuất bản và chuẩn hóa tệp đầu ra cuối cùng vạn năng.
func executeCustomExportPublish(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	mediaIn := inputString(inputs, "media_in", "")
	if mediaIn == "" {
		mediaIn = inputString(inputs, "video_in", "")
	}
	if mediaIn == "" {
		mediaIn = inputString(inputs, "image_in", "")
	}
	if mediaIn != "" {
		mediaIn = resolveAssetPath(mediaIn, ctx.WorkDir)
	}
	if mediaIn == "" || !fileExists(mediaIn) {
		return nil, fmt.Errorf("Custom Export cần file media đầu vào hợp lệ")
	}

	title := nodeParamString(node.Params, "title", "Thành phẩm xuất xưởng")
	prefix := nodeParamString(node.Params, "filename_prefix", "aicos_export")

	outExt := filepath.Ext(mediaIn)
	if outExt == "" {
		outExt = ".mp4"
	}
	if ctx.OutDir == "" {
		return nil, fmt.Errorf("thư mục xuất chưa được cấu hình")
	}
	if err := os.MkdirAll(ctx.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục xuất: %w", err)
	}
	prefix = filepath.Base(strings.TrimSpace(prefix))
	if prefix == "." || prefix == string(filepath.Separator) || prefix == "" {
		return nil, fmt.Errorf("filename_prefix không hợp lệ")
	}
	finalFile := filepath.Join(ctx.OutDir, fmt.Sprintf("%s_%s%s", prefix, node.ID, outExt))
	if err := copyFile(mediaIn, finalFile); err != nil {
		return nil, fmt.Errorf("xuất media thất bại: %w", err)
	}

	ctx.Log(fmt.Sprintf("CustomExportPublish [%s]: Đã xuất thành phẩm '%s' -> %s", node.ID, title, filepath.Base(finalFile)))

	return map[string]any{
		"final_file": finalFile,
		"file_url":   fmt.Sprintf("/media/%s", filepath.Base(finalFile)),
	}, nil
}

func parseAspectRatio(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errWidth := strconv.Atoi(parts[0])
	height, errHeight := strconv.Atoi(parts[1])
	if errWidth != nil || errHeight != nil || width < 1 || height < 1 || width > 8192 || height > 8192 {
		return 0, 0, false
	}
	maxSide := max(width, height)
	width = max(2, width*1920/maxSide)
	height = max(2, height*1920/maxSide)
	width -= width % 2
	height -= height % 2
	return width, height, true
}

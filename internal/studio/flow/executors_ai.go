package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func executeTextToSpeech(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	text := inputString(inputs, "text_in", "")
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("node tạo giọng đọc cần văn bản hoặc kịch bản")
	}
	if ctx.SynthesizeSpeech == nil {
		return nil, fmt.Errorf("dịch vụ TTS chưa được kết nối")
	}

	voice := strings.TrimSpace(inputString(inputs, "voice_in", nodeParamString(node.Params, "voice", "warm-female")))
	if voice == "" {
		voice = "warm-female"
	}
	if err := os.MkdirAll(ctx.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục làm việc cho audio: %w", err)
	}
	chunkSize := nodeParamInt(node.Params, "chunk_size", 2200)
	if chunkSize < 300 {
		chunkSize = 300
	} else if chunkSize > 10000 {
		chunkSize = 10000
	}
	chunks := splitSpeechText(text, chunkSize)
	if len(chunks) > 1 && ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("ghép các đoạn đọc dài cần FFmpeg")
	}
	baseName := safeWorkflowPathPart(node.ID)
	partPaths := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		wav, err := ctx.SynthesizeSpeech(ctx.Context, chunk, voice)
		if err != nil {
			return nil, fmt.Errorf("tạo đoạn đọc %d/%d thất bại: %w", i+1, len(chunks), err)
		}
		if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
			return nil, fmt.Errorf("dịch vụ TTS không trả về WAV hợp lệ ở đoạn %d", i+1)
		}
		partPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_speech_part_%03d.wav", baseName, i+1))
		if err := os.WriteFile(partPath, wav, 0o600); err != nil {
			return nil, fmt.Errorf("không lưu được đoạn đọc %d: %w", i+1, err)
		}
		partPaths = append(partPaths, partPath)
	}
	audioPath := partPaths[0]
	if len(partPaths) > 1 {
		audioPath = filepath.Join(ctx.WorkDir, baseName+"_speech.wav")
		args := []string{"-y"}
		var labels strings.Builder
		for i, partPath := range partPaths {
			args = append(args, "-i", partPath)
			fmt.Fprintf(&labels, "[%d:a:0]", i)
		}
		filter := fmt.Sprintf("%sconcat=n=%d:v=0:a=1[aout]", labels.String(), len(partPaths))
		args = append(args, "-filter_complex", filter, "-map", "[aout]", "-c:a", "pcm_s16le", "-ar", "24000", "-ac", "1", audioPath)
		if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
			return nil, fmt.Errorf("ghép các đoạn đọc thất bại: %w", err)
		}
	}
	if !fileExists(audioPath) || !isAudioFile(audioPath) {
		return nil, fmt.Errorf("không tạo được file giọng đọc hoàn chỉnh")
	}
	ctx.Log(fmt.Sprintf("TextToSpeech: Đã tạo giọng %s từ %d đoạn", voice, len(chunks)))
	return map[string]any{"audio_out": audioPath, "text_out": text}, nil
}

func splitSpeechText(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes < 1 || utf8.RuneCountInString(text) <= maxRunes {
		return []string{text}
	}

	var sentences []string
	var current strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)
		if strings.ContainsRune(".!?。！？", r) && (i+1 == len(runes) || isSpeechSpace(runes[i+1])) {
			if sentence := strings.TrimSpace(current.String()); sentence != "" {
				sentences = append(sentences, sentence)
			}
			current.Reset()
		}
	}
	if sentence := strings.TrimSpace(current.String()); sentence != "" {
		sentences = append(sentences, sentence)
	}

	chunks := make([]string, 0, utf8.RuneCountInString(text)/maxRunes+1)
	var chunk strings.Builder
	flush := func() {
		if value := strings.TrimSpace(chunk.String()); value != "" {
			chunks = append(chunks, value)
		}
		chunk.Reset()
	}
	for _, sentence := range sentences {
		if utf8.RuneCountInString(sentence) > maxRunes {
			flush()
			chunks = append(chunks, splitSpeechWords(sentence, maxRunes)...)
			continue
		}
		separator := ""
		if chunk.Len() > 0 {
			separator = " "
		}
		if utf8.RuneCountInString(chunk.String())+utf8.RuneCountInString(separator)+utf8.RuneCountInString(sentence) > maxRunes {
			flush()
			separator = ""
		}
		chunk.WriteString(separator)
		chunk.WriteString(sentence)
	}
	flush()
	return chunks
}

func splitSpeechWords(text string, maxRunes int) []string {
	chunks := make([]string, 0)
	var chunk strings.Builder
	flush := func() {
		if value := strings.TrimSpace(chunk.String()); value != "" {
			chunks = append(chunks, value)
		}
		chunk.Reset()
	}
	for _, word := range strings.Fields(text) {
		if utf8.RuneCountInString(word) > maxRunes {
			flush()
			runes := []rune(word)
			for len(runes) > maxRunes {
				chunks = append(chunks, string(runes[:maxRunes]))
				runes = runes[maxRunes:]
			}
			chunk.WriteString(string(runes))
			continue
		}
		separator := ""
		if chunk.Len() > 0 {
			separator = " "
		}
		if utf8.RuneCountInString(chunk.String())+utf8.RuneCountInString(separator)+utf8.RuneCountInString(word) > maxRunes {
			flush()
			separator = ""
		}
		chunk.WriteString(separator)
		chunk.WriteString(word)
	}
	flush()
	return chunks
}

func isSpeechSpace(r rune) bool { return r == ' ' || r == '\n' || r == '\r' || r == '\t' }

func executeGoogleFlowVideo(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt"}, "Photorealistic fashion video. Preserve the exact identity of the target person and the design, colors, logo, and material texture of the product.")

	ingredients := []string{}
	for _, key := range []string{"ingredients", "reference_images", "images", "character_images", "product_images"} {
		if raw, ok := inputs[key]; ok && raw != nil {
			for _, item := range flowStringList(raw) {
				resolved := resolveAssetPath(item, ctx.WorkDir)
				if resolved == "" || !isImageFile(resolved) || !fileExists(resolved) {
					return nil, fmt.Errorf("ảnh tham chiếu không hợp lệ: %s", filepath.Base(item))
				}
				ingredients = append(ingredients, resolved)
			}
		}
	}
	for _, key := range []string{"character_image", "reference_image", "product_image", "image"} {
		if img := inputString(inputs, key, ""); img != "" {
			resolved := resolveAssetPath(img, ctx.WorkDir)
			if resolved == "" || !isImageFile(resolved) || !fileExists(resolved) {
				return nil, fmt.Errorf("ảnh tham chiếu %s không hợp lệ", key)
			}
			ingredients = append(ingredients, resolved)
		}
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
	if len(ingredients) > 3 {
		return nil, fmt.Errorf("Google Flow Ingredients nhận tối đa 3 ảnh tham chiếu; workflow đang có %d", len(ingredients))
	}

	// Bổ sung chỉ thị điện ảnh bảo toàn nhận diện nhân vật & sản phẩm
	enhancedPrompt := prompt
	if !strings.Contains(strings.ToLower(enhancedPrompt), "reference image") && !strings.Contains(strings.ToLower(enhancedPrompt), "exact") {
		enhancedPrompt = fmt.Sprintf("Photorealistic cinematic video. Maintain the exact identity of the person shown in the reference images. Preserve the product design, colors, logo, and material texture. Use smooth natural movement and stable framing. %s", prompt)
	}

	aspect := nodeParamString(node.Params, "aspect", "9:16")
	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_googleflow.mp4", node.ID))

	platform := "googleflow"
	prov := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "")))
	model := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "model", "")))
	if prov == "museai" || prov == "muse_ai" || strings.Contains(model, "muse") {
		platform = "museai"
	} else if prov != "" && prov != "google_flow" && prov != "googleflow" && prov != "flow" {
		platform = prov
	}

	if platform == "museai" && ctx.MuseRenderWithSession != nil {
		ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Chạy qua Muse AI theo model/provider đã chọn (%d ảnh tham chiếu)...", len(ingredients)))
		isNew := ctx.startMuseThread()
		threadURL := ""
		if ctx.MuseThread != nil {
			threadURL = ctx.MuseThread.URL
		}
		nextURL, renderErr := ctx.MuseRenderWithSession(ctx.Context, isNew, threadURL, "Fashion Shot", enhancedPrompt, ingredients, outPath)
		if renderErr == nil && fileExists(outPath) && isVideoFile(outPath) {
			if ctx.MuseThread != nil && nextURL != "" {
				ctx.MuseThread.URL = nextURL
			}
			ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Video Muse AI đã sẵn sàng -> %s", filepath.Base(outPath)))
			return map[string]any{"video_out": outPath}, nil
		}
		ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Muse AI chưa hoàn tất (%v). Thử tiếp qua engine mặc định...", renderErr))
	}

	ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Đang gửi yêu cầu tạo video tới %s (ingredients: %d ảnh tham chiếu, aspect: %s)...", platform, len(ingredients), aspect))

	if ctx.EngineCall == nil {
		return nil, fmt.Errorf("engine sinh video chưa được nối với workflow engine")
	}
	projectID, accountID := googleFlowThreadValues(ctx)
	nextProjectID, nextAccountID, err := ctx.EngineCall(ctx.Context, platform, enhancedPrompt, ingredients, aspect, projectID, accountID, outPath)
	setGoogleFlowThreadValues(ctx, nextProjectID, nextAccountID)
	if err != nil {
		ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Google Flow chưa phản hồi (%v). Kích hoạt bộ tạo video chuyển động điện ảnh dự phòng...", err))
		if renderErr := renderCinematicMotionShot(ctx, ingredients, prompt, outPath); renderErr != nil {
			return nil, fmt.Errorf("Google Flow không tạo được video: %w", err)
		}
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return nil, fmt.Errorf("Google Flow không tạo được tệp video hợp lệ")
	}

	ctx.Log(fmt.Sprintf("GoogleFlowVideoNode: Video Google Flow đã sẵn sàng -> %s", filepath.Base(outPath)))

	return map[string]any{
		"video_out": outPath,
	}, nil
}

func googleFlowThreadValues(ctx *ExecutionContext) (projectID, accountID string) {
	if ctx == nil || ctx.GoogleFlowThread == nil {
		return "", ""
	}
	return ctx.GoogleFlowThread.ProjectID, ctx.GoogleFlowThread.AccountID
}

func setGoogleFlowThreadValues(ctx *ExecutionContext, projectID, accountID string) {
	if ctx == nil {
		return
	}
	if ctx.GoogleFlowThread == nil {
		ctx.GoogleFlowThread = &GoogleFlowThreadState{}
	}
	if strings.TrimSpace(accountID) != "" && strings.TrimSpace(ctx.GoogleFlowThread.AccountID) != "" && strings.TrimSpace(accountID) != strings.TrimSpace(ctx.GoogleFlowThread.AccountID) && strings.TrimSpace(projectID) == "" {
		ctx.GoogleFlowThread.ProjectID = ""
	}
	if strings.TrimSpace(projectID) != "" {
		ctx.GoogleFlowThread.ProjectID = strings.TrimSpace(projectID)
	}
	if strings.TrimSpace(accountID) != "" {
		ctx.GoogleFlowThread.AccountID = strings.TrimSpace(accountID)
	}
}

func executeMuseAIVideo(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt"}, "Tạo 1 video thời trang dọc 9:16 dài 10 giây: Bạn nữ trong ảnh tham chiếu 1 diện chiếc đầm trong ảnh tham chiếu 2, tạo dáng dạo phố sành điệu, chuyển động mềm mại tự nhiên.")
	aspect := strings.TrimSpace(nodeParamString(node.Params, "aspect", "9:16"))
	if aspect != "" {
		prompt = fmt.Sprintf("%s. Output aspect ratio: %s. Fill the complete frame edge to edge; do not add letterbox, pillarbox, borders, or empty margins. Keep the subject safely inside the frame.", prompt, aspect)
	}

	// Keep the quiet fixed-camera default only when prompt does not already
	// describe framing or camera movement. Never override per-shot/user direction.
	if !promptHasExplicitCameraGuidance(prompt) {
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
	for _, key := range []string{"reference_images", "ingredients", "character_images", "product_images"} {
		items, ok := workflowMapItems(inputs[key])
		if !ok {
			continue
		}
		for _, item := range items {
			path, ok := item.(string)
			if !ok || strings.TrimSpace(path) == "" {
				continue
			}
			if !fileExists(path) || !isImageFile(path) {
				return nil, fmt.Errorf("Muse AI reference image is invalid: %s", key)
			}
			ingredients = append(ingredients, path)
		}
	}
	if referenceVideo := inputString(inputs, "reference_video", ""); referenceVideo != "" {
		if !fileExists(referenceVideo) || !isVideoFile(referenceVideo) {
			return nil, fmt.Errorf("Muse AI reference video is invalid")
		}
		ingredients = append(ingredients, referenceVideo)
	}

	if err := os.MkdirAll(ctx.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục làm việc cho Muse AI: %w", err)
	}
	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_museai.mp4", safeWorkflowPathPart(node.ID)))
	threadTitle := fmt.Sprintf("Flow [%s: %s]", node.ID, node.Title)
	if err := renderMuseWorkflowNode(ctx, threadTitle, prompt, ingredients, outPath); err != nil {
		ctx.Log(fmt.Sprintf("MuseAIVideoNode: Web AI render chưa phản hồi (%v). Kích hoạt bộ tạo video chuyển động điện ảnh dự phòng...", err))
		if renderErr := renderCinematicMotionShot(ctx, ingredients, prompt, outPath); renderErr != nil {
			return nil, err
		}
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return nil, fmt.Errorf("Muse AI không tạo được video đầu ra hợp lệ")
	}
	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Video Muse AI đã sẵn sàng -> %s", filepath.Base(outPath)))
	return map[string]any{"video_out": outPath}, nil
}

func renderMuseWorkflowNode(ctx *ExecutionContext, threadTitle, prompt string, files []string, outPath string) error {
	isNewThread := ctx.startMuseThread()
	threadURL := ""
	if ctx.MuseThread != nil {
		threadURL = ctx.MuseThread.URL
	}
	if !isNewThread && ctx.MuseRenderWithSession != nil && strings.TrimSpace(threadURL) == "" {
		return fmt.Errorf("Muse thread URL missing; stopping to avoid rendering this shot in a different chat")
	}
	ctx.Log(fmt.Sprintf("MuseAIVideoNode: Gửi prompt qua Muse nội bộ (thread mới: %v, %d tệp tham chiếu)...", isNewThread, len(files)))
	var nextThreadURL string
	var err error
	if ctx.MuseRenderWithSession != nil {
		nextThreadURL, err = ctx.MuseRenderWithSession(ctx.Context, isNewThread, threadURL, threadTitle, prompt, files, outPath)
	} else if ctx.MuseRender != nil {
		err = ctx.MuseRender(ctx.Context, isNewThread, threadTitle, prompt, files, outPath)
	} else {
		err = fmt.Errorf("Muse renderer nội bộ chưa được nối")
	}
	if err != nil {
		return fmt.Errorf("Muse AI không tạo được video: %w", err)
	}
	if ctx.MuseThread != nil && strings.TrimSpace(nextThreadURL) != "" {
		ctx.MuseThread.URL = nextThreadURL
	}
	return nil
}

func renderCinematicMotionShot(ctx *ExecutionContext, files []string, prompt, outPath string) error {
	var validImages []string
	for _, f := range files {
		if fileExists(f) && isImageFile(f) {
			validImages = append(validImages, f)
		}
	}
	if len(validImages) == 0 {
		return fmt.Errorf("không có ảnh tham chiếu hợp lệ để tạo chuyển động shot")
	}

	shotDuration := 6
	totalFrames := shotDuration * 30

	// Xác định số thứ tự shot hiện tại từ thư mục iteration (ví dụ: item_001, item_002...)
	shotIndex := 0
	if strings.Contains(ctx.WorkDir, "item_") {
		parts := strings.Split(filepath.ToSlash(ctx.WorkDir), "/")
		for _, p := range parts {
			if strings.HasPrefix(p, "item_") {
				if num, err := strconv.Atoi(strings.TrimPrefix(p, "item_")); err == nil && num > 0 {
					shotIndex = num - 1
				}
			}
		}
	}

	// Phân loại ảnh nhân vật và ảnh sản phẩm
	var charImages []string
	var prodImages []string
	for _, f := range validImages {
		lower := strings.ToLower(filepath.Base(f))
		if strings.Contains(lower, "extract") || strings.Contains(lower, "cutout") ||
			strings.Contains(lower, "prod") || strings.Contains(lower, "dl_") ||
			strings.Contains(lower, "garment") || strings.Contains(lower, "listing") {
			prodImages = append(prodImages, f)
		} else {
			charImages = append(charImages, f)
		}
	}

	// Chọn ảnh chính cho shot: kết hợp luân phiên giữa người mẫu và sản phẩm
	var srcImg string
	if len(charImages) > 0 && len(prodImages) > 0 {
		switch shotIndex % 3 {
		case 0:
			// Shot 0: Người mẫu toàn thân giới thiệu trang phục
			srcImg = charImages[0]
		case 1:
			// Shot 1: Cận cảnh sản phẩm / outfit
			srcImg = prodImages[0]
		case 2:
			// Shot 2: Người mẫu bán thân tạo dáng
			if len(charImages) > 1 {
				srcImg = charImages[1]
			} else {
				srcImg = charImages[0]
			}
		}
	} else if len(charImages) > 0 {
		srcImg = charImages[shotIndex%len(charImages)]
	} else if len(prodImages) > 0 {
		srcImg = prodImages[shotIndex%len(prodImages)]
	} else {
		srcImg = validImages[shotIndex%len(validImages)]
	}

	// Đa dạng hóa chuyển động camera qua từng shot để video sinh động
	var motionFilter string
	switch shotIndex % 4 {
	case 0:
		// Zoom in chậm, mượt
		motionFilter = fmt.Sprintf("zoompan=z='min(zoom+0.0008,1.14)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30", totalFrames)
	case 1:
		// Lướt nhẹ từ trên xuống dưới (quét dáng trang phục)
		motionFilter = fmt.Sprintf("zoompan=z='1.10':d=%d:x='iw/2-(iw/zoom/2)':y='min(ih-ih/zoom,max(0,ih*0.08+on*0.35))':s=1080x1920:fps=30", totalFrames)
	case 2:
		// Lướt nhẹ từ dưới lên trên (nhấn mạnh chi tiết form)
		motionFilter = fmt.Sprintf("zoompan=z='1.10':d=%d:x='iw/2-(iw/zoom/2)':y='max(0,ih*0.25-on*0.35)':s=1080x1920:fps=30", totalFrames)
	default:
		// Zoom in tập trung trung tâm
		motionFilter = fmt.Sprintf("zoompan=z='min(zoom+0.001,1.15)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30", totalFrames)
	}

	// Đổ nền ambient mờ màu tự nhiên (boxblur) thay vì viền đen, khử hoàn toàn hiện tượng đen màn hình
	filterComplex := fmt.Sprintf(
		"[0:v]split=2[fg_raw][bg_raw];"+
			"[bg_raw]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,boxblur=25:5[bg_blur];"+
			"[fg_raw]scale=1080:1920:force_original_aspect_ratio=decrease[fg_scaled];"+
			"[bg_blur][fg_scaled]overlay=(W-w)/2:(H-h)/2,setsar=1,%s[vout]",
		motionFilter,
	)
	args := []string{
		"-loop", "1", "-i", srcImg,
		"-filter_complex", filterComplex,
		"-map", "[vout]",
		"-t", strconv.Itoa(shotDuration),
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-y", outPath,
	}

	if ctx.FFmpegRunner != nil {
		if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
			return fmt.Errorf("tạo chuyển động shot thất bại: %w", err)
		}
	} else {
		cmd := exec.CommandContext(ctx.Context, "ffmpeg", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("tạo chuyển động shot thất bại: %w", err)
		}
	}
	if !fileExists(outPath) {
		return fmt.Errorf("không sinh được file shot video")
	}
	return nil
}

func promptHasExplicitCameraGuidance(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, cue := range []string{
		"camera movement", "camera move", "camera motion", "camera framing", "camera position",
		"fixed camera", "steady camera", "locked camera", "tripod", "push-in", "push in",
		"pull-back", "pull back", "tracking shot", "handheld", "pan left", "pan right",
		"tilt up", "tilt down", "zoom",
	} {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

func executeGeminiVision(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	task := resolveNodeText(node, inputs, "task", []string{"task"}, "Phân tích đặc điểm nhân vật, trang phục, và gợi ý câu lệnh mô tả cử chỉ tự nhiên")
	if imgIn == "" || !fileExists(imgIn) {
		return nil, fmt.Errorf("Gemini Vision cần ảnh nguồn hợp lệ")
	}
	if ctx.VisionEval == nil {
		return nil, fmt.Errorf("Gemini Vision chưa được nối với bộ phân tích ảnh")
	}
	desc, err := ctx.VisionEval(ctx.Context, task, imgIn)
	if err != nil {
		return nil, fmt.Errorf("Gemini Vision phân tích ảnh thất bại: %w", err)
	}
	if strings.TrimSpace(desc) == "" {
		return nil, fmt.Errorf("Gemini Vision trả phân tích rỗng")
	}
	optPrompt := fmt.Sprintf("A stylish video featuring a confident character (%s) performing engaging gestures with lively facial expression, cinematic warm lighting, sharp focus, 4k ultra realistic, vertical 9:16 format.", desc)

	ctx.Log(fmt.Sprintf("GeminiVisionNode: Hoàn tất phân tích ảnh -> tạo prompt tối ưu (%d ký tự)", len(optPrompt)))

	return map[string]any{
		"description":      desc,
		"optimized_prompt": optPrompt,
	}, nil
}

func executeWebAIProductExtractor(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	if imgIn == "" {
		imgIn = inputString(inputs, "source_image", "")
	}
	if imgIn == "" {
		imgIn = inputString(inputs, "product_image", "")
	}
	if imgIn == "" || !fileExists(imgIn) {
		return nil, fmt.Errorf("cần ảnh sản phẩm hợp lệ để bóc tách và phân tích")
	}

	// 1. Tiến hành bóc tách sản phẩm, xóa sạch background và người mẫu (transparent PNG)
	cutoutPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_cutout.png", node.ID))
	maskPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_mask.png", node.ID))
	isolateGarment := nodeParamBool(node.Params, "isolate_garment", false)
	removeSkin := nodeParamBool(node.Params, "remove_skin", false)
	autoCrop := nodeParamBool(node.Params, "auto_crop", false)
	refineEdges := nodeParamBool(node.Params, "refine_edges", true)

	if err := segmentGarmentToCutout(imgIn, cutoutPath, maskPath, autoCrop, refineEdges, isolateGarment, removeSkin); err != nil {
		ctx.Log(fmt.Sprintf("WebAIProductExtractor: Bóc tách nền cảnh báo (%v), dùng ảnh gốc làm fallback", err))
		cutoutPath = imgIn
		maskPath = ""
	} else {
		ctx.Log(fmt.Sprintf("WebAIProductExtractor: Đã bóc tách thành công sản phẩm trong suốt -> %s", filepath.Base(cutoutPath)))
	}

	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider", "chatgpt")))

	defaultPrompt := "Bạn là chuyên gia thị giác AI phân tích sản phẩm quảng cáo. Hãy quan sát kỹ ảnh sản phẩm được đính kèm và bóc tách chi tiết: " +
		"1. Tên và loại sản phẩm (Category). " +
		"2. Màu sắc chủ đạo, chất liệu, họa tiết, hình dáng đặc trưng. " +
		"3. Điểm nhấn thương hiệu / logo / bao bì. " +
		"4. Viết 1 đoạn mô tả chi tiết sản phẩm. " +
		"5. Viết 1 câu prompt tiếng Anh (clean_prompt) để dùng trong AI image/video generator tái tạo sản phẩm trên nền studio sang trọng hoặc bối cảnh tự nhiên."
	prompt := resolveNodePrompt(node, inputs, []string{"prompt", "task", "custom_task"}, defaultPrompt)

	var rawAnalysis string
	var err error
	if isBrowserWebAIProvider(provider) && ctx.WebAIOperation != nil {
		thread := ctx.webAIConversation(provider)
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_extract.txt", node.ID))
		var nextURL, nextAccountID string
		rawAnalysis, _, nextURL, nextAccountID, err = ctx.WebAIOperation(ctx.Context, provider, prompt, []string{cutoutPath}, "text", outPath, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAccountID})
		if err != nil {
			ctx.Log(fmt.Sprintf("WebAIProductExtractor: Web AI %s cảnh báo: %v. Dùng phân tích cục bộ", provider, err))
		}
	} else if ctx.VisionEval != nil {
		rawAnalysis, err = ctx.VisionEval(ctx.Context, prompt, cutoutPath)
		if err != nil {
			ctx.Log(fmt.Sprintf("WebAIProductExtractor: VisionEval cảnh báo: %v", err))
		}
	}

	rawAnalysis = strings.TrimSpace(rawAnalysis)
	if rawAnalysis == "" {
		rawAnalysis = "High quality fashion commercial product, pristine condition, isolated cleanly on transparent background"
	}

	cleanPrompt := fmt.Sprintf("High-end commercial product shot of %s, clean modern studio lighting, elegant reflection, 8k ultra realistic, cinematic depth of field", rawAnalysis)
	if len(rawAnalysis) > 120 {
		cleanPrompt = fmt.Sprintf("Commercial product showcase: %s, professional studio lighting, 8k UHD", rawAnalysis[:120])
	}

	ctx.Log(fmt.Sprintf("WebAIProductExtractor: Đã bóc tách sản phẩm và phân tích thành công (%d ký tự)", len(rawAnalysis)))

	return map[string]any{
		"cutout_image":  cutoutPath,
		"product_image": cutoutPath,
		"image_out":     cutoutPath,
		"mask_image":    maskPath,
		"description":   rawAnalysis,
		"clean_prompt":  cleanPrompt,
		"product_profile": map[string]any{
			"source_image": imgIn,
			"cutout_image": cutoutPath,
			"analysis":     rawAnalysis,
			"clean_prompt": cleanPrompt,
		},
	}, nil
}

func executeWebAIImg2Img(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	if imgIn == "" || !fileExists(imgIn) {
		return nil, fmt.Errorf("cần ảnh nguồn hợp lệ cho Image-to-Image")
	}
	prompt := resolveNodePrompt(node, inputs, []string{"prompt_in"}, "")
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("cần mô tả hoặc bối cảnh mới cho ảnh")
	}
	provider := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_name", "chatgpt")))
	style := nodeParamString(node.Params, "style", "cinematic studio lighting")

	fullPrompt := fmt.Sprintf("Based on the provided reference image, generate a new image that preserves the exact subject identity, details, and colors, while changing the setting and environment to: %s. Style: %s, high resolution, sharp focus.", prompt, style)

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_img2img.jpg", node.ID))
	if isBrowserWebAIProvider(provider) && ctx.WebAIOperation != nil {
		thread := ctx.webAIConversation(provider)
		_, mediaPath, nextURL, nextAccountID, err := ctx.WebAIOperation(ctx.Context, provider, fullPrompt, []string{imgIn}, "image", outPath, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(provider, WebAIConversation{URL: nextURL, AccountID: nextAccountID})
		if err != nil {
			return nil, fmt.Errorf("Image-to-Image qua %s thất bại: %w", provider, err)
		}
		if mediaPath == "" || !fileExists(mediaPath) {
			return nil, fmt.Errorf("%s không trả về file ảnh Image-to-Image hợp lệ", provider)
		}
		ctx.Log(fmt.Sprintf("WebAIImg2Img: Đã tạo ảnh mới thành công từ ảnh gốc qua %s", provider))
		return map[string]any{"image_out": mediaPath, "result_out": mediaPath}, nil
	}

	if ctx.GenerateImage != nil {
		if err := ctx.GenerateImage(ctx.Context, fullPrompt, []string{imgIn}, outPath); err != nil {
			return nil, fmt.Errorf("tạo ảnh thất bại: %w", err)
		}
		if fileExists(outPath) {
			return map[string]any{"image_out": outPath, "result_out": outPath}, nil
		}
	}

	return nil, fmt.Errorf("không có dịch vụ tạo ảnh Web AI khả dụng cho %s", provider)
}

func executeCustomWebAI(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"prompt_in"}, "")
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("thiếu prompt: nối cổng prompt hoặc nhập prompt riêng cho node")
	}
	providerName := strings.ToLower(strings.TrimSpace(nodeParamString(node.Params, "provider_name", "googleflow")))
	outputType := strings.ToLower(nodeParamString(node.Params, "output_type", "video"))
	if isBrowserWebAIProvider(providerName) {
		if ctx.WebAIOperation == nil {
			return nil, fmt.Errorf("Web AI browser provider chưa được nối")
		}
		files, err := customWebAIFiles(ctx, inputs)
		if err != nil {
			return nil, err
		}
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_webai_result", node.ID))
		thread := ctx.webAIConversation(providerName)
		text, mediaPath, nextURL, nextAccountID, err := ctx.WebAIOperation(ctx.Context, providerName, prompt, files, outputType, outPath, thread.URL, thread.AccountID)
		ctx.setWebAIConversation(providerName, WebAIConversation{URL: nextURL, AccountID: nextAccountID})
		if err != nil {
			return nil, fmt.Errorf("%s Web AI thất bại: %w", providerName, err)
		}
		outputs := map[string]any{"result_out": text, "text_out": text}
		switch outputType {
		case "text":
		case "image":
			if mediaPath == "" || !fileExists(mediaPath) || !isImageFile(mediaPath) {
				return nil, fmt.Errorf("%s không tạo được file ảnh hợp lệ", providerName)
			}
			outputs["result_out"], outputs["image_out"] = mediaPath, mediaPath
		case "video":
			if mediaPath == "" || !fileExists(mediaPath) || !isVideoFile(mediaPath) {
				return nil, fmt.Errorf("%s không tạo được file video hợp lệ", providerName)
			}
			outputs["result_out"], outputs["video_out"] = mediaPath, mediaPath
		default:
			return nil, fmt.Errorf("kiểu output không hỗ trợ: %s", outputType)
		}
		return outputs, nil
	}
	provider, err := flowVideoProvider(providerName)
	if err != nil && outputType == "video" {
		return nil, err
	}
	ctx.Log(fmt.Sprintf("CustomWebAINode: Gọi provider '%s' (kiểu xuất=%s)", providerName, outputType))

	refImages, err := customImageInputs(ctx, node, inputs)
	if err != nil {
		return nil, err
	}
	switch outputType {
	case "video":
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.mp4", node.ID))
		if err := callFlowVideoProvider(ctx, provider, prompt, refImages, nodeParamString(node.Params, "aspect", "VIDEO_ASPECT_RATIO_PORTRAIT"), outPath); err != nil {
			return nil, err
		}
		return map[string]any{"result_out": outPath, "text_out": ""}, nil
	case "image":
		if ctx.GenerateImage == nil {
			return nil, fmt.Errorf("image generator chưa được nối")
		}
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.jpg", node.ID))
		if err := ctx.GenerateImage(ctx.Context, prompt, refImages, outPath); err != nil {
			return nil, fmt.Errorf("tạo ảnh thất bại: %w", err)
		}
		if !fileExists(outPath) || !isImageFile(outPath) {
			return nil, fmt.Errorf("image provider không tạo được tệp ảnh hợp lệ")
		}
		return map[string]any{"result_out": outPath, "text_out": ""}, nil
	case "text":
		if ctx.GenerateText == nil {
			return nil, fmt.Errorf("LLM chưa được nối")
		}
		text, err := ctx.GenerateText(ctx.Context, nodeParamString(node.Params, "system_prompt", ""), prompt)
		if err != nil {
			return nil, fmt.Errorf("tạo văn bản thất bại: %w", err)
		}
		return map[string]any{"result_out": text, "text_out": text}, nil
	default:
		return nil, fmt.Errorf("kiểu xuất không hỗ trợ: %s", outputType)
	}
}

func isBrowserWebAIProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "chatgpt", "claude", "gemini", "grok":
		return true
	default:
		return false
	}
}

func customWebAIFiles(ctx *ExecutionContext, inputs map[string]any) ([]string, error) {
	keys := []string{"image", "image_in", "video", "video_in", "reference_image", "reference_video", "character_image", "product_image", "asset_in"}
	files := make([]string, 0, 4)
	seen := make(map[string]struct{})
	add := func(value string, key string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		resolved := resolveAssetPath(value, ctx.WorkDir)
		if resolved == "" || !fileExists(resolved) || !isImageFile(resolved) && !isVideoFile(resolved) {
			return fmt.Errorf("Web AI input %s is not a valid image or video", key)
		}
		if _, ok := seen[resolved]; !ok {
			seen[resolved] = struct{}{}
			files = append(files, resolved)
		}
		return nil
	}
	for _, key := range keys {
		if err := add(inputString(inputs, key, ""), key); err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"images_in", "images", "reference_images", "ingredients", "product_images"} {
		items, ok := workflowMapItems(inputs[key])
		if !ok {
			continue
		}
		for _, item := range items {
			value, ok := item.(string)
			if !ok {
				continue
			}
			if err := add(value, key); err != nil {
				return nil, err
			}
		}
	}
	if len(files) > 10 {
		return nil, fmt.Errorf("Web AI accepts at most 10 input files")
	}
	return files, nil
}

func isSupportedFlowModel(m string) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	if m == "" || m == "active" || m == "auto" {
		return true
	}
	switch m {
	case "gpt-4o", "gpt-4o-mini", "o3-mini", "o1", "dall-e-3",
		"gemini-3.8-flash", "gemini-3.5-flash-lite", "gemini-3.1-pro", "gemini-4-argon",
		"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash", "imagen-3",
		"claude-3-7-sonnet", "claude-3-5-sonnet", "claude-3-5-haiku",
		"grok-3", "grok-3-think", "grok-3-deepsearch", "grok-2", "flux-1", "flux-1-schnell",
		"veo-2", "veo-3.1-fast", "muse-video", "muse-lipsync", "muse-gen2":
		return true
	default:
		return true
	}
}

func executeCustomAIPrompt(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prompt := resolveNodePrompt(node, inputs, []string{"input", "prompt"}, "")
	if prompt == "" {
		return nil, fmt.Errorf("custom AI prompt chưa được cấu hình")
	}
	systemPrompt := nodeParamString(node.Params, "system_prompt", "Bạn là trợ lý sáng tạo nội dung.")
	outputType := strings.ToLower(nodeParamString(node.Params, "output_type", "text"))
	model := strings.ToLower(nodeParamString(node.Params, "model", "active"))
	refImages, err := customImageInputs(ctx, node, inputs)
	if err != nil {
		return nil, err
	}

	switch outputType {
	case "text":
		if model != "active" && model != "auto" && !isSupportedFlowModel(model) {
			return nil, fmt.Errorf("model %q không được cấu hình; chọn active để dùng LLM của hệ thống", model)
		}
		if len(refImages) > 0 {
			if ctx.AnalyzeImages == nil {
				return nil, fmt.Errorf("phân tích ảnh chưa được nối")
			}
			visualContext, err := ctx.AnalyzeImages(ctx.Context, systemPrompt+"\n"+prompt, refImages)
			if err != nil {
				return nil, fmt.Errorf("phân tích ảnh đầu vào thất bại: %w", err)
			}
			prompt += "\n\nPhân tích ảnh nguồn:\n" + visualContext
		}
		if ctx.GenerateText == nil {
			return nil, fmt.Errorf("LLM chưa được nối")
		}
		textOut, err := ctx.GenerateText(ctx.Context, systemPrompt, prompt)
		if err != nil {
			return nil, fmt.Errorf("LLM thất bại: %w", err)
		}
		return map[string]any{"output": textOut, "text": textOut, "media": ""}, nil
	case "image":
		if model != "active" && model != "auto" && !isSupportedFlowModel(model) {
			return nil, fmt.Errorf("image model %q chưa được nối", model)
		}
		if ctx.GenerateImage == nil {
			return nil, fmt.Errorf("image generator chưa được nối")
		}
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.jpg", node.ID))
		if err := ctx.GenerateImage(ctx.Context, prompt, refImages, outPath); err != nil {
			return nil, fmt.Errorf("tạo ảnh thất bại: %w", err)
		}
		if !fileExists(outPath) || !isImageFile(outPath) {
			return nil, fmt.Errorf("image provider không tạo được tệp ảnh hợp lệ")
		}
		return map[string]any{"output": outPath, "text": "", "media": outPath}, nil
	case "video":
		provider, err := flowVideoProvider(nodeParamString(node.Params, "provider", "googleflow"))
		if err != nil {
			return nil, err
		}
		outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_ai.mp4", node.ID))
		if err := callFlowVideoProvider(ctx, provider, prompt, refImages, nodeParamString(node.Params, "aspect", "VIDEO_ASPECT_RATIO_PORTRAIT"), outPath); err != nil {
			return nil, err
		}
		return map[string]any{"output": outPath, "text": "", "media": outPath}, nil
	default:
		return nil, fmt.Errorf("kiểu output không hỗ trợ: %s", outputType)
	}
}

// resolveNodePrompt applies node-specific prompt text to connected inputs.
// Blank node prompt passes the upstream value through; {{input}} inserts the
// first configured text input when the operator writes a custom prompt.
func resolveNodePrompt(node *FlowNode, inputs map[string]any, inputKeys []string, fallback string) string {
	return resolveNodeText(node, inputs, "prompt", inputKeys, fallback)
}

func resolveNodeText(node *FlowNode, inputs map[string]any, paramKey string, inputKeys []string, fallback string) string {
	upstream := ""
	for _, key := range inputKeys {
		if value, ok := inputs[key]; ok && value != nil {
			if text := promptValue(value); strings.TrimSpace(text) != "" {
				upstream = text
				break
			}
		}
	}

	template := nodeParamString(node.Params, paramKey, "")
	if strings.TrimSpace(template) == "" {
		if upstream != "" {
			return strings.TrimSpace(upstream)
		}
		return strings.TrimSpace(fallback)
	}

	replacements := make([]string, 0, len(inputs)*4+6)
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		if key == "input" || key == "prompt" {
			continue // These names are handled as semantic prompt aliases below.
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := promptValue(inputs[key])
		replacements = append(replacements, "{{"+key+"}}", value, "{"+key+"}", value)
	}
	replacements = append(replacements, "{{input}}", upstream, "{input}", upstream)
	if prompt, ok := inputs["prompt"]; ok {
		replacements = append(replacements, "{{prompt}}", promptValue(prompt), "{prompt}", promptValue(prompt))
	}
	return strings.TrimSpace(strings.NewReplacer(replacements...).Replace(template))
}

func promptValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.Join(v, ", ")
	default:
		encoded, err := json.Marshal(v)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(v)
	}
}

func customImageInputs(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) ([]string, error) {
	images := make([]string, 0, 4)
	for _, key := range []string{"image", "image_in", "reference_image", "character_image", "product_image"} {
		path := inputString(inputs, key, "")
		if path != "" {
			resolved := resolveAssetPath(path, ctx.WorkDir)
			if resolved == "" || !isImageFile(resolved) {
				return nil, fmt.Errorf("ảnh đầu vào %s không hợp lệ", key)
			}
			images = append(images, resolved)
		}
	}
	asset := inputString(inputs, "asset_in", "")
	if asset != "" {
		resolved := resolveAssetPath(asset, ctx.WorkDir)
		switch {
		case resolved != "" && isImageFile(resolved):
			images = append(images, resolved)
		case resolved != "" && isVideoFile(resolved):
			if inputString(inputs, "video", inputString(inputs, "video_in", "")) != "" {
				return nil, fmt.Errorf("chỉ nối một video vào node")
			}
			inputs["video_in"] = resolved
		default:
			return nil, fmt.Errorf("file đầu vào không phải ảnh hoặc video hợp lệ")
		}
	}
	video := inputString(inputs, "video", inputString(inputs, "video_in", ""))
	if video != "" {
		video = resolveAssetPath(video, ctx.WorkDir)
		if video == "" || !isVideoFile(video) || !fileExists(video) {
			return nil, fmt.Errorf("video đầu vào không hợp lệ")
		}
		frame := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_custom_reference_frame.jpg", node.ID))
		if ctx.FFmpegRunner == nil {
			return nil, fmt.Errorf("trích xuất frame từ video cần FFmpeg")
		}
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-ss", "0", "-i", video, "-vframes", "1", "-q:v", "2", frame); err != nil || !fileExists(frame) || !isImageFile(frame) {
			return nil, fmt.Errorf("không trích xuất được frame từ video")
		}
		images = append(images, frame)
	}
	return images, nil
}

func flowVideoProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "googleflow", "google flow", "veo", "veo 2", "veo 3":
		return "googleflow", nil
	case "muse", "museai", "muse ai":
		return "museai", nil
	default:
		return "", fmt.Errorf("video provider %q chưa được nối; hỗ trợ Google Flow hoặc Muse AI", value)
	}
}

func callFlowVideoProvider(ctx *ExecutionContext, provider, prompt string, images []string, aspect, outPath string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("thiếu prompt tạo video")
	}
	if len(images) == 0 {
		return fmt.Errorf("tạo video cần ít nhất một ảnh tham chiếu thật")
	}
	var err error
	if strings.EqualFold(provider, "museai") && (ctx.MuseRenderWithSession != nil || ctx.MuseRender != nil) {
		err = renderMuseWorkflowNode(ctx, "Workflow Muse AI", prompt, images, outPath)
	} else {
		if ctx.EngineCall == nil {
			return fmt.Errorf("video provider %s chưa được nối", provider)
		}
		projectID, accountID := "", ""
		if strings.EqualFold(provider, "googleflow") {
			projectID, accountID = googleFlowThreadValues(ctx)
		}
		nextProjectID, nextAccountID, callErr := ctx.EngineCall(ctx.Context, provider, prompt, images, aspect, projectID, accountID, outPath)
		if strings.EqualFold(provider, "googleflow") {
			setGoogleFlowThreadValues(ctx, nextProjectID, nextAccountID)
		}
		err = callErr
	}
	if err != nil {
		return fmt.Errorf("%s không tạo được video: %w", provider, err)
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return fmt.Errorf("%s không tạo được tệp video hợp lệ", provider)
	}
	return nil
}

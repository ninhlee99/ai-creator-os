package flow

import (
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func resolveAssetPath(p string, workDir string, fallbacks ...string) string {
	if p != "" && fileExists(p) {
		return p
	}
	candidates := make([]string, 0, 16)
	if p != "" {
		base := filepath.Base(p)
		clean := strings.TrimPrefix(p, "/")
		clean = strings.TrimPrefix(clean, "static/")
		candidates = append(candidates,
			p,
			clean,
			filepath.Join("internal", "web", "static", clean),
			filepath.Join("internal", "web", "static", "presets", base),
			filepath.Join("data", "presets", "characters", base),
			filepath.Join("data", "uploads", base),
			filepath.Join("internal", "web", "static", base),
		)
		if workDir != "" {
			candidates = append(candidates, filepath.Join(workDir, base))
		}
	}
	for _, fb := range fallbacks {
		fbBase := filepath.Base(fb)
		fbClean := strings.TrimPrefix(fb, "/")
		fbClean = strings.TrimPrefix(fbClean, "static/")
		candidates = append(candidates,
			fb,
			fbClean,
			filepath.Join("internal", "web", "static", fbClean),
			filepath.Join("internal", "web", "static", "presets", fbBase),
			filepath.Join("data", "presets", "characters", fbBase),
			filepath.Join("data", "uploads", fbBase),
			filepath.Join("internal", "web", "static", fbBase),
		)
		if workDir != "" {
			candidates = append(candidates, filepath.Join(workDir, fbBase))
		}
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func decodeBase64DataURL(dataURL, outPath string) error {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return errors.New("invalid data url")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
	return os.WriteFile(outPath, raw, 0o644)
}

func executeCharacterInput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	modelPath := nodeParamString(node.Params, "model_path", "")
	charURL := nodeParamString(node.Params, "character_url", "")
	previewURL := nodeParamString(node.Params, "preview_url", "")
	modelName := nodeParamString(node.Params, "model_name", "")
	var characterImages []string

	// 1. Kiểm tra nếu có dữ liệu ảnh upload trực tiếp dạng base64
	if strings.HasPrefix(previewURL, "data:image/") {
		savedUpload := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_character_upload.jpg", node.ID))
		if err := decodeBase64DataURL(previewURL, savedUpload); err != nil {
			return nil, fmt.Errorf("ảnh nhân vật tải lên không hợp lệ: %w", err)
		}
		modelPath = savedUpload
	}

	// 2. Nếu có URL trực tuyến (TikTok, Reels, Instagram, Web URL)
	targetURL := charURL
	if targetURL == "" && strings.HasPrefix(previewURL, "http") {
		targetURL = previewURL
	}
	if targetURL != "" && (modelPath == "" || !fileExists(modelPath)) {
		ctx.Log(fmt.Sprintf("CharacterNode: Đang nạp nhân vật từ link trực tuyến: %s", targetURL))
		downloaded, _, err := DownloadMediaResource(ctx.Context, targetURL, ctx.WorkDir, "image")
		if err != nil {
			return nil, fmt.Errorf("không tải được ảnh nhân vật từ URL: %w", err)
		}
		if !fileExists(downloaded) {
			return nil, fmt.Errorf("URL không trả về file ảnh nhân vật hợp lệ")
		}
		modelPath = downloaded
	}

	// 3. Nếu modelPath là tên file tải lên (tìm trong data/uploads hoặc ctx.WorkDir)
	if modelPath != "" {
		if resolved := resolveAssetPath(modelPath, ctx.WorkDir); resolved != "" {
			modelPath = resolved
		}
	}
	if modelPath == "" || !fileExists(modelPath) {
		accountID := int64(nodeParamInt(node.Params, "account_id", 0))
		if accountID > 0 && ctx.CharacterPhotos != nil {
			photos, err := ctx.CharacterPhotos(accountID)
			if err != nil {
				return nil, fmt.Errorf("không tải được ảnh nhân vật của account: %w", err)
			}
			characterImages = append(characterImages, photos...)
			index := nodeParamInt(node.Params, "model_photo_index", 0)
			if index < 0 || index >= len(photos) {
				return nil, fmt.Errorf("model_photo_index ngoài phạm vi ảnh nhân vật (%d ảnh)", len(photos))
			}
			modelPath = photos[index]
		}
	}
	if modelPath == "" || !fileExists(modelPath) || !isImageFile(modelPath) {
		return nil, fmt.Errorf("chọn ảnh nhân vật hoặc account có ảnh đại diện hợp lệ")
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = strings.TrimSuffix(filepath.Base(modelPath), filepath.Ext(modelPath))
	}

	if len(characterImages) == 0 {
		characterImages = []string{modelPath}
	}
	ctx.Log(fmt.Sprintf("CharacterNode: Đã nạp nhân vật '%s' (%s)", modelName, filepath.Base(modelPath)))

	return map[string]any{
		"character_image":  modelPath,
		"character_images": characterImages,
		"character_name":   modelName,
	}, nil
}

func executeProductInput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prodImg := nodeParamString(node.Params, "product_image", "")
	prodURL := nodeParamString(node.Params, "product_url", "")
	previewURL := nodeParamString(node.Params, "preview_url", "")
	affURL := nodeParamString(node.Params, "affiliate_url", "")
	prodID := nodeParamString(node.Params, "product_id", "")
	prodTitle := nodeParamString(node.Params, "product_title", "")
	productInfo := map[string]any{}
	var productImageURLs []string

	if prodID != "" {
		id, err := strconv.ParseInt(prodID, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("product_id không hợp lệ: %q", prodID)
		}
		if ctx.ResolveProduct == nil {
			return nil, fmt.Errorf("Product Input chưa được nối với catalog")
		}
		product, err := ctx.ResolveProduct(ctx.Context, id)
		if err != nil {
			return nil, fmt.Errorf("không tải được sản phẩm %d: %w", id, err)
		}
		prodTitle = product.Title
		productImageURLs = append(productImageURLs, product.ImageURLs...)
		if len(product.ImageURLs) > 0 {
			prodURL = product.ImageURLs[0]
		}
		prodImg = ""
		affURL = product.ProductURL
		productInfo = map[string]any{
			"id": id, "title": product.Title, "price": product.Price, "currency": product.Currency,
			"commission_rate": product.CommissionRate, "commission_amount": product.CommissionAmount,
			"commission_currency": product.CommissionCurrency, "shop_name": product.ShopName,
			"category": product.Category, "url": product.ProductURL, "review_status": product.ReviewStatus,
		}
	}

	// 1. Kiểm tra nếu có dữ liệu ảnh sản phẩm upload trực tiếp dạng base64
	if strings.HasPrefix(previewURL, "data:image/") {
		savedUpload := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_product_upload.jpg", node.ID))
		if err := decodeBase64DataURL(previewURL, savedUpload); err != nil {
			return nil, fmt.Errorf("ảnh sản phẩm tải lên không hợp lệ: %w", err)
		}
		prodImg = savedUpload
	}

	// 2. Nếu có URL trực tuyến (TikTok, Reels, Instagram, Shopee, Web URL)
	targetURL := prodURL
	if targetURL == "" && strings.HasPrefix(previewURL, "http") {
		targetURL = previewURL
	}
	if targetURL != "" && (prodImg == "" || !fileExists(prodImg)) {
		ctx.Log(fmt.Sprintf("ProductNode: Đang nạp sản phẩm từ link trực tuyến: %s", targetURL))
		downloaded, _, err := DownloadMediaResource(ctx.Context, targetURL, ctx.WorkDir, "image")
		if err != nil {
			return nil, fmt.Errorf("không tải được ảnh sản phẩm từ URL: %w", err)
		}
		if !fileExists(downloaded) {
			return nil, fmt.Errorf("URL không trả về file ảnh sản phẩm hợp lệ")
		}
		prodImg = downloaded
	}

	// 3. Nếu prodImg là tên file tải lên (trong data/uploads hoặc ctx.WorkDir)
	if prodImg != "" {
		if resolved := resolveAssetPath(prodImg, ctx.WorkDir); resolved != "" {
			prodImg = resolved
		}
	}
	if prodImg == "" || !fileExists(prodImg) || !isImageFile(prodImg) {
		return nil, fmt.Errorf("chọn sản phẩm trong catalog hoặc cung cấp ảnh sản phẩm thật")
	}
	if strings.TrimSpace(prodTitle) == "" {
		return nil, fmt.Errorf("thiếu tên sản phẩm")
	}
	productInfo["id"] = prodID
	productInfo["title"] = prodTitle
	productInfo["image"] = prodImg
	productInfo["url"] = affURL
	productImages := []string{prodImg}
	if len(productImageURLs) > 1 {
		for _, imageURL := range productImageURLs[1:] {
			if len(productImages) >= 4 {
				break
			}
			imagePath, _, err := DownloadMediaResource(ctx.Context, imageURL, ctx.WorkDir, "image")
			if err != nil || !fileExists(imagePath) || !isImageFile(imagePath) {
				return nil, fmt.Errorf("không tải được ảnh phụ của sản phẩm %q", prodTitle)
			}
			productImages = append(productImages, imagePath)
		}
	}
	productInfo["images"] = productImages

	ctx.Log(fmt.Sprintf("ProductNode: Đã tải sản phẩm '%s' (%s)", prodTitle, filepath.Base(prodImg)))

	return map[string]any{
		"product_image":  prodImg,
		"product_images": productImages,
		"product_title":  prodTitle,
		"product_info":   productInfo,
	}, nil
}

func executeVideoSource(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoPath := nodeParamString(node.Params, "video_path", "")
	videoURL := nodeParamString(node.Params, "video_url", "")
	previewURL := nodeParamString(node.Params, "preview_url", "")
	videoID := ""
	author := ""
	soundTitle := ""
	radarVerified := false

	// 1. Kiểm tra video tải lên dạng base64
	if strings.HasPrefix(previewURL, "data:video/") {
		savedUpload := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_video_upload.mp4", node.ID))
		if err := decodeBase64DataURL(previewURL, savedUpload); err != nil {
			return nil, fmt.Errorf("video tải lên không hợp lệ: %w", err)
		}
		videoPath = savedUpload
	}

	// 2. Nếu có URL video từ TikTok, Reels, Instagram, YouTube Shorts hoặc Web URL
	targetURL := videoURL
	if targetURL == "" && strings.HasPrefix(previewURL, "http") {
		targetURL = previewURL
	}
	if targetURL != "" && (videoPath == "" || !fileExists(videoPath)) {
		if IsSocialMediaURL(targetURL) {
			if ctx.ResolveRadarVideo == nil {
				return nil, fmt.Errorf("TikTok source cần Radar video đã lưu")
			}
			resolved, err := ctx.ResolveRadarVideo(ctx.Context, targetURL, nodeParamBool(node.Params, "require_radar_dance", false))
			if err != nil {
				return nil, fmt.Errorf("không xác thực được video với Radar: %w", err)
			}
			downloaded, _, err := DownloadMediaResource(ctx.Context, resolved.VideoURL, ctx.WorkDir, "video")
			if err != nil {
				return nil, fmt.Errorf("không tải được video đã xác thực từ Radar: %w", err)
			}
			videoPath = downloaded
			videoID = resolved.VideoID
			author = resolved.Author
			soundTitle = resolved.SoundTitle
			radarVerified = true
		} else {
			if !nodeParamBool(node.Params, "allow_external_url", false) {
				return nil, fmt.Errorf("URL không phải nguồn Radar; bật allow_external_url để tải nguồn trực tiếp")
			}
			ctx.Log(fmt.Sprintf("VideoSourceNode: Đang tải video từ URL trực tiếp: %s", targetURL))
			downloaded, _, err := DownloadMediaResource(ctx.Context, targetURL, ctx.WorkDir, "video")
			if err != nil {
				return nil, fmt.Errorf("không tải được video: %w", err)
			}
			videoPath = downloaded
		}
	}

	// 3. Kiểm tra file cục bộ đã tải lên
	if videoPath != "" {
		if resolved := resolveAssetPath(videoPath, ctx.WorkDir); resolved != "" {
			videoPath = resolved
		}
	}
	if videoPath == "" || !fileExists(videoPath) || !isVideoFile(videoPath) {
		return nil, fmt.Errorf("Video Source cần video thật từ upload, Radar hoặc URL được cho phép")
	}
	if nodeParamBool(node.Params, "require_radar_dance", false) && !radarVerified {
		return nil, fmt.Errorf("workflow này yêu cầu video nhảy đã xác thực trong Radar")
	}

	duration, err := probeFlowDuration(videoPath)
	if err != nil {
		return nil, fmt.Errorf("không đọc được thời lượng video: %w", err)
	}
	if duration <= 0 {
		return nil, fmt.Errorf("thời lượng video không hợp lệ")
	}
	audioTrack := ""
	if hasAudio, _ := probeFlowHasAudio(videoPath); hasAudio && ctx.FFmpegRunner != nil {
		audioTrack = filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_source_audio.aac", node.ID))
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-i", videoPath, "-vn", "-c:a", "aac", audioTrack); err != nil || !fileExists(audioTrack) {
			return nil, fmt.Errorf("không trích xuất được audio từ video nguồn")
		}
	}
	ctx.Log(fmt.Sprintf("VideoSourceNode: Đã nạp video nguồn (id=%s, file=%s)", videoID, filepath.Base(videoPath)))

	return map[string]any{
		"video_path":  videoPath,
		"audio_track": audioTrack,
		"duration":    duration,
		"video_id":    videoID,
		"author":      author,
		"sound_title": soundTitle,
	}, nil
}

func executeAudioSource(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	audioPath := nodeParamString(node.Params, "audio_path", "")
	audioURL := nodeParamString(node.Params, "audio_url", "")
	previewURL := nodeParamString(node.Params, "preview_url", "")
	audioTitle := nodeParamString(node.Params, "audio_title", "")
	if strings.HasPrefix(previewURL, "data:audio/") {
		headerEnd := strings.Index(previewURL, ",")
		if headerEnd < 0 {
			return nil, fmt.Errorf("upload audio có data URL không hợp lệ")
		}
		ext := audioExtension(previewURL[:headerEnd])
		audioPath = filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_audio_upload.%s", node.ID, ext))
		if err := decodeBase64DataURL(previewURL, audioPath); err != nil {
			return nil, fmt.Errorf("upload audio không hợp lệ: %w", err)
		}
	}
	if audioURL == "" && strings.HasPrefix(previewURL, "http") {
		audioURL = previewURL
	}
	if audioURL != "" && (audioPath == "" || !fileExists(audioPath)) {
		downloaded, _, err := DownloadMediaResource(ctx.Context, audioURL, ctx.WorkDir, "audio")
		if err != nil {
			return nil, fmt.Errorf("không tải được audio: %w", err)
		}
		audioPath = downloaded
	}

	if audioPath != "" {
		if resolved := resolveAssetPath(audioPath, ctx.WorkDir); resolved != "" {
			audioPath = resolved
		}
	}
	if audioPath == "" || !fileExists(audioPath) || !isAudioFile(audioPath) {
		return nil, fmt.Errorf("Audio Source cần file audio thật hoặc audio URL hợp lệ")
	}
	if audioTitle == "" {
		audioTitle = strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	}

	ctx.Log(fmt.Sprintf("AudioSourceNode: Đã nạp âm thanh '%s' (%s)", audioTitle, filepath.Base(audioPath)))

	return map[string]any{
		"audio_file":  audioPath,
		"audio_title": audioTitle,
	}, nil
}

func executePromptTemplate(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	template := nodeParamString(node.Params, "template", "Cinematic vertical 9:16 video of {character} {motion}, showcasing {product}. Beautiful soft lighting, 4k ultra realistic, smooth motion.")

	charDesc := inputString(inputs, "character_desc", "the adult character from the reference images")
	prodDesc := inputString(inputs, "product_desc", "the featured product")
	motionDesc := inputString(inputs, "motion_desc", "performing a graceful and dynamic modern dance choreography")

	prompt := strings.ReplaceAll(template, "{character}", charDesc)
	prompt = strings.ReplaceAll(prompt, "{product}", prodDesc)
	prompt = strings.ReplaceAll(prompt, "{motion}", motionDesc)

	// Chuẩn hóa khoảng trắng
	prompt = strings.Join(strings.Fields(prompt), " ")
	ctx.Log(fmt.Sprintf("PromptTemplateNode: Đã tạo câu lệnh prompt (%d ký tự)", len(prompt)))

	return map[string]any{
		"prompt": prompt,
	}, nil
}

func inputString(inputs map[string]any, key, fallback string) string {
	if inputs == nil {
		return fallback
	}
	if val, ok := inputs[key]; ok && val != nil {
		if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return fallback
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

func isVideoFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".mp4" || ext == ".mov" || ext == ".webm" || ext == ".mkv" || ext == ".avi"
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".mp3" || ext == ".aac" || ext == ".wav" || ext == ".m4a" || ext == ".ogg" || ext == ".flac"
}

func probeFlowDuration(path string) (float64, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, fmt.Errorf("ffprobe chưa được cài")
	}
	output, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe lỗi: %w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("thời lượng video không hợp lệ")
	}
	return duration, nil
}

func probeFlowHasAudio(path string) (bool, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return false, fmt.Errorf("ffprobe chưa được cài")
	}
	output, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_type", "-of", "csv=p=0", path).Output()
	if err != nil {
		return false, fmt.Errorf("ffprobe audio lỗi: %w", err)
	}
	return strings.TrimSpace(string(output)) == "audio", nil
}

func writeDummyImage(path string, width, height int, label string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 210, B: 220, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
}

func writeDummyVideo(ctx *ExecutionContext, path string, duration float64) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if ctx != nil && ctx.FFmpegRunner != nil {
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-f", "lavfi", "-i", fmt.Sprintf("color=c=purple:s=1080x1920:d=%.2f", duration), "-c:v", "libx264", "-pix_fmt", "yuv420p", path); err == nil {
			return nil
		}
	}
	mp4Minimal := []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0x00, 0x00, 0x00, 0x00, 'm', 'p', '4', '2', 'i', 's', 'o', 'm',
		0x00, 0x00, 0x00, 0x08, 'f', 'r', 'e', 'e',
		0x00, 0x00, 0x00, 0x10, 'm', 'd', 'a', 't', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	return os.WriteFile(path, mp4Minimal, 0o644)
}

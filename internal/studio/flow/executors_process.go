package flow

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
)

func executeProductExtractor(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	// 1. Tìm nguồn tài nguyên: chấp nhận cả Ảnh hoặc Video
	src := inputString(inputs, "source_image", "")
	if src == "" {
		src = inputString(inputs, "source_video", "")
	}
	if src == "" {
		src = inputString(inputs, "video_in", "")
	}
	if src == "" {
		src = inputString(inputs, "video_path", "")
	}
	if src == "" {
		src = inputString(inputs, "product_image", "")
	}
	if src == "" {
		src = nodeParamString(node.Params, "source_url", "")
	}
	if src == "" {
		src = nodeParamString(node.Params, "preview_url", "")
	}
	if src == "" {
		src = nodeParamString(node.Params, "product_image", "")
	}
	if src == "" {
		src = nodeParamString(node.Params, "source_image", "")
	}
	if src == "" {
		src = nodeParamString(node.Params, "image", "")
	}

	// 2. Tìm kiếm trong kho tài nguyên cục bộ
	if src != "" {
		if resolved := resolveAssetPath(src, ctx.WorkDir); resolved != "" {
			src = resolved
		}
	}

	// 3. Nếu là URL trực tuyến (TikTok, Reels, Instagram, Shopee, Web URL)
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		ctx.Log(fmt.Sprintf("ProductExtractorNode: Đang tải tài nguyên nguồn từ link: %s", src))
		downloaded, _, err := DownloadMediaResource(ctx.Context, src, ctx.WorkDir, "auto")
		if err != nil {
			return nil, fmt.Errorf("không tải được nguồn sản phẩm: %w", err)
		}
		if !fileExists(downloaded) {
			return nil, fmt.Errorf("URL không trả về file ảnh hoặc video hợp lệ")
		}
		src = downloaded
	}

	if src == "" || !fileExists(src) {
		return nil, fmt.Errorf("chưa có ảnh hoặc video nguồn; nối nguồn hoặc chọn file trong node bóc tách sản phẩm")
	}

	sourceImage := src
	// 4. Kiểm tra nếu nguồn đầu vào là Video: trích xuất khung hình chứa sản phẩm
	if isVideoFile(src) {
		extractedFrame := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_extracted_frame.jpg", node.ID))
		timestamp := 2.0
		if tsStr := nodeParamString(node.Params, "video_timestamp", ""); tsStr != "" && tsStr != "auto" {
			var ts float64
			if _, err := fmt.Sscanf(tsStr, "%f", &ts); err == nil && ts > 0 {
				timestamp = ts
			}
		}

		if err := ExtractFrameFromVideo(ctx.Context, src, extractedFrame, timestamp); err != nil {
			return nil, fmt.Errorf("không trích xuất được frame video: %w", err)
		}
		sourceImage = extractedFrame
		ctx.Log(fmt.Sprintf("ProductExtractorNode: Đã trích xuất khung hình sản phẩm từ video '%s' tại %.1fs", filepath.Base(src), timestamp))
	} else {
		ctx.Log(fmt.Sprintf("ProductExtractorNode: Nhận ảnh sản phẩm '%s' để tiến hành bóc tách phông nền", filepath.Base(src)))
	}

	cutoutPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_cutout.png", node.ID))
	maskPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_mask.png", node.ID))

	autoCrop := nodeParamBool(node.Params, "auto_crop", true)
	refineEdges := nodeParamBool(node.Params, "refine_edges", true)
	// 5. Tiến hành bóc tách sản phẩm, loại bỏ phông nền tạo PNG trong suốt
	isolateGarment := nodeParamBool(node.Params, "isolate_garment", false) || nodeParamString(node.Params, "subject_type", "") == "garment"
	removeSkin := nodeParamBool(node.Params, "remove_skin", false)
	if err := segmentGarmentToCutout(sourceImage, cutoutPath, maskPath, autoCrop, refineEdges, isolateGarment, removeSkin); err != nil {
		return nil, fmt.Errorf("không tách được nền sản phẩm: %w", err)
	}
	if !fileExists(cutoutPath) || !fileExists(maskPath) {
		return nil, fmt.Errorf("bộ tách nền không tạo đủ ảnh cutout và mask")
	}

	ctx.Log(fmt.Sprintf("ProductExtractorNode: Đã bóc tách thành công sản phẩm trong suốt -> %s", filepath.Base(cutoutPath)))

	return map[string]any{
		"cutout_image":  cutoutPath,
		"mask_image":    maskPath,
		"source_frame":  sourceImage,
		"product_image": cutoutPath,
	}, nil
}

func segmentProductToCutout(sourceImgPath, cutoutPath, maskPath string, autoCrop bool, refineEdges bool) error {
	return segmentGarmentToCutout(sourceImgPath, cutoutPath, maskPath, autoCrop, refineEdges, false, false)
}

func segmentGarmentToCutout(sourceImgPath, cutoutPath, maskPath string, autoCrop bool, refineEdges bool, isolateGarment bool, removeSkin bool) error {
	// 0. Ưu tiên tuyệt đối bộ tách nền Neural Matting AI (Silueta / U2-Net ONNX)
	// Khử hoàn toàn viền đen (De-fringing) và hỗ trợ cô lập riêng trang phục (bỏ khuôn mặt/đầu/da)
	if err := tryAINeuralCutout(sourceImgPath, cutoutPath, maskPath, autoCrop, refineEdges, isolateGarment, removeSkin); err == nil {
		return nil
	}

	f, err := os.Open(sourceImgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	bounds := srcImg.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return errors.New("kích thước ảnh nguồn không hợp lệ")
	}

	// 1. Lấy mẫu bảng màu nền từ 4 đường biên ngoài cùng (Border Color Palette)
	type rgbColor struct {
		r, g, b float64
	}
	var bgPalette []rgbColor

	addSample := func(x, y int) {
		r, g, b, a := srcImg.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
		if (a >> 8) < 20 {
			return
		}
		r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
		for _, pal := range bgPalette {
			dr, dg, db := pal.r-r8, pal.g-g8, pal.b-b8
			if math.Sqrt(dr*dr+dg*dg+db*db) < 14.0 {
				return
			}
		}
		bgPalette = append(bgPalette, rgbColor{r8, g8, b8})
	}

	step := max(1, w/40)
	for x := 0; x < w; x += step {
		addSample(x, 0)
		addSample(x, h-1)
	}
	stepY := max(1, h/40)
	for y := 0; y < h; y += stepY {
		addSample(0, y)
		addSample(w-1, y)
	}

	if len(bgPalette) == 0 {
		bgPalette = append(bgPalette, rgbColor{255, 255, 255})
	}

	matchBg := func(r8, g8, b8 uint8, tol float64) bool {
		rF, gF, bF := float64(r8), float64(g8), float64(b8)
		for _, pal := range bgPalette {
			dr, dg, db := pal.r-rF, pal.g-gF, pal.b-bF
			if math.Sqrt(dr*dr+dg*dg+db*db) <= tol {
				return true
			}
		}
		return false
	}

	// 2. Thuật toán Loang từ viền ngoài (Border-Connected Flood Fill)
	isBg := make([]bool, w*h)
	queue := make([]int, 0, w*h/4)

	baseThreshold := 26.0

	enqueueIfBg := func(x, y int) {
		idx := y*w + x
		if isBg[idx] {
			return
		}
		r, g, b, a := srcImg.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
		a8 := uint8(a >> 8)
		if a8 < 20 || matchBg(uint8(r>>8), uint8(g>>8), uint8(b>>8), baseThreshold) {
			isBg[idx] = true
			queue = append(queue, idx)
		}
	}

	for x := 0; x < w; x++ {
		enqueueIfBg(x, 0)
		enqueueIfBg(x, h-1)
	}
	for y := 0; y < h; y++ {
		enqueueIfBg(0, y)
		enqueueIfBg(w-1, y)
	}

	dirs := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		cx := curr % w
		cy := curr / w

		for _, d := range dirs {
			nx, ny := cx+d[0], cy+d[1]
			if nx >= 0 && nx < w && ny >= 0 && ny < h {
				nIdx := ny*w + nx
				if !isBg[nIdx] {
					r, g, b, a := srcImg.At(bounds.Min.X+nx, bounds.Min.Y+ny).RGBA()
					a8 := uint8(a >> 8)
					if a8 < 20 || matchBg(uint8(r>>8), uint8(g>>8), uint8(b>>8), baseThreshold+4.0) {
						isBg[nIdx] = true
						queue = append(queue, nIdx)
					}
				}
			}
		}
	}

	// 3. Làm mịn biên kênh Alpha (Anti-Aliasing 5x5 Box Filter Smoothing)
	// Khử hoàn toàn hiện tượng bậc thang răng cưa và vết loang lổ
	smoothAlpha := make([]uint8, w*h)
	radius := 2
	if !refineEdges {
		radius = 0
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if radius == 0 {
				if isBg[y*w+x] {
					smoothAlpha[y*w+x] = 0
				} else {
					smoothAlpha[y*w+x] = 255
				}
				continue
			}
			sum := 0
			count := 0
			for dy := -radius; dy <= radius; dy++ {
				ny := y + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -radius; dx <= radius; dx++ {
					nx := x + dx
					if nx < 0 || nx >= w {
						continue
					}
					if !isBg[ny*w+nx] {
						sum += 255
					}
					count++
				}
			}
			smoothAlpha[y*w+x] = uint8(sum / count)
		}
	}

	rgbaImg := image.NewRGBA(image.Rect(0, 0, w, h))
	grayMask := image.NewGray(image.Rect(0, 0, w, h))

	minX, minY := w, h
	maxX, maxY := 0, 0

	// 4. Tổng hợp màu sắc với De-fringing (khử ám màu phông cũ ở đường biên)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			outAlpha := smoothAlpha[idx]

			c := srcImg.At(bounds.Min.X+x, bounds.Min.Y+y)
			r, g, b, _ := c.RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

			// De-fringe: ở các pixel bán trong suốt (10 < alpha < 240), lấy mẫu màu sắc từ vùng đặc bên trong
			if refineEdges && outAlpha > 15 && outAlpha < 240 {
				var sumR, sumG, sumB, countFg int
				for dy := -2; dy <= 2; dy++ {
					ny := y + dy
					if ny < 0 || ny >= h {
						continue
					}
					for dx := -2; dx <= 2; dx++ {
						nx := x + dx
						if nx < 0 || nx >= w {
							continue
						}
						if smoothAlpha[ny*w+nx] >= 245 {
							cNear := srcImg.At(bounds.Min.X+nx, bounds.Min.Y+ny)
							nr, ng, nb, _ := cNear.RGBA()
							sumR += int(nr >> 8)
							sumG += int(ng >> 8)
							sumB += int(nb >> 8)
							countFg++
						}
					}
				}
				if countFg > 0 {
					r8 = uint8(sumR / countFg)
					g8 = uint8(sumG / countFg)
					b8 = uint8(sumB / countFg)
				}
			}

			if outAlpha > 20 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}

			rgbaImg.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: outAlpha})
			grayMask.SetGray(x, y, color.Gray{Y: outAlpha})
		}
	}

	// 5. Crop sát viền sản phẩm nếu autoCrop được bật
	finalCutout := image.Image(rgbaImg)
	finalMask := image.Image(grayMask)

	if autoCrop && minX < maxX && minY < maxY {
		pad := 20
		cropMinX := max(0, minX-pad)
		cropMinY := max(0, minY-pad)
		cropMaxX := min(w, maxX+pad)
		cropMaxY := min(h, maxY+pad)

		cropRect := image.Rect(cropMinX, cropMinY, cropMaxX, cropMaxY)
		croppedCutout := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
		draw.Draw(croppedCutout, croppedCutout.Bounds(), rgbaImg, cropRect.Min, draw.Src)
		finalCutout = croppedCutout

		croppedMask := image.NewGray(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
		draw.Draw(croppedMask, croppedMask.Bounds(), grayMask, cropRect.Min, draw.Src)
		finalMask = croppedMask
	}

	// 6. Ghi file PNG trong suốt và mặt nạ
	_ = os.MkdirAll(filepath.Dir(cutoutPath), 0o755)
	_ = os.MkdirAll(filepath.Dir(maskPath), 0o755)

	outF, err := os.Create(cutoutPath)
	if err != nil {
		return err
	}
	defer outF.Close()
	if err := png.Encode(outF, finalCutout); err != nil {
		return err
	}

	mF, err := os.Create(maskPath)
	if err != nil {
		return err
	}
	defer mF.Close()
	return png.Encode(mF, finalMask)
}

func tryAINeuralCutout(sourceImgPath, cutoutPath, maskPath string, autoCrop bool, refineEdges bool, isolateGarment bool, removeSkin bool) error {
	scriptCandidates := []string{
		"data/scripts/ai_cutout.py",
		filepath.Join(filepath.Dir(sourceImgPath), "../scripts/ai_cutout.py"),
	}
	var scriptPath string
	for _, s := range scriptCandidates {
		if fileExists(s) {
			scriptPath = s
			break
		}
	}
	if scriptPath == "" {
		return errors.New("ai_cutout.py script not found")
	}

	pyCandidates := []string{
		"data/third_party/vieneu-tts/.venv/bin/python",
		"data/third_party/vieneu-tts/.venv/bin/python3",
		"python3",
		"python",
	}
	var pyExec string
	for _, p := range pyCandidates {
		if fileExists(p) {
			pyExec = p
			break
		}
	}
	if pyExec == "" {
		pyExec = "python3"
	}

	absSource, _ := filepath.Abs(sourceImgPath)
	absCutout, _ := filepath.Abs(cutoutPath)
	absMask, _ := filepath.Abs(maskPath)

	args := []string{scriptPath, absSource, absCutout, absMask}
	if autoCrop {
		args = append(args, "--auto-crop")
	}
	if refineEdges {
		args = append(args, "--feather", "0.6")
	} else {
		args = append(args, "--feather", "0.0")
	}
	if isolateGarment {
		args = append(args, "--isolate-garment")
	}
	if removeSkin {
		args = append(args, "--remove-skin")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pyExec, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ai_cutout failed: %v (output: %s)", err, string(out))
	}

	if !fileExists(cutoutPath) || !fileExists(maskPath) {
		return errors.New("ai_cutout outputs missing")
	}
	return nil
}

func executeImageComposite(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	charImg := inputString(inputs, "character_image", "")
	prodImg := inputString(inputs, "product_image", "")

	if charImg == "" || !fileExists(charImg) {
		return nil, fmt.Errorf("ImageCompositeNode: Thiếu ảnh nhân vật mẫu")
	}
	if prodImg == "" || !fileExists(prodImg) {
		return nil, fmt.Errorf("ImageCompositeNode: Thiếu ảnh sản phẩm mẫu")
	}

	compositePath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_composite.jpg", node.ID))

	placement := nodeParamString(node.Params, "placement", "holding_hand")
	scale := nodeParamFloat(node.Params, "scale", 0.45)

	// Ghép sản phẩm vào nhân vật mẫu theo vị trí tự nhiên
	if placement != "beside" && placement != "foreground" && placement != "holding_hand" {
		return nil, fmt.Errorf("vị trí ghép không hỗ trợ: %s", placement)
	}
	if scale <= 0 || scale > 1 {
		return nil, fmt.Errorf("tỷ lệ sản phẩm phải lớn hơn 0 và không vượt quá 1")
	}
	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("Image Composite cần FFmpeg")
	}
	overlayX, overlayY := "W-w-50", "H-h-120"
	if placement == "beside" {
		overlayX, overlayY = "50", "(H-h)/2"
	} else if placement == "foreground" {
		overlayX, overlayY = "(W-w)/2", "H-h-80"
	}
	scaleFilter := fmt.Sprintf("scale=iw*%.2f:-1", scale)
	filterGraph := fmt.Sprintf("[1:v]%s[p];[0:v][p]overlay=%s:%s", scaleFilter, overlayX, overlayY)
	if err := ctx.FFmpegRunner(ctx.Context, "-y", "-i", charImg, "-i", prodImg, "-filter_complex", filterGraph, "-q:v", "2", compositePath); err != nil {
		return nil, fmt.Errorf("FFmpeg ghép ảnh thất bại: %w", err)
	}
	if !fileExists(compositePath) || !isImageFile(compositePath) {
		return nil, fmt.Errorf("FFmpeg không tạo được ảnh ghép hợp lệ")
	}

	ctx.Log(fmt.Sprintf("ImageCompositeNode: Đã ghép sản phẩm với nhân vật mẫu (kiểu=%s) -> %s", placement, filepath.Base(compositePath)))

	return map[string]any{
		"composite_image": compositePath,
	}, nil
}

func executeDanceMotionExtractor(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" {
		videoIn = inputString(inputs, "video_path", "")
	}
	if videoIn == "" {
		videoIn = nodeParamString(node.Params, "video_path", "")
	}
	if videoIn != "" {
		if resolved := resolveAssetPath(videoIn, ctx.WorkDir); resolved != "" {
			videoIn = resolved
		}
	}
	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("DanceMotionExtractorNode: Thiếu video gốc")
	}
	duration, err := probeFlowDuration(videoIn)
	if err != nil {
		return nil, fmt.Errorf("không đọc được thời lượng video nhảy: %w", err)
	}
	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("bóc tách vũ đạo cần FFmpeg")
	}
	frameCount := nodeParamInt(node.Params, "keyframe_count", 5)
	if frameCount < 2 || frameCount > 12 {
		return nil, fmt.Errorf("keyframe_count phải từ 2 đến 12")
	}
	keyframes := make([]string, 0, frameCount)
	for i := 0; i < frameCount; i++ {
		timestamp := duration * float64(i+1) / float64(frameCount+1)
		frame := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_keyframe_%02d.jpg", node.ID, i+1))
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-ss", fmt.Sprintf("%.3f", timestamp), "-i", videoIn, "-vframes", "1", "-q:v", "2", frame); err != nil || !fileExists(frame) {
			return nil, fmt.Errorf("không trích xuất được keyframe %.2fs", timestamp)
		}
		keyframes = append(keyframes, frame)
	}
	if ctx.AnalyzeImages == nil {
		return nil, fmt.Errorf("bóc tách chuyển động cần Vision AI đã cấu hình")
	}
	styleHint := nodeParamString(node.Params, "dance_style", "")
	analysisPrompt := fmt.Sprintf("Phân tích thứ tự chuyển động cơ thể qua các keyframe của video dài %.2f giây. Mô tả pose, hướng, nhịp và chuyển tiếp chỉ từ hình đã thấy; không đoán BPM. Gợi ý phong cách: %s", duration, styleHint)
	movement, err := ctx.AnalyzeImages(ctx.Context, analysisPrompt, keyframes)
	if err != nil {
		return nil, fmt.Errorf("Vision AI không phân tích được chuyển động từ keyframe: %w", err)
	}
	if strings.TrimSpace(movement) == "" {
		return nil, fmt.Errorf("Vision AI trả mô tả chuyển động rỗng")
	}
	audioPath := ""
	if hasAudio, err := probeFlowHasAudio(videoIn); err != nil {
		return nil, err
	} else if hasAudio {
		audioPath = filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_beat_audio.aac", node.ID))
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-i", videoIn, "-vn", "-c:a", "aac", audioPath); err != nil || !fileExists(audioPath) {
			return nil, fmt.Errorf("không trích xuất được audio nguồn")
		}
	} else if nodeParamBool(node.Params, "require_audio", false) {
		return nil, fmt.Errorf("video nhảy đã chọn không có audio")
	}

	motionData := map[string]any{"movement_analysis": movement, "duration_seconds": duration, "keyframes_count": len(keyframes)}
	ctx.Log(fmt.Sprintf("DanceMotionExtractorNode: Phân tích %d keyframe trong video %.2fs", len(keyframes), duration))

	return map[string]any{
		"motion_data": motionData,
		"keyframes":   keyframes,
		"beat_audio":  audioPath,
		"dance_style": movement,
	}, nil
}

func executeDanceRetarget(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	charImg := inputString(inputs, "character_image", "")
	if charImg == "" {
		charImg = inputString(inputs, "fitted_image", "")
	}
	if charImg != "" {
		if resolved := resolveAssetPath(charImg, ctx.WorkDir); resolved != "" {
			charImg = resolved
		}
	}
	if charImg == "" || !fileExists(charImg) || !isImageFile(charImg) {
		return nil, fmt.Errorf("DanceRetargetNode: Thiếu ảnh nhân vật mẫu")
	}

	expression := nodeParamString(node.Params, "expression", "sensual_alluring")
	danceType := nodeParamString(node.Params, "dance_type", "sexy_dance")
	envDesc := inputString(inputs, "environment_desc", "")
	lightDesc := inputString(inputs, "lighting_desc", "")
	outfitDesc := inputString(inputs, "outfit_desc", "")
	motionData, ok := inputs["motion_data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Dance Prompt Builder cần motion_data từ video source")
	}
	movement := strings.TrimSpace(fmt.Sprint(motionData["movement_analysis"]))
	if movement == "" || movement == "<nil>" {
		return nil, fmt.Errorf("motion_data không có phân tích chuyển động")
	}

	var keyframes []string
	if kfRaw, ok := inputs["keyframes"]; ok {
		if kfList, ok := kfRaw.([]string); ok {
			keyframes = kfList
		}
	}

	prompt := fmt.Sprintf("Create a vertical 9:16 dance video with the character from the reference image. Use this observed motion sequence as guidance; do not claim exact motion capture: %s. Dance style: %s. Expression: %s. Setting: %s. Lighting: %s. Outfit: %s. Preserve the visible character identity and keep one subject.", movement, danceType, expression, envDesc, lightDesc, outfitDesc)

	ingredients := []string{charImg}
	for _, kf := range keyframes {
		if fileExists(kf) && len(ingredients) < 3 {
			ingredients = append(ingredients, kf)
		}
	}
	if extra, ok := inputs["ingredients"]; ok {
		for _, path := range flowStringList(extra) {
			if fileExists(path) && isImageFile(path) && len(ingredients) < 4 && !containsString(ingredients, path) {
				ingredients = append(ingredients, path)
			}
		}
	}

	ctx.Log(fmt.Sprintf("Dance Prompt Builder: Tạo prompt từ phân tích chuyển động (style=%s)", danceType))

	return map[string]any{
		"prompt_choreography": prompt,
		"prompt":              prompt,
		"ingredients":         ingredients,
		"character_image":     charImg,
	}, nil
}

func executeAudioMerge(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	audioIn := inputString(inputs, "audio_in", "")
	audioCodec := strings.ToLower(nodeParamString(node.Params, "audio_codec", "aac"))
	if audioCodec != "aac" && audioCodec != "copy" {
		return nil, fmt.Errorf("audio_codec chỉ hỗ trợ aac hoặc copy")
	}

	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("AudioMergeNode: Thiếu file video đầu vào")
	}
	if audioIn == "" {
		hasAudio, err := probeFlowHasAudio(videoIn)
		if err != nil {
			return nil, err
		}
		if !hasAudio {
			return nil, fmt.Errorf("không có audio riêng hoặc audio nhúng trong video")
		}
		return map[string]any{"video_out": videoIn}, nil
	}
	if !fileExists(audioIn) || !isAudioFile(audioIn) {
		return nil, fmt.Errorf("AudioMergeNode: Track audio không hợp lệ")
	}

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_audio_merged.mp4", node.ID))

	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("Audio Merge cần FFmpeg")
	}
	args := []string{"-y", "-i", videoIn, "-i", audioIn, "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", audioCodec, "-shortest", outPath}
	if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
		return nil, fmt.Errorf("không ghép được audio: %w", err)
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return nil, fmt.Errorf("FFmpeg không tạo được video có audio")
	}

	ctx.Log(fmt.Sprintf("AudioMergeNode: Đã ghép âm thanh vào video -> %s", filepath.Base(outPath)))

	return map[string]any{
		"video_out": outPath,
	}, nil
}

func executeVideoUpscale(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("VideoUpscaleNode: Thiếu file video đầu vào")
	}

	resolution := strings.ToLower(nodeParamString(node.Params, "resolution", "4k"))
	crf := nodeParamInt(node.Params, "crf", 18)
	if crf < 0 || crf > 51 {
		return nil, fmt.Errorf("CRF phải từ 0 đến 51")
	}
	targetScale := "1080:1920"
	if resolution == "4k" || resolution == "2160p" {
		targetScale = "2160:3840"
	} else if resolution != "1080p" {
		return nil, fmt.Errorf("resolution chưa hỗ trợ: %s", resolution)
	}

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_upscaled_%s.mp4", node.ID, resolution))

	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("Video Upscale cần FFmpeg")
	}
	if err := ctx.FFmpegRunner(ctx.Context, "-y", "-i", videoIn, "-vf", fmt.Sprintf("scale=%s:force_original_aspect_ratio=decrease:flags=lanczos,pad=%s:(ow-iw)/2:(oh-ih)/2,unsharp=5:5:0.8:3:3:0.4", targetScale, targetScale), "-c:v", "libx264", "-preset", "veryfast", "-crf", fmt.Sprint(crf), "-pix_fmt", "yuv420p", "-c:a", "copy", outPath); err != nil {
		return nil, fmt.Errorf("không upscale được video: %w", err)
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return nil, fmt.Errorf("FFmpeg không tạo được video đã upscale")
	}

	ctx.Log(fmt.Sprintf("VideoUpscaleNode: Đã nâng cấp video lên chuẩn %s (%s)", strings.ToUpper(resolution), filepath.Base(outPath)))

	return map[string]any{
		"video_out": outPath,
	}, nil
}

func executeConcatScenes(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	scenes := []string{}
	for _, key := range []string{"scene_1", "scene_2", "scene_3"} {
		if path := inputString(inputs, key, ""); path != "" {
			if !fileExists(path) || !isVideoFile(path) {
				return nil, fmt.Errorf("phân cảnh %s không phải video hợp lệ", key)
			}
			scenes = append(scenes, path)
		}
	}

	if len(scenes) == 0 {
		return nil, fmt.Errorf("ConcatScenesNode: Cần ít nhất một phân cảnh video để ghép nối")
	}
	if len(scenes) == 1 {
		return map[string]any{"master_video": scenes[0]}, nil
	}
	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("Concat Scenes cần FFmpeg")
	}

	outPath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_master.mp4", node.ID))
	args := []string{"-y"}
	allScenesHaveAudio := true
	for _, scene := range scenes {
		args = append(args, "-i", scene)
		hasAudio, err := probeFlowHasAudio(scene)
		if err != nil {
			return nil, fmt.Errorf("không thể kiểm tra audio của scene %s: %w", filepath.Base(scene), err)
		}
		allScenesHaveAudio = allScenesHaveAudio && hasAudio
	}
	filters := make([]string, 0, len(scenes)*2+1)
	concatInputs := make([]string, 0, len(scenes)*2)
	for i := range scenes {
		filters = append(filters, fmt.Sprintf("[%d:v:0]scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,fps=30,setsar=1,format=yuv420p[v%d]", i, i))
		concatInputs = append(concatInputs, fmt.Sprintf("[v%d]", i))
		if allScenesHaveAudio {
			filters = append(filters, fmt.Sprintf("[%d:a:0]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo[a%d]", i, i))
			concatInputs = append(concatInputs, fmt.Sprintf("[a%d]", i))
		}
	}
	audioCount := 0
	outputLabels := "[vout]"
	if allScenesHaveAudio {
		audioCount = 1
		outputLabels = "[vout][aout]"
	}
	concatFilter := strings.Join(concatInputs, "") + fmt.Sprintf("concat=n=%d:v=1:a=%d%s", len(scenes), audioCount, outputLabels)
	filters = append(filters, concatFilter)
	args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[vout]", "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
	if allScenesHaveAudio {
		args = append(args, "-map", "[aout]", "-c:a", "aac")
	}
	args = append(args, outPath)
	if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
		return nil, fmt.Errorf("không ghép được video scenes: %w", err)
	}
	if !fileExists(outPath) || !isVideoFile(outPath) {
		return nil, fmt.Errorf("FFmpeg không tạo được video scenes")
	}

	ctx.Log(fmt.Sprintf("ConcatScenesNode: Đã ghép nối %d phân cảnh thành video master -> %s", len(scenes), filepath.Base(outPath)))

	return map[string]any{
		"master_video": outPath,
	}, nil
}

func executeVirtualTryOn(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	charImg := inputString(inputs, "character_image", "")
	if resolved := resolveAssetPath(charImg, ctx.WorkDir); resolved != "" {
		charImg = resolved
	}
	if charImg == "" || !fileExists(charImg) || !isImageFile(charImg) {
		return nil, fmt.Errorf("Outfit Prompt Builder cần ảnh nhân vật hợp lệ")
	}

	garmentImg := inputString(inputs, "garment_image", "")
	if garmentImg == "" {
		garmentImg = inputString(inputs, "product_image", "")
	}
	if garmentImg == "" {
		garmentImg = inputString(inputs, "cutout_image", "")
	}
	if resolved := resolveAssetPath(garmentImg, ctx.WorkDir); resolved != "" {
		garmentImg = resolved
	}
	if garmentImg != "" && (!fileExists(garmentImg) || !isImageFile(garmentImg)) {
		return nil, fmt.Errorf("ảnh trang phục không hợp lệ")
	}

	fitStyle := nodeParamString(node.Params, "fit_style", "seamless_luxury")
	outfitName := inputString(inputs, "outfit_name", nodeParamString(node.Params, "outfit_name", "trang phục thời trang cao cấp"))

	var outfitDesc string
	switch fitStyle {
	case "sexy_dance_fit", "kpop_stage_outfit":
		outfitDesc = fmt.Sprintf("wearing %s, styled as a K-pop performance outfit with coordinated stage details", outfitName)
	case "bodycon_glamour":
		outfitDesc = fmt.Sprintf("wearing %s in a glamorous bodycon styling", outfitName)
	default:
		outfitDesc = fmt.Sprintf("wearing %s with seamless bespoke luxury tailoring", outfitName)
	}

	ingredients := []string{charImg}
	if garmentImg != "" {
		ingredients = append(ingredients, garmentImg)
	}
	ctx.Log(fmt.Sprintf("Outfit Prompt Builder: Giữ ảnh tham chiếu gốc, tạo mô tả outfit (%s)", fitStyle))
	return map[string]any{
		"fitted_image":    charImg,
		"character_image": charImg,
		"garment_image":   garmentImg,
		"ingredients":     ingredients,
		"outfit_desc":     outfitDesc,
	}, nil
}

func executeSceneEnvironment(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	locationType := nodeParamString(node.Params, "location_type", "fashion_boutique")
	customLocation := nodeParamString(node.Params, "custom_location", "")
	depthOfField := nodeParamString(node.Params, "depth_of_field", "cinematic_bokeh")
	charImg := inputString(inputs, "character_image", "")
	promptIn := inputString(inputs, "prompt_in", "")

	var envDesc string
	switch locationType {
	case "luxury_penthouse":
		envDesc = "an ultra-luxurious modern penthouse apartment with floor-to-ceiling glass panoramic windows overlooking a breathtaking modern skyline, marble flooring, sophisticated interior architecture"
	case "minimalist_studio":
		envDesc = "a sleek minimalist high-fashion studio with soft curved cyclorama background, elegant architectural arches, neutral cream and warm beige aesthetic"
	case "fashion_boutique":
		envDesc = "a prestigious Paris haute-couture fashion boutique, warm golden ambient interior accents, mirrored dressing display, luxury velvet decor"
	case "paris_runway":
		envDesc = "an iconic Paris Fashion Week indoor grand runway stage, polished glossy reflective floor, dramatic fashion editorial stage backdrop, international fashion show atmosphere"
	case "cafe_chill":
		envDesc = "a cozy chic aesthetic Seoul designer cafe, warm wooden textures, soft aesthetic indoor plants, sunlit window seats, inviting contemporary lifestyle vibe"
	case "sunset_rooftop":
		envDesc = "a glamorous open-air skyscraper rooftop terrace during sunset, panoramic skyline view with glass railing, luxury lounge aesthetic"
	case "neon_dance_studio":
		envDesc = "a professional modern urban dance rehearsal studio with full-wall reflective mirrors, polished dark hardwood dance floor, vibrant programmable LED light strips framing the space"
	case "kpop_concert_stage":
		envDesc = "a spectacular K-pop live concert main stage, giant immersive visual LED backdrop screens displaying dynamic particle motion graphics, concert arena production"
	case "cyberpunk_rooftop":
		envDesc = "a futuristic cyber city rooftop at night, glowing neon signs, wet reflective floor surfaces reflecting colorful lights, high-tech urban nightscape"
	case "sensual_club_lounge":
		envDesc = "an exclusive sensual VIP club lounge, ambient mood lighting, moody atmospheric haze, plush velvet furnishings, seductive nightlife ambiance"
	case "custom":
		if customLocation != "" {
			envDesc = customLocation
		} else {
			envDesc = "a stylish aesthetic studio environment with modern artistic backdrop"
		}
	default:
		envDesc = locationType
	}

	envPrompt := fmt.Sprintf("Setting and environment: %s, with %s camera depth of field.", envDesc, depthOfField)
	promptOut := promptIn
	if promptOut != "" {
		promptOut = fmt.Sprintf("%s, set in %s", promptOut, envDesc)
	} else {
		promptOut = envPrompt
	}

	ctx.Log(fmt.Sprintf("SceneEnvironmentNode: Đã thiết lập không gian bối cảnh '%s' (%s)", locationType, envDesc))

	outputs := map[string]any{
		"environment_desc":   envDesc,
		"environment_prompt": envPrompt,
		"prompt_out":         promptOut,
	}
	if charImg != "" {
		outputs["character_image"] = charImg
	}
	return outputs, nil
}

func executeLightingAtmosphere(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	timeOfDay := nodeParamString(node.Params, "time_of_day", "golden_hour")
	colorTemp := nodeParamString(node.Params, "color_temperature", "warm_golden")
	intensity := nodeParamString(node.Params, "light_intensity", "soft_diffused")
	promptIn := inputString(inputs, "prompt_in", "")
	environment := inputString(inputs, "environment_desc", "")

	var lightDesc string
	switch timeOfDay {
	case "golden_hour":
		lightDesc = "golden hour warm sunset lighting, soft sun flares, radiant backlight emphasizing silhouette and contours, luminous golden rim light"
	case "morning_sunlight":
		lightDesc = "fresh early morning natural sunlight streaming through, clean soft daylight illumination, pure clarity, gentle highlights"
	case "high_noon_editorial":
		lightDesc = "crisp high-fashion editorial daylight, high-key clear lighting, pristine clarity, sharp shadow contrast"
	case "blue_hour_dusk":
		lightDesc = "cinematic blue hour twilight, gentle ambient dusk hues mixed with warm interior illumination, romantic moody contrast"
	case "night_neon":
		lightDesc = "vibrant night neon illumination, dramatic dual-tone cyan and magenta neon edge highlights, striking nighttime contrast"
	case "dynamic_stage_lights":
		lightDesc = "dynamic theatrical concert stage lighting, pulsating multi-color spotlights synchronized to musical rhythm, dramatic volumetric light beams, glowing rim light"
	case "studio_three_point":
		lightDesc = "professional commercial 3-point studio softbox lighting (key light, fill light, and delicate hair/rim backlight) perfectly sculpting facial features and garment texture"
	case "midnight_luxury":
		lightDesc = "sophisticated midnight ambient lighting, moody dimmed luxury warmth, subtle accent illumination"
	default:
		lightDesc = timeOfDay
	}

	lightingPrompt := fmt.Sprintf("Lighting and atmosphere: %s, %s color temperature, %s illumination. Environment: %s.", lightDesc, colorTemp, intensity, environment)
	promptOut := promptIn
	if promptOut != "" {
		promptOut = fmt.Sprintf("%s, illuminated by %s, in %s", promptOut, lightDesc, environment)
	} else {
		promptOut = lightingPrompt
	}

	ctx.Log(fmt.Sprintf("LightingAtmosphereNode: Đã thiết lập ánh sáng & thời gian '%s' (%s)", timeOfDay, lightDesc))

	return map[string]any{
		"lighting_desc":   lightDesc,
		"lighting_prompt": lightingPrompt,
		"prompt_out":      promptOut,
	}, nil
}

func executeCameraAnglesDirector(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	subjectImg := inputString(inputs, "subject_image", "")
	if subjectImg == "" {
		subjectImg = inputString(inputs, "character_image", "")
	}
	if resolved := resolveAssetPath(subjectImg, ctx.WorkDir); resolved != "" {
		subjectImg = resolved
	}
	if subjectImg == "" || !fileExists(subjectImg) || !isImageFile(subjectImg) {
		return nil, fmt.Errorf("Camera Angles Director cần ảnh nhân vật tham chiếu hợp lệ")
	}

	productTitle := inputString(inputs, "product_title", "")
	if productTitle == "" {
		productTitle = nodeParamString(node.Params, "product_title", "trang phục/sản phẩm cao cấp")
	}

	charName := inputString(inputs, "character_name", "")
	if charName == "" {
		charName = nodeParamString(node.Params, "model_name", "nhân vật đại diện")
	}

	environment := inputString(inputs, "environment_desc", "")
	lighting := inputString(inputs, "lighting_desc", "")
	shotStyle := nodeParamString(node.Params, "shot_style", "high_fashion_editorial")
	fullPrompt := fmt.Sprintf("%s style. Cinematic 9:16 full body runway shot with the character (%s) walking confidently and presenting %s. Keep visible character and garment details. Environment: %s. Lighting: %s. Photorealistic, steady camera movement.", shotStyle, charName, productTitle, environment, lighting)
	mediumPrompt := fmt.Sprintf("%s style. Cinematic 9:16 medium shot of the character (%s) turning naturally to present %s. Keep visible character and garment details. Environment: %s. Lighting: %s. 35mm lens.", shotStyle, charName, productTitle, environment, lighting)
	detailPrompt := fmt.Sprintf("%s style. Cinematic 9:16 detail shot showing the visible material and construction of %s. Environment: %s. Lighting: %s. Keep design details consistent with reference images.", shotStyle, productTitle, environment, lighting)

	ctx.Log(fmt.Sprintf("CameraAnglesDirector: Đã thiết lập đạo diễn 3 góc quay điện ảnh bảo toàn nhận diện (%s & %s)", charName, productTitle))

	return map[string]any{
		"angle_full":         fullPrompt,
		"angle_medium":       mediumPrompt,
		"angle_detail":       detailPrompt,
		"storyboard_prompts": []string{fullPrompt, mediumPrompt, detailPrompt},
		"subject_image":      subjectImg,
	}, nil
}

func copyFile(src, dst string) error {
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func flowStringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, item := range values {
			if path, ok := item.(string); ok && path != "" {
				result = append(result, path)
			}
		}
		return result
	case string:
		if values != "" {
			return []string{values}
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// executePreviewMedia thực thi node xem trước kết quả trực quan (tương tự Preview Image trong ComfyUI)
func executePreviewMedia(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	var mediaPath string
	// Quét các cổng phổ biến truyền vào
	for _, k := range []string{"media_in", "image", "images", "video", "media", "cutout_image", "mask_image", "source_media", "product_image", "character_ref", "warped_garment", "rendered_video", "graded_video", "master_video"} {
		if v, ok := inputs[k]; ok {
			if s, ok := v.(string); ok && s != "" && fileExists(s) {
				mediaPath = s
				break
			}
		}
	}
	if mediaPath == "" {
		for _, v := range inputs {
			if s, ok := v.(string); ok && s != "" && fileExists(s) {
				ext := strings.ToLower(filepath.Ext(s))
				if ext == ".mp4" || ext == ".webm" || ext == ".mov" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
					mediaPath = s
					break
				}
			}
		}
	}

	if mediaPath == "" {
		return nil, fmt.Errorf("Preview cần file ảnh hoặc video đầu vào")
	}

	destMedia := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_%s", node.ID, filepath.Base(mediaPath)))
	if destMedia != mediaPath {
		if err := copyFile(mediaPath, destMedia); err != nil {
			return nil, fmt.Errorf("không thể chuẩn bị file preview: %w", err)
		}
	}

	ctx.Log(fmt.Sprintf("PreviewMediaNode: Đang hiển thị kết quả xem trước cho '%s'", filepath.Base(destMedia)))

	outputs := map[string]any{
		"media_out": destMedia,
		"preview":   destMedia,
	}
	if isImageFile(destMedia) {
		outputs["image"] = destMedia
		outputs["images"] = destMedia
	} else if isVideoFile(destMedia) {
		outputs["video"] = destMedia
	} else {
		return nil, fmt.Errorf("Preview không hỗ trợ media dạng %s", filepath.Ext(destMedia))
	}
	return outputs, nil
}

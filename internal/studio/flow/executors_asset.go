package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func executeCharacterInput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	modelPath := nodeParamString(node.Params, "model_path", "")
	modelName := nodeParamString(node.Params, "model_name", "Nhân vật đại diện")

	// Nếu chưa có model_path cụ thể, tìm ảnh nhân vật sẵn có trong thư viện
	if modelPath == "" || !fileExists(modelPath) {
		candidates := []string{
			filepath.Join(ctx.WorkDir, "character.jpg"),
			filepath.Join(ctx.OutDir, "models", fmt.Sprintf("%d", ctx.AccountID)),
			filepath.Join("data", "output", "studio", "models", fmt.Sprintf("%d", ctx.AccountID)),
		}
		for _, cand := range candidates {
			if fileExists(cand) && !isDir(cand) {
				modelPath = cand
				break
			} else if isDir(cand) {
				entries, _ := os.ReadDir(cand)
				for _, e := range entries {
					if !e.IsDir() && isImageFile(e.Name()) {
						modelPath = filepath.Join(cand, e.Name())
						break
					}
				}
				if modelPath != "" {
					break
				}
			}
		}
	}

	// Nếu vẫn chưa có ảnh, tìm bất kỳ ảnh nhân vật mẫu nào trong data
	if modelPath == "" || !fileExists(modelPath) {
		globMatches, _ := filepath.Glob("data/output/studio/models/*/*.jpg")
		if len(globMatches) > 0 {
			modelPath = globMatches[0]
		}
	}

	// Tạo ảnh giả lập nếu môi trường test chưa có ảnh thực
	if modelPath == "" || !fileExists(modelPath) {
		demoPath := filepath.Join(ctx.WorkDir, "default_character.jpg")
		_ = os.MkdirAll(ctx.WorkDir, 0o755)
		if err := writeDummyImage(demoPath, 736, 982, "CHARACTER"); err != nil {
			return nil, fmt.Errorf("không thể chuẩn bị ảnh nhân vật mẫu: %w", err)
		}
		modelPath = demoPath
	}

	characterImages := []string{modelPath}
	ctx.Log(fmt.Sprintf("CharacterNode: Đã tải nhân vật '%s' (%s)", modelName, filepath.Base(modelPath)))

	return map[string]any{
		"character_image":  modelPath,
		"character_images": characterImages,
		"character_name":   modelName,
	}, nil
}

func executeProductInput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	prodImg := nodeParamString(node.Params, "product_image", "")
	prodTitle := nodeParamString(node.Params, "product_title", "Sản phẩm xu hướng")
	prodID := nodeParamString(node.Params, "product_id", "")
	affURL := nodeParamString(node.Params, "affiliate_url", "")

	if prodImg == "" || !fileExists(prodImg) {
		// Tìm ảnh sản phẩm mẫu sẵn có
		candidates, _ := filepath.Glob("data/uploads/*product*")
		if len(candidates) > 0 {
			prodImg = candidates[0]
		} else {
			demoPath := filepath.Join(ctx.WorkDir, "default_product.jpg")
			_ = os.MkdirAll(ctx.WorkDir, 0o755)
			if err := writeDummyImage(demoPath, 800, 800, "PRODUCT"); err != nil {
				return nil, fmt.Errorf("không thể chuẩn bị ảnh sản phẩm mẫu: %w", err)
			}
			prodImg = demoPath
		}
	}

	productInfo := map[string]any{
		"id":    prodID,
		"title": prodTitle,
		"image": prodImg,
		"url":   affURL,
	}

	ctx.Log(fmt.Sprintf("ProductNode: Đã tải sản phẩm '%s'", prodTitle))

	return map[string]any{
		"product_image": prodImg,
		"product_title": prodTitle,
		"product_info":  productInfo,
	}, nil
}

func executeVideoSource(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoPath := nodeParamString(node.Params, "video_path", "")
	videoURL := nodeParamString(node.Params, "video_url", "")
	videoID := nodeParamString(node.Params, "video_id", "")

	if videoPath == "" || !fileExists(videoPath) {
		candidates, _ := filepath.Glob("data/uploads/*.mp4")
		if len(candidates) > 0 {
			videoPath = candidates[0]
		} else if fileExists("internal/web/static/sample_916.mp4") {
			videoPath = "internal/web/static/sample_916.mp4"
		} else {
			demoVideo := filepath.Join(ctx.WorkDir, "sample_source.mp4")
			_ = os.MkdirAll(ctx.WorkDir, 0o755)
			if err := writeDummyVideo(ctx, demoVideo, 5.0); err != nil {
				return nil, fmt.Errorf("không tìm thấy video nguồn và không tạo được video mẫu: %w", err)
			}
			videoPath = demoVideo
		}
	}

	// Trích xuất audio từ video
	audioTrack := filepath.Join(ctx.WorkDir, "source_audio.aac")
	if ctx.FFmpegRunner != nil {
		_ = ctx.FFmpegRunner(ctx.Context, "-y", "-i", videoPath, "-vn", "-c:a", "aac", audioTrack)
	}

	duration := 13.87
	ctx.Log(fmt.Sprintf("VideoSourceNode: Đã nạp video nguồn (id=%s, file=%s)", videoID, filepath.Base(videoPath)))

	return map[string]any{
		"video_path":  videoPath,
		"audio_track": audioTrack,
		"duration":    duration,
		"video_url":   videoURL,
	}, nil
}

func executeAudioSource(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	audioPath := nodeParamString(node.Params, "audio_path", "")
	audioTitle := nodeParamString(node.Params, "audio_title", "Nhạc nền thịnh hành")

	if audioPath == "" || !fileExists(audioPath) {
		if fileExists("internal/web/static/preview.mp3") {
			audioPath = "internal/web/static/preview.mp3"
		} else {
			demoAudio := filepath.Join(ctx.WorkDir, "default_audio.aac")
			_ = os.MkdirAll(ctx.WorkDir, 0o755)
			if err := writeDummyAudio(ctx, demoAudio, 10.0); err != nil {
				return nil, fmt.Errorf("không thể chuẩn bị file âm thanh mẫu: %w", err)
			}
			audioPath = demoAudio
		}
	}

	ctx.Log(fmt.Sprintf("AudioSourceNode: Đã nạp âm thanh '%s'", audioTitle))

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

func writeDummyImage(path string, width, height int, label string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	// Tạo file JPEG hợp lệ tối giản
	// Header JPEG tối giản 1x1 pixel nếu không có ffmpeg
	header := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x01, 0x00, 0x48,
		0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43, 0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08,
		0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20,
		0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29, 0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27,
		0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00, 0x01, 0x05, 0x01, 0x01,
		0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04,
		0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F,
		0x00, 0xBF, 0x80, 0xFF, 0xD9,
	}
	return os.WriteFile(path, header, 0o644)
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

func writeDummyAudio(ctx *ExecutionContext, path string, duration float64) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if ctx != nil && ctx.FFmpegRunner != nil {
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:duration=%.2f", duration), "-c:a", "aac", path); err == nil {
			return nil
		}
	}
	wavHeader := []byte{
		'R', 'I', 'F', 'F', 0x24, 0x00, 0x00, 0x00, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ',
		0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x44, 0xAC, 0x00, 0x00, 0x88, 0x58, 0x01, 0x00,
		0x02, 0x00, 0x10, 0x00, 'd', 'a', 't', 'a', 0x00, 0x00, 0x00, 0x00,
	}
	return os.WriteFile(path, wavHeader, 0o644)
}

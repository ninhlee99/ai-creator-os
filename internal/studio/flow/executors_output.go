package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func executeVideoOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	title := inputString(inputs, "title", nodeParamString(node.Params, "title", "Video hoàn thiện"))

	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("VideoOutputNode: Không có file video đầu vào hợp lệ")
	}

	if err := os.MkdirAll(ctx.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục output: %w", err)
	}
	finalFilename := fmt.Sprintf("flow_video_%s%s", ctx.RunID, filepath.Ext(videoIn))
	finalPath := filepath.Join(ctx.OutDir, finalFilename)

	if err := copyFile(videoIn, finalPath); err != nil {
		return nil, fmt.Errorf("không thể xuất video: %w", err)
	}

	ctx.Log(fmt.Sprintf("VideoOutputNode: Video cuối cùng đã sẵn sàng tại %s (%s)", filepath.Base(finalPath), title))

	return map[string]any{
		"final_video": finalPath,
	}, nil
}

func executeImageOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	imgIn := inputString(inputs, "image_in", "")
	if imgIn == "" || !fileExists(imgIn) || !isImageFile(imgIn) {
		return nil, fmt.Errorf("ImageOutputNode: Không có file ảnh đầu vào hợp lệ")
	}
	format := strings.ToLower(nodeParamString(node.Params, "format", strings.TrimPrefix(filepath.Ext(imgIn), ".")))
	if format == "jpeg" {
		format = "jpg"
	}
	switch format {
	case "jpg", "png", "webp":
	default:
		return nil, fmt.Errorf("định dạng ảnh chưa hỗ trợ: %s", format)
	}

	if err := os.MkdirAll(ctx.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục output: %w", err)
	}
	finalFilename := fmt.Sprintf("flow_image_%s.%s", ctx.RunID, format)
	finalPath := filepath.Join(ctx.OutDir, finalFilename)

	if strings.EqualFold(filepath.Ext(imgIn), "."+format) {
		if err := copyFile(imgIn, finalPath); err != nil {
			return nil, fmt.Errorf("không thể xuất ảnh: %w", err)
		}
	} else {
		if ctx.FFmpegRunner == nil {
			return nil, fmt.Errorf("đổi định dạng ảnh cần FFmpeg")
		}
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-i", imgIn, "-frames:v", "1", finalPath); err != nil {
			return nil, fmt.Errorf("không đổi được ảnh sang %s: %w", format, err)
		}
		if !fileExists(finalPath) || !isImageFile(finalPath) {
			return nil, fmt.Errorf("FFmpeg không tạo được ảnh đầu ra hợp lệ")
		}
	}

	ctx.Log(fmt.Sprintf("ImageOutputNode: Ảnh hoàn thiện đã sẵn sàng tại %s", filepath.Base(finalPath)))

	return map[string]any{
		"final_image": finalPath,
	}, nil
}

func executeAudioOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	audioIn := inputString(inputs, "audio_in", "")
	if audioIn == "" || !fileExists(audioIn) || !isAudioFile(audioIn) {
		return nil, fmt.Errorf("AudioOutputNode: Không có file audio đầu vào hợp lệ")
	}
	if err := os.MkdirAll(ctx.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục output: %w", err)
	}
	finalPath := filepath.Join(ctx.OutDir, fmt.Sprintf("flow_audio_%s%s", ctx.RunID, filepath.Ext(audioIn)))
	if err := copyFile(audioIn, finalPath); err != nil {
		return nil, fmt.Errorf("không thể xuất audio: %w", err)
	}
	ctx.Log(fmt.Sprintf("AudioOutputNode: Audio cuối cùng đã sẵn sàng tại %s", filepath.Base(finalPath)))
	return map[string]any{"final_audio": finalPath}, nil
}

package flow

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var flowVideoToolboxH264 = sync.OnceValue(func() bool {
	encoders, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(encoders, []byte("h264_videotoolbox")) {
		return false
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:r=30:d=1",
		"-frames:v", "2", "-c:v", "h264_videotoolbox", "-f", "null", "-")
	return cmd.Run() == nil
})

func flowH264EncodeArgs(fastIntermediate bool) []string {
	if flowVideoToolboxH264() {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "16M", "-maxrate", "22M"}
	}
	if fastIntermediate {
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "16"}
	}
	return []string{"-c:v", "libx264", "-preset", "medium", "-crf", "18"}
}

func executeMuseAINeuralRender(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	promptText := inputString(inputs, "prompt", "")
	if promptText == "" {
		return nil, fmt.Errorf("Muse AI video node cần prompt")
	}
	masterPrompt := strings.TrimSpace(promptText)
	refFiles := make([]string, 0, 2)
	for _, key := range []string{"character_ref", "warped_garment"} {
		path := inputString(inputs, key, "")
		if path == "" {
			continue
		}
		if !fileExists(path) {
			return nil, fmt.Errorf("ảnh tham chiếu Muse AI không tồn tại: %s", key)
		}
		refFiles = append(refFiles, path)
	}
	if len(refFiles) == 0 {
		return nil, fmt.Errorf("Muse AI video node cần ít nhất một ảnh tham chiếu thật")
	}

	outVideo := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_neural_render.mp4", node.ID))
	sampleFrame := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_sample_frame.jpg", node.ID))

	threadTitle := fmt.Sprintf("Flow [%s: %s]", node.ID, node.Title)
	ctx.Log(fmt.Sprintf("Muse AI: gửi prompt tạo video (%d ảnh tham chiếu)", len(refFiles)))
	if err := renderMuseWorkflowNode(ctx, threadTitle, masterPrompt, refFiles, outVideo); err != nil {
		return nil, err
	}
	if !fileExists(outVideo) || !isVideoFile(outVideo) {
		return nil, fmt.Errorf("Muse AI chưa tạo file video đầu ra hợp lệ")
	}

	// Trích xuất 1 frame mẫu cho Preview
	if ctx.FFmpegRunner != nil {
		if err := ctx.FFmpegRunner(ctx.Context, "-y", "-ss", "00:00:01", "-i", outVideo, "-vframes", "1", "-q:v", "2", sampleFrame); err != nil || !fileExists(sampleFrame) {
			sampleFrame = ""
		}
	} else {
		sampleFrame = ""
	}

	ctx.Log(fmt.Sprintf("Muse AI đã tạo video thật -> %s", filepath.Base(outVideo)))
	outputs := map[string]any{
		"rendered_video": outVideo,
		"master_video":   outVideo,
	}
	if sampleFrame != "" {
		outputs["sample_frame"] = sampleFrame
	}
	return outputs, nil
}

func executeCinematicColorGrading(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" {
		videoIn = inputString(inputs, "rendered_video", "")
	}
	if videoIn == "" {
		videoIn = inputString(inputs, "media_in", "")
	}

	if videoIn == "" {
		videoIn = inputString(inputs, "video_out", "")
	}
	contrast := nodeParamFloat(node.Params, "contrast_boost", 1.12)
	saturation := nodeParamFloat(node.Params, "saturation", 1.05)
	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("cân chỉnh màu cần file video đầu vào hợp lệ")
	}

	gradedOut := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_adjusted.mp4", node.ID))
	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("Cinematic Color Grading cần FFmpeg")
	}

	eqFilter := fmt.Sprintf("eq=contrast=%.2f:saturation=%.2f:brightness=0.01", contrast, saturation)
	args := []string{"-y", "-i", videoIn, "-vf", eqFilter}
	args = append(args, flowH264EncodeArgs(true)...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", gradedOut)
	if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
		return nil, fmt.Errorf("FFmpeg cân chỉnh màu thất bại: %w", err)
	}
	if !fileExists(gradedOut) || !isVideoFile(gradedOut) {
		return nil, fmt.Errorf("FFmpeg không tạo được video đã cân chỉnh")
	}

	ctx.Log(fmt.Sprintf("Đã cân chỉnh tương phản %.2f và bão hòa %.2f", contrast, saturation))

	return map[string]any{
		"graded_video": gradedOut,
		"video_out":    gradedOut,
	}, nil
}

// executeVideoMasterOutput transcodes a supplied video to requested dimensions and frame rate.
func executeVideoMasterOutput(ctx *ExecutionContext, node *FlowNode, inputs map[string]any) (map[string]any, error) {
	videoIn := inputString(inputs, "video_in", "")
	if videoIn == "" {
		videoIn = inputString(inputs, "graded_video", "")
	}
	if videoIn == "" {
		videoIn = inputString(inputs, "rendered_video", "")
	}

	audioTrack := inputString(inputs, "audio_track", "")
	title := inputString(inputs, "title", nodeParamString(node.Params, "title", "Video Master"))

	res := nodeParamString(node.Params, "target_resolution", "1080x1920")
	fps := nodeParamInt(node.Params, "fps", 24)

	masterOut := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s_master_export.mp4", node.ID))

	if videoIn == "" || !fileExists(videoIn) || !isVideoFile(videoIn) {
		return nil, fmt.Errorf("xuất master cần file video đầu vào hợp lệ")
	}
	if audioTrack != "" && !fileExists(audioTrack) {
		return nil, fmt.Errorf("track âm thanh không tồn tại")
	}
	if ctx.FFmpegRunner == nil {
		return nil, fmt.Errorf("xuất master cần FFmpeg; không có file đầu ra nào được tạo")
	}
	width, height, ok := parseMasterResolution(res)
	if !ok {
		return nil, fmt.Errorf("độ phân giải master không hợp lệ: %q", res)
	}
	if fps < 1 || fps > 120 {
		return nil, fmt.Errorf("FPS master phải từ 1 đến 120")
	}

	filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,fps=%d", width, height, width, height, fps)
	args := []string{"-y", "-i", videoIn}
	if audioTrack != "" {
		args = append(args, "-i", audioTrack)
	}
	args = append(args, "-map", "0:v:0")
	if audioTrack != "" {
		args = append(args, "-map", "1:a:0", "-shortest")
	} else {
		args = append(args, "-map", "0:a?")
	}
	args = append(args, "-vf", filter)
	args = append(args, flowH264EncodeArgs(false)...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", masterOut)
	if err := ctx.FFmpegRunner(ctx.Context, args...); err != nil {
		return nil, fmt.Errorf("FFmpeg không thể xuất video master: %w", err)
	}
	if !fileExists(masterOut) {
		return nil, fmt.Errorf("FFmpeg hoàn tất nhưng không tạo được video master")
	}

	ctx.Log(fmt.Sprintf("VideoMasterOutputNode: Đã hoàn tất xuất bản Video Master [%s, %d fps] -> %s", res, fps, filepath.Base(masterOut)))

	return map[string]any{
		"master_video": masterOut,
		"final_export": masterOut,
		"title":        title,
	}, nil
}

func parseMasterResolution(raw string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errWidth := strconv.Atoi(parts[0])
	height, errHeight := strconv.Atoi(parts[1])
	if errWidth != nil || errHeight != nil || width < 2 || height < 2 || width > 8192 || height > 8192 {
		return 0, 0, false
	}
	return width, height, true
}

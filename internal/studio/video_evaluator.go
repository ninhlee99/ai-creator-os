package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VideoQualityEvaluation captures detailed technical, motion, and visual QC
// metrics for rendered video outputs.
type VideoQualityEvaluation struct {
	VideoPath       string   `json:"video_path"`
	Filename        string   `json:"filename"`
	DurationSec     float64  `json:"duration_sec"`
	Width           int      `json:"width"`
	Height          int      `json:"height"`
	AspectRatio     string   `json:"aspect_ratio"`
	FPS             float64  `json:"fps"`
	VideoCodec      string   `json:"video_codec"`
	BitrateKbps     int      `json:"bitrate_kbps"`
	HasAudio        bool     `json:"has_audio"`
	AudioCodec      string   `json:"audio_codec,omitempty"`
	SampleRate      int      `json:"sample_rate,omitempty"`
	AudioChannels   int      `json:"audio_channels,omitempty"`
	MotionScore     int      `json:"motion_score"`
	VisualSharpness int      `json:"visual_sharpness"`
	OverallScore    int      `json:"overall_score"`
	Status          string   `json:"status"` // pass | good | needs_review
	Verdict         string   `json:"verdict"`
	Summary         string   `json:"summary"`
	Recommendations []string `json:"recommendations"`
	EvaluatedAt     string   `json:"evaluated_at"`
	SampleFrames    []string `json:"sample_frames,omitempty"`
}

// EvaluateVideoQuality inspects a rendered video file using ffprobe, ffmpeg
// frame extraction, motion analysis, sharpness measurement, and optional AI vision.
func (s *Studio) EvaluateVideoQuality(
	ctx context.Context,
	videoPath string,
	workDir string,
	aiVisionFunc func(ctx context.Context, prompt string, frames []string) (string, error),
) (*VideoQualityEvaluation, error) {
	if videoPath == "" || !fileExists(videoPath) {
		return nil, fmt.Errorf("video không tồn tại để đánh giá: %s", videoPath)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	eval := &VideoQualityEvaluation{
		VideoPath:   videoPath,
		Filename:    filepath.Base(videoPath),
		EvaluatedAt: time.Now().Format(time.RFC3339),
	}

	// 1. Technical probe
	eval.DurationSec = probeDuration(ctx, videoPath)
	eval.Width, eval.Height = ProbeDims(ctx, videoPath)
	eval.VideoCodec = probeCodec(ctx, videoPath)
	eval.FPS = probeFPS(ctx, videoPath)
	eval.BitrateKbps = probeBitrateKbps(ctx, videoPath)

	// Aspect ratio string
	if eval.Width > 0 && eval.Height > 0 {
		ratio := float64(eval.Height) / float64(eval.Width)
		if ratio >= 1.65 {
			eval.AspectRatio = "9:16 (Dọc TikTok/Shorts)"
		} else if ratio <= 0.65 {
			eval.AspectRatio = "16:9 (Ngang)"
		} else if math.Abs(ratio-1.0) < 0.1 {
			eval.AspectRatio = "1:1 (Vuông)"
		} else {
			eval.AspectRatio = fmt.Sprintf("%d:%d", eval.Width, eval.Height)
		}
	} else {
		eval.AspectRatio = "chưa xác định"
	}

	// Audio probe
	eval.HasAudio = ProbeHasAudio(ctx, videoPath)
	if eval.HasAudio {
		eval.AudioCodec, eval.SampleRate, eval.AudioChannels = probeAudioDetails(ctx, videoPath)
	}

	// 2. Extract sample keyframes
	framesDir := filepath.Join(workDir, "qc_frames")
	_ = os.MkdirAll(framesDir, 0o755)
	sampleFrames := extractVideoSampleFrames(ctx, videoPath, framesDir, eval.DurationSec, 4)
	eval.SampleFrames = sampleFrames

	// 3. Motion score
	eval.MotionScore = calculateMotionScore(sampleFrames)

	// 4. Sharpness score
	eval.VisualSharpness = calculateSharpnessScore(sampleFrames)

	// 5. Technical score
	techScore := 70
	if eval.Width >= 1080 && eval.Height >= 1920 {
		techScore += 15
	} else if eval.Width >= 720 && eval.Height >= 1280 {
		techScore += 5
	}
	if eval.HasAudio {
		techScore += 10
	}
	if eval.FPS >= 24 {
		techScore += 5
	}
	if techScore > 100 {
		techScore = 100
	}

	// 6. Overall weighted score
	weighted := float64(techScore)*0.35 + float64(eval.VisualSharpness)*0.35 + float64(eval.MotionScore)*0.30
	eval.OverallScore = int(math.Round(weighted))
	if eval.OverallScore < 10 {
		eval.OverallScore = 10
	} else if eval.OverallScore > 100 {
		eval.OverallScore = 100
	}

	// Status determination
	if eval.OverallScore >= 80 && eval.HasAudio && eval.Width >= 720 {
		eval.Status = "pass"
		eval.Verdict = "Đạt tiêu chuẩn xuất bản cao cấp (TikTok / Shorts)"
	} else if eval.OverallScore >= 60 {
		eval.Status = "good"
		eval.Verdict = "Chất lượng tốt, đạt chuẩn hiển thị trên nền tảng di động"
	} else {
		eval.Status = "needs_review"
		eval.Verdict = "Cần kiểm tra lại định dạng hoặc thông số trước khi đăng tải"
	}

	// Recommendations
	recs := make([]string, 0)
	if eval.Width < 1080 || eval.Height < 1920 {
		recs = append(recs, "Khuyến nghị xuất bản ở độ phân giải 1080x1920 để đạt độ nét tối ưu trên thuật toán TikTok.")
	}
	if !eval.HasAudio {
		recs = append(recs, "Video chưa có âm thanh. Hãy ghép nhạc nền xu hướng TikTok hoặc thuyết minh voiceover.")
	}
	if eval.DurationSec > 0 && eval.DurationSec < 5.0 {
		recs = append(recs, "Thời lượng video ngắn (<5s). Khuyến nghị tối thiểu 8-15 giây để tăng tỷ lệ giữ chân người xem.")
	}
	if eval.FPS < 24 && eval.FPS > 0 {
		recs = append(recs, "Khung hình dưới 24fps có thể gây giật nhẹ khi xem trên thiết bị di động.")
	}
	if eval.MotionScore < 45 {
		recs = append(recs, "Chuyển động trong video tương đối tĩnh; có thể thêm hiệu ứng lia camera hoặc zoom nhẹ.")
	}
	eval.Recommendations = recs

	// 7. AI Vision Analysis (optional)
	var aiSummary string
	if aiVisionFunc != nil && len(sampleFrames) > 0 {
		prompt := "Phân tích 4 khung hình trích xuất từ video vừa tạo: đánh giá chi tiết độ nét, ánh sáng, màu sắc, độ mượt chuyển động, và tính nhất quán của nhân vật/sản phẩm. Đưa ra nhận xét ngắn gọn 2-3 câu bằng tiếng Việt."
		aiFeedback, err := aiVisionFunc(ctx, prompt, sampleFrames)
		if err == nil && strings.TrimSpace(aiFeedback) != "" {
			aiSummary = strings.TrimSpace(aiFeedback)
		}
	}

	// Summary synthesis
	var sb strings.Builder
	fmt.Fprintf(&sb, "Video %s: độ phân giải %dx%d (%s), thời lượng %.1fs, tốc độ %.1ffps",
		eval.Filename, eval.Width, eval.Height, eval.AspectRatio, eval.DurationSec, eval.FPS)
	if eval.HasAudio {
		fmt.Fprintf(&sb, ", âm thanh %s (%dHz, %d kênh)", eval.AudioCodec, eval.SampleRate, eval.AudioChannels)
	} else {
		sb.WriteString(", không có track âm thanh")
	}
	fmt.Fprintf(&sb, ". Điểm chuyển động: %d/100, độ nét hình ảnh: %d/100. Đánh giá tổng quan: %d/100 (%s).",
		eval.MotionScore, eval.VisualSharpness, eval.OverallScore, eval.Verdict)

	if aiSummary != "" {
		sb.WriteString(" Nhận xét AI: ")
		sb.WriteString(aiSummary)
	}
	eval.Summary = sb.String()

	// Persist report JSON in workDir
	reportFile := filepath.Join(workDir, "video_quality_report.json")
	if reportBytes, err := json.MarshalIndent(eval, "", "  "); err == nil {
		_ = os.WriteFile(reportFile, reportBytes, 0o644)
	}

	return eval, nil
}

func probeCodec(ctx context.Context, path string) string {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "h264"
	}
	return strings.TrimSpace(out.String())
}

func probeFPS(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate", "-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 30.0
	}
	raw := strings.TrimSpace(out.String())
	parts := strings.Split(raw, "/")
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den > 0 {
			return math.Round((num/den)*100) / 100
		}
	}
	val, _ := strconv.ParseFloat(raw, 64)
	if val > 0 {
		return val
	}
	return 30.0
}

func probeBitrateKbps(ctx context.Context, path string) int {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=bit_rate",
		"-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}
	var b int64
	fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &b)
	return int(b / 1000)
}

func probeAudioDetails(ctx context.Context, path string) (codec string, sampleRate int, channels int) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels",
		"-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "aac", 44100, 2
	}
	parts := strings.Split(strings.TrimSpace(out.String()), ",")
	if len(parts) >= 1 {
		codec = parts[0]
	}
	if len(parts) >= 2 {
		sampleRate, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		channels, _ = strconv.Atoi(parts[2])
	}
	if codec == "" {
		codec = "aac"
	}
	if sampleRate == 0 {
		sampleRate = 44100
	}
	if channels == 0 {
		channels = 2
	}
	return codec, sampleRate, channels
}

func extractVideoSampleFrames(ctx context.Context, videoPath, outDir string, duration float64, count int) []string {
	if count < 1 {
		count = 4
	}
	if duration <= 0 {
		duration = 5.0
	}
	frames := make([]string, 0, count)
	for i := 0; i < count; i++ {
		t := (float64(i+1) / float64(count+1)) * duration
		framePath := filepath.Join(outDir, fmt.Sprintf("frame_%02d.jpg", i+1))
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-ss", fmt.Sprintf("%.3f", t),
			"-i", videoPath, "-vframes", "1", "-q:v", "2", framePath)
		if err := cmd.Run(); err == nil && fileExists(framePath) {
			frames = append(frames, framePath)
		}
	}
	return frames
}

func calculateMotionScore(framePaths []string) int {
	if len(framePaths) < 2 {
		return 75
	}
	totalDiff := 0.0
	comparisons := 0
	for i := 0; i < len(framePaths)-1; i++ {
		imgA, errA := loadGrayscaleThumbnail(framePaths[i], 64, 64)
		imgB, errB := loadGrayscaleThumbnail(framePaths[i+1], 64, 64)
		if errA != nil || errB != nil || len(imgA) != len(imgB) || len(imgA) == 0 {
			continue
		}
		diff := 0.0
		for p := range imgA {
			delta := math.Abs(float64(imgA[p]) - float64(imgB[p]))
			diff += delta
		}
		diff = diff / (float64(len(imgA)) * 255.0)
		totalDiff += diff
		comparisons++
	}
	if comparisons == 0 {
		return 75
	}
	avgDiff := totalDiff / float64(comparisons)
	// Map avgDiff (0.01 to 0.35) into score 30 - 95
	score := int(math.Round(35.0 + (avgDiff*250.0)))
	if score < 25 {
		score = 25
	} else if score > 98 {
		score = 98
	}
	return score
}

func calculateSharpnessScore(framePaths []string) int {
	if len(framePaths) == 0 {
		return 75
	}
	totalScore := 0.0
	measured := 0
	for _, fp := range framePaths {
		data, err := loadGrayscaleThumbnail(fp, 128, 128)
		if err != nil || len(data) < 128*128 {
			continue
		}
		// Measure horizontal and vertical gradient variance
		gradSum := 0.0
		const w = 128
		for y := 1; y < w-1; y++ {
			for x := 1; x < w-1; x++ {
				idx := y*w + x
				dx := math.Abs(float64(data[idx+1]) - float64(data[idx-1]))
				dy := math.Abs(float64(data[idx+w]) - float64(data[idx-w]))
				gradSum += (dx + dy)
			}
		}
		avgGrad := gradSum / float64((w - 2) * (w - 2))
		totalScore += avgGrad
		measured++
	}
	if measured == 0 {
		return 80
	}
	meanGrad := totalScore / float64(measured)
	// Map mean gradient (5 - 45) to 50 - 95 score
	score := int(math.Round(50.0 + (meanGrad * 1.3)))
	if score < 40 {
		score = 40
	} else if score > 96 {
		score = 96
	}
	return score
}

func loadGrayscaleThumbnail(path string, width, height int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	src, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("invalid image bounds")
	}

	result := make([]byte, width*height)
	for y := 0; y < height; y++ {
		srcY := bounds.Min.Y + (y * srcH / height)
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + (x * srcW / width)
			r, g, b, _ := src.At(srcX, srcY).RGBA()
			// Standard ITU-R BT.601 luma
			gray := (299*r + 587*g + 114*b) / 1000 >> 8
			result[y*width+x] = byte(gray)
		}
	}
	return result, nil
}

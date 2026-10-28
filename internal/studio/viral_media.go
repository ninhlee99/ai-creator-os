package studio

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func ValidateTikTokSourceFile(path string) error {
	return validateTikTokSourceFileSize(path, MaxTikTokSourceBytes)
}

func validateTikTokSourceFileSize(path string, maxBytes int64) error {
	if !isViralVideoFile(path) {
		return fmt.Errorf("file không phải video hợp lệ")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("không đọc được file video: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("file video rỗng hoặc không phải file thường")
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("file video vượt giới hạn %.0f MiB", float64(maxBytes)/(1<<20))
	}
	return nil
}

func splitViralSourceForEditing(ctx context.Context, source string, duration float64, workDir string) ([]viralEditSegment, error) {
	if duration < 3 {
		return nil, fmt.Errorf("video nguồn phải dài ít nhất 3s để chia và render bằng Muse AI")
	}
	const maxEditSeconds = 10.0
	count := int(math.Ceil(duration / maxEditSeconds))
	if count < 1 {
		count = 1
	}
	segmentSeconds := duration / float64(count)
	if segmentSeconds < 3 || segmentSeconds > maxEditSeconds {
		return nil, fmt.Errorf("không chia được video %.2fs thành đoạn edit 3-10s", duration)
	}
	segments := make([]viralEditSegment, 0, count)
	for i := 0; i < count; i++ {
		start := float64(i) * segmentSeconds
		segmentDuration := segmentSeconds
		if i == count-1 {
			segmentDuration = duration - start
		}
		path := filepath.Join(workDir, fmt.Sprintf("source-segment-%02d.mp4", i+1))
		if err := ffmpegRun(ctx, "-y", "-ss", fmt.Sprintf("%.3f", start), "-i", source,
			"-t", fmt.Sprintf("%.3f", segmentDuration), "-map", "0:v:0", "-map", "0:a?",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "160k", "-avoid_negative_ts", "make_zero", path); err != nil {
			return nil, fmt.Errorf("tách đoạn nguồn %d/%d: %w", i+1, count, err)
		}
		segments = append(segments, viralEditSegment{Path: path, Start: start, Duration: segmentDuration})
	}
	return segments, nil
}

func probeViralDimensions(ctx context.Context, videoPath string) (int, int, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", videoPath)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("đọc kích thước video thất bại: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("ffprobe trả kích thước video không hợp lệ")
	}
	width, wErr := strconv.Atoi(parts[0])
	height, hErr := strconv.Atoi(parts[1])
	if wErr != nil || hErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("ffprobe trả kích thước video không hợp lệ")
	}
	return width, height, nil
}

// isDanceOrPerformance kiểm tra chủ đề hoặc thể loại có phải là vũ đạo/nhảy/dance không.
func isDanceOrPerformance(topic, genre string) bool {
	t := strings.ToLower(topic)
	g := strings.ToLower(genre)
	return g == "dance" ||
		strings.Contains(t, "nhảy") ||
		strings.Contains(t, "dance") ||
		strings.Contains(t, "vũ đạo") ||
		strings.Contains(t, "sexy") ||
		strings.Contains(t, "choreo")
}

// createSolidColorPhoto tạo 1 ảnh nền mẫu chuẩn 9:16 hợp lệ (PNG) cho trường hợp offline, test hoặc quota hết.
func (s *Studio) createSolidColorPhoto(outPath string, w, h int, text string) error {
	_ = text
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Đổ nền gradient dọc tinh tế từ xanh đậm sang đen tạo bối cảnh điện ảnh
	for y := 0; y < h; y++ {
		factor := float64(y) / float64(h)
		r := uint8(15 + factor*10)
		g := uint8(23 + factor*15)
		b := uint8(42 + factor*20)
		col := color.RGBA{R: r, G: g, B: b, A: 255}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func copyViralFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

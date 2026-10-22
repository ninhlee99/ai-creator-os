package studio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// generateStorySilenceWav tạo track audio chuyển cảnh dự phòng chuẩn PCM 24kHz mono.
func generateStorySilenceWav(ctx context.Context, dst string, durationSec float64) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.2f", durationSec),
		"-acodec", "pcm_s16le", dst)
	return cmd.Run()
}

// concatWAVs nối các file wav thành 1 track audio m4a (aac).
func concatWAVs(ctx context.Context, wavs []string, outPath string) error {
	if len(wavs) == 0 {
		return fmt.Errorf("không có track giọng đọc")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	var args []string
	var fc []string
	for i, w := range wavs {
		args = append(args, "-i", w)
		fc = append(fc, fmt.Sprintf("[%d:a]", i))
	}
	fc = append(fc, fmt.Sprintf("concat=n=%d:v=0:a=1[aout]", len(wavs)))
	args = append(args, "-filter_complex", strings.Join(fc, ""), "-map", "[aout]",
		"-c:a", "aac", "-b:a", "192k", outPath)
	return ffmpegRun(ctx, args...)
}

// probeDuration trả về thời lượng giây của file media (0 khi lỗi).
// (Bản unexported: ProbeDuration của film_shots.go nằm sau build tag
// parked — không đụng vào để tránh trùng symbol khi build -tags parked.)
func storyProbeDuration(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var d float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d)
	if d < 0 {
		return 0
	}
	return d
}

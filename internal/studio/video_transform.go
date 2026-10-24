package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// TrimVideo cuts a single video to targetSec seconds while maintaining exact aspect ratio,
// native resolution (e.g. 1080x1920), 30fps, and high visual quality without concatenation artifacts.
func TrimVideo(ctx context.Context, inPath string, targetSec float64, aspect, outPath string) error {
	w, h := AspectDims(aspect)
	dir := filepath.Dir(outPath)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", w, h, w, h)
	var args []string
	args = append(args, "-i", inPath)
	if targetSec > 0 {
		args = append(args, "-t", fmt.Sprintf("%.2f", targetSec))
	}
	args = append(args, "-vf", vf, "-r", fmt.Sprint(outFPS))
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-an", outPath)
	return ffmpegRun(ctx, args...)
}

// ensureVideoDuration loops a short render to the requested minimum. This
// keeps short product/source clips from silently violating the delivery
// duration after trimming or audio muxing.
func ensureVideoDuration(ctx context.Context, path string, targetSec float64, aspect string) error {
	if targetSec <= 0 {
		return fmt.Errorf("invalid target duration %.2fs", targetSec)
	}
	duration := probeDuration(ctx, path)
	if duration <= 0 {
		return fmt.Errorf("cannot read video duration: %s", filepath.Base(path))
	}
	if duration >= targetSec-0.05 {
		return nil
	}
	w, h := AspectDims(aspect)
	filtered := path + ".extended.mp4"
	defer os.Remove(filtered)
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", w, h, w, h)
	args := []string{
		"-stream_loop", "-1", "-i", path,
		"-t", fmt.Sprintf("%.3f", targetSec),
		"-vf", vf, "-r", fmt.Sprint(outFPS),
	}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-map", "0:v:0", "-map", "0:a?", "-c:a", "aac", "-b:a", "192k", filtered)
	if err := ffmpegRun(ctx, args...); err != nil {
		return fmt.Errorf("extend video to %.2fs: %w", targetSec, err)
	}
	if got := probeDuration(ctx, filtered); got < targetSec-0.05 {
		return fmt.Errorf("extended video is only %.2fs; requires at least %.2fs", got, targetSec)
	}
	if err := os.Rename(filtered, path); err != nil {
		return fmt.Errorf("replace short video: %w", err)
	}
	return nil
}

// fitSilentVideoDuration conforms generated footage to the requested runtime
// without looping a short result. Short output means missing scenes and must
// fail instead of repeating frames or footage.
func fitSilentVideoDuration(ctx context.Context, path string, targetSec float64, aspect string) error {
	if targetSec <= 0 {
		return fmt.Errorf("invalid target duration %.2fs", targetSec)
	}
	duration := probeDuration(ctx, path)
	if duration <= 0 {
		return fmt.Errorf("cannot read video duration: %s", filepath.Base(path))
	}
	if duration < targetSec-0.15 {
		return fmt.Errorf("rendered footage %.2fs is shorter than required %.2fs; repeating footage is disabled", duration, targetSec)
	}
	if duration <= targetSec+0.05 {
		return nil
	}
	tmp := path + ".trimmed.mp4"
	defer os.Remove(tmp)
	if err := TrimVideo(ctx, path, targetSec, aspect, tmp); err != nil {
		return fmt.Errorf("trim footage to %.2fs: %w", targetSec, err)
	}
	if got := probeDuration(ctx, tmp); got < targetSec-0.15 {
		return fmt.Errorf("trimmed footage is only %.2fs; required %.2fs", got, targetSec)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace trimmed footage: %w", err)
	}
	return nil
}

// verifyVideoDuration checks final muxed output without silently extending it.
func verifyVideoDuration(ctx context.Context, path string, targetSec float64) error {
	if targetSec <= 0 {
		return fmt.Errorf("invalid target duration %.2fs", targetSec)
	}
	duration := probeDuration(ctx, path)
	if duration <= 0 {
		return fmt.Errorf("cannot read video duration: %s", filepath.Base(path))
	}
	if duration < targetSec-0.25 || duration > targetSec+0.25 {
		return fmt.Errorf("final video is %.2fs; expected %.2fs ±0.25s", duration, targetSec)
	}
	return nil
}

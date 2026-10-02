package studio

import (
	"context"
	"fmt"
	"os"
)

// Photo widths for the quality bar Ninh set (2026-10-01): model+product
// shots are mastered in 4K (2160x3840 for 9:16). 8K masters are supported
// for archival; the TikTok delivery render stays 1080x1920.
const (
	PhotoWidth4K = 2160
	PhotoWidth8K = 4320
)

// UpscalePhoto masters a generated photo to the target size (4K/8K).
// Generators output ~1K; this is an AI-detail upscale (lanczos + gentle
// sharpening), not native sensor resolution — the pipeline is honest about
// that. If the source already meets the target, it is copied as-is.
//
// w/h define the delivery frame: 9:16 affiliate masters use (2160,3840) /
// (4320,7680); film finals pass their own frame (e.g. 3840x2160 for a
// 4K 16:9 master — films ship 16:9 per Ninh's final 2026-10-02 decision,
// CreateFilmJob forces Aspect="16:9").
func UpscalePhoto(ctx context.Context, src, dst string, w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("upscale: kích thước %dx%d không hợp lệ", w, h)
	}
	if w != PhotoWidth4K && w != PhotoWidth8K && w != 3840 {
		return fmt.Errorf("upscale: unsupported width %d", w)
	}
	cur := ProbeWidth(ctx, src)
	if cur >= w {
		// Already at target: copy, don't waste a generation of quality.
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	}
	return ffmpegRun(ctx,
		"-i", src,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,"+
			"crop=%d:%d,unsharp=5:5:0.6:5:5:0.0", w, h, w, h),
		"-frames:v", "1", "-q:v", "2", dst)
}

// UpscaleVideo masters a finished video to w×h with lanczos. This is an
// UPSCALE, not native 4K — the UI must label it honestly. Slow: only for
// finals the user explicitly asks to upscale.
func UpscaleVideo(ctx context.Context, src, dst string, w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("upscale: kích thước %dx%d không hợp lệ", w, h)
	}
	args := []string{"-i", src,
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h),
	}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", dst)
	return ffmpegRun(ctx, args...)
}

package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConcatClips joins per-shot clips end to end (multi-shot mode). Every clip
// is probed and, when its frame differs from the job aspect, scaled+padded
// to AspectDims(aspect) first — ffmpeg's concat demuxer would otherwise emit
// a broken file. An unreadable clip fails loudly instead of a silent bad
// edit (P1-6).
func ConcatClips(ctx context.Context, clips []string, aspect, outPath string) error {
	if len(clips) == 0 {
		return fmt.Errorf("no clips")
	}
	w, h := AspectDims(aspect)
	dir := filepath.Dir(outPath)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	var norm []string
	var tmp []string
	defer func() {
		for _, t := range tmp {
			_ = os.Remove(t)
		}
	}()
	for _, c := range clips {
		pw, ph := ProbeDims(ctx, c)
		if pw == 0 || ph == 0 {
			return fmt.Errorf("clip không đọc được (ffprobe thất bại): %s", filepath.Base(c))
		}
		if pw == w && ph == h {
			norm = append(norm, c)
			continue
		}
		nc := filepath.Join(dir, fmt.Sprintf("norm-%d-%s", len(tmp), filepath.Base(c)))
		vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1",
			w, h, w, h)
		if err := ffmpegRun(ctx, "-v", "error", "-y", "-i", c,
			"-vf", vf, "-c:a", "aac", nc); err != nil {
			return fmt.Errorf("chuẩn hoá khổ %s về %dx%d: %w", filepath.Base(c), w, h, err)
		}
		tmp = append(tmp, nc)
		norm = append(norm, nc)
	}
	lst := filepath.Join(dir, "concat.txt")
	var sb strings.Builder
	for _, c := range norm {
		abs, err := filepath.Abs(c)
		if err != nil {
			return err
		}
		fmt.Fprintf(&sb, "file '%s'\n", abs)
	}
	if err := os.WriteFile(lst, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	args := []string{"-f", "concat", "-safe", "0", "-i", lst}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", outPath)
	return ffmpegRun(ctx, args...)
}

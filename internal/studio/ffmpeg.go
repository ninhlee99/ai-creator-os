package studio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// FFmpegRun chạy ffmpeg -y với args cho trước; lỗi kèm 500 ký tự cuối
// stderr để debug. Export cho reup transform (Đợt E) dùng chung.
func FFmpegRun(ctx context.Context, args ...string) error {
	return ffmpegRun(ctx, args...)
}

func ffmpegRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := stderr.String()
		if len(out) > 500 {
			out = out[len(out)-500:]
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, out)
	}
	return nil
}

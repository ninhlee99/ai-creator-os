package studio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
)

const outFPS = 30

// (Delivery dimensions come from AspectDims in studio.go: every renderer
// takes the job aspect explicitly, so a "long" YouTube cut can never
// silently render vertical.)

// videoEncoder picks the H.264 encoder once per process: Apple's hardware
// encoder (h264_videotoolbox — included in macOS ffmpeg builds such as
// Homebrew's) when present, libx264 otherwise. Delivery renders are
// 1080x1920 for platforms that re-encode on upload anyway, so the hardware
// encoder's fixed bitrate costs nothing visible, runs several times
// faster, and frees CPU cores for the zoompan/scale filters — which stay
// on CPU either way and are the real bottleneck of these graphs.
var videoEncoder = sync.OnceValue(func() []string {
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output(); err == nil &&
		bytes.Contains(out, []byte("h264_videotoolbox")) {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "12M", "-maxrate", "16M"}
	}
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "18"}
})

// VideoEncoderArgs trả về args encoder H.264 đã chọn cho process này —
// dùng chung cho reup transform (Đợt E) để không copy logic chọn encoder.
func VideoEncoderArgs() []string { return videoEncoder() }

var videoEncoder4K = sync.OnceValue(func() []string {
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output(); err == nil &&
		bytes.Contains(out, []byte("h264_videotoolbox")) {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "35M", "-maxrate", "45M"}
	}
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "16", "-b:v", "35M"}
})

var videoEncoder8K = sync.OnceValue(func() []string {
	encoders, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if bytes.Contains(encoders, []byte("hevc_videotoolbox")) {
		return []string{"-c:v", "hevc_videotoolbox", "-b:v", "85M", "-maxrate", "110M"}
	}
	if bytes.Contains(encoders, []byte("libx265")) {
		return []string{"-c:v", "libx265", "-preset", "medium", "-crf", "17", "-tag:v", "hvc1"}
	}
	if bytes.Contains(encoders, []byte("h264_videotoolbox")) {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "100M", "-maxrate", "125M"}
	}
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "15", "-b:v", "100M"}
})

// VideoEncoderArgs4K trả về args encoder H.264 chất lượng Master 4K UHD với bitrate 35M.
func VideoEncoderArgs4K() []string { return videoEncoder4K() }

// VideoEncoderArgs8K prefers hardware HEVC, then software HEVC, for 8K masters.
func VideoEncoderArgs8K() []string { return videoEncoder8K() }

// Master4KFilter trả về chuỗi bộ lọc FFmpeg 2 tầng chống vỡ hình và chống nhòe khi nâng lên 4K:
// Khử nhiễu nén (hqdn3d) + Phóng to đa thùy Lanczos 3-lobe chuẩn xác + Tái tạo tương phản vi mô (unsharp).
func Master4KFilter(outW, outH int) string {
	return fmt.Sprintf("hqdn3d=1.5:1.5:4:4,scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos+accurate_rnd,crop=%d:%d,unsharp=5:5:0.8:5:5:0.2,setsar=1", outW, outH, outW, outH)
}

// UpscaleVideoTo4K keeps the legacy 4K entry point for callers outside Studio.
func UpscaleVideoTo4K(ctx context.Context, srcVideo, dstVideo, aspect string) error {
	return UpscaleVideo(ctx, srcVideo, dstVideo, aspect, VideoOutput4K)
}

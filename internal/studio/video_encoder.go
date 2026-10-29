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
// takes the job aspect explicitly, so video rendering always follows
// silently render vertical.)

// videoEncoder picks the H.264 encoder once per process: Apple's hardware
// encoder only when a real encode probe succeeds, libx264 otherwise. Delivery renders are
// 1080x1920 for platforms that re-encode on upload anyway, so the hardware
// encoder's fixed bitrate costs nothing visible, runs several times
// faster, and frees CPU cores for the zoompan/scale filters — which stay
// on CPU either way and are the real bottleneck of these graphs.
var videoToolboxH264Available = sync.OnceValue(func() bool { return probeVideoToolboxEncoder("h264_videotoolbox") })
var videoToolboxHEVCAvailable = sync.OnceValue(func() bool { return probeVideoToolboxEncoder("hevc_videotoolbox") })
var softwareHEVCAvailable = sync.OnceValue(func() bool {
	encoders, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	return err == nil && bytes.Contains(encoders, []byte("libx265"))
})

func probeVideoToolboxEncoder(codec string) bool {
	encoders, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(encoders, []byte(codec)) {
		return false
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:r=30:d=1",
		"-frames:v", "2", "-c:v", codec, "-f", "null", "-")
	return cmd.Run() == nil
}

var videoEncoder = sync.OnceValue(func() []string {
	if videoToolboxH264Available() {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "12M", "-maxrate", "16M"}
	}
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "18"}
})

// VideoEncoderArgs trả về args encoder H.264 đã chọn cho process này —
// dùng chung cho reup transform (Đợt E) để không copy logic chọn encoder.
func VideoEncoderArgs() []string { return videoEncoder() }

var videoEncoder2K = sync.OnceValue(func() []string {
	if videoToolboxH264Available() {
		return []string{"-c:v", "h264_videotoolbox", "-b:v", "20M", "-maxrate", "26M"}
	}
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "18", "-b:v", "20M"}
})

// VideoEncoderArgs2K trả về args encoder H.264 chất lượng Master 2K QHD với bitrate 20M.
func VideoEncoderArgs2K() []string { return videoEncoder2K() }

// Master2KFilter trả về chuỗi bộ lọc FFmpeg 2 tầng chống vỡ hình và chống nhòe khi nâng lên 2K:
// Khử nhiễu nén (hqdn3d) + Phóng to đa thùy Lanczos 3-lobe chuẩn xác + Tái tạo tương phản vi mô (unsharp).
func Master2KFilter(outW, outH int) string {
	return fmt.Sprintf("hqdn3d=1.0:1.0:3:3,scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos+accurate_rnd,crop=%d:%d,unsharp=5:5:0.6:5:5:0.2,setsar=1", outW, outH, outW, outH)
}

// UpscaleVideoTo2K nâng cấp video lên chuẩn 2K.
func UpscaleVideoTo2K(ctx context.Context, srcVideo, dstVideo, aspect string) error {
	return UpscaleVideo(ctx, srcVideo, dstVideo, aspect, VideoOutput2K)
}

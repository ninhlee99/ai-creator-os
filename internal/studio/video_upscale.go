package studio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func NormalizeVideoOutputResolution(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", VideoOutput1080P:
		return VideoOutput1080P, nil
	case VideoOutput2K, "1440p", "2560x1440", "1440x2560":
		return VideoOutput2K, nil
	default:
		return "", fmt.Errorf("độ phân giải đầu ra không hỗ trợ: %q (chỉ hỗ trợ 1080p và 2k)", value)
	}
}

func VideoOutputDims(aspect, resolution string) (int, int, error) {
	if aspect != "9:16" && aspect != "16:9" && aspect != "1:1" {
		return 0, 0, fmt.Errorf("tỉ lệ video không hỗ trợ: %q", aspect)
	}
	resolution, err := NormalizeVideoOutputResolution(resolution)
	if err != nil {
		return 0, 0, err
	}
	if resolution == VideoOutput1080P {
		if aspect == "1:1" {
			return 1080, 1080, nil
		}
		w, h := AspectDims(aspect)
		return w, h, nil
	}
	w, h := AspectDims2K(aspect)
	return w, h, nil
}

// VideoMasterFilter removes mild spatial compression noise, upscales once
// with Lanczos, and pads to the target canvas. It avoids temporal denoising
// and strong sharpening so moving details do not smear or grow halos. Padding
// preserves the full source frame when its aspect differs; this never crops
// or stretches the image.
func VideoMasterFilter(width, height int) string {
	return fmt.Sprintf("hqdn3d=0.6:0.5:0:0,scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos+accurate_rnd+full_chroma_int,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,unsharp=5:5:0.3:5:5:0.0,setsar=1,format=yuv420p",
		width, height, width, height)
}

// ConcatClipsToMaster trims and joins source clips while scaling directly to
// the selected master canvas in one video encode. It avoids first enlarging
// 360p/480p footage to the normal 1080p delivery canvas and enlarging it again.
func ConcatClipsToMaster(ctx context.Context, clips []string, durations []float64, aspect, resolution, dstVideo string, keepAudio bool) error {
	if len(clips) == 0 || len(clips) != len(durations) {
		return errors.New("upscale: clip và thời lượng phải khớp, không được rỗng")
	}
	if strings.TrimSpace(dstVideo) == "" {
		return errors.New("upscale: thiếu đường dẫn đầu ra")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolution, err := NormalizeVideoOutputResolution(resolution)
	if err != nil {
		return err
	}
	width, height, err := VideoOutputDims(aspect, resolution)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstVideo), 0o755); err != nil {
		return fmt.Errorf("upscale: tạo thư mục đầu ra: %w", err)
	}

	hasAudio := false
	if keepAudio {
		hasAudio = ProbeHasAudio(ctx, clips[0])
	}
	var args []string
	var filter strings.Builder
	var concatInputs strings.Builder
	var expectedDuration float64
	for i, clip := range clips {
		seconds := durations[i]
		if strings.TrimSpace(clip) == "" || seconds <= 0 {
			return fmt.Errorf("upscale: clip %d hoặc thời lượng không hợp lệ", i+1)
		}
		clipWidth, clipHeight := ProbeDims(ctx, clip)
		if clipWidth <= 0 || clipHeight <= 0 {
			return fmt.Errorf("upscale: clip không đọc được: %s", filepath.Base(clip))
		}
		clipDuration := probeDuration(ctx, clip)
		if clipDuration <= 0 || clipDuration < seconds-0.15 {
			return fmt.Errorf("upscale: clip %s dài %.3fs, cần ít nhất %.3fs", filepath.Base(clip), clipDuration, seconds)
		}
		if keepAudio && ProbeHasAudio(ctx, clip) != hasAudio {
			return fmt.Errorf("upscale: các clip có cấu hình audio khác nhau")
		}
		args = append(args, "-i", clip)
		fmt.Fprintf(&filter, "[%d:v:0]trim=duration=%.6f,setpts=PTS-STARTPTS,%s,fps=30[v%d];",
			i, seconds, VideoMasterFilter(width, height), i)
		fmt.Fprintf(&concatInputs, "[v%d]", i)
		if hasAudio {
			fmt.Fprintf(&filter, "[%d:a:0]atrim=duration=%.6f,asetpts=PTS-STARTPTS,aresample=48000[a%d];", i, seconds, i)
			fmt.Fprintf(&concatInputs, "[a%d]", i)
		}
		expectedDuration += seconds
	}
	if hasAudio {
		fmt.Fprintf(&filter, "%sconcat=n=%d:v=1:a=1[vout][aout]", concatInputs.String(), len(clips))
	} else {
		fmt.Fprintf(&filter, "%sconcat=n=%d:v=1:a=0[vout]", concatInputs.String(), len(clips))
	}
	args = append(args, "-filter_complex", filter.String(), "-map", "[vout]")
	if hasAudio {
		args = append(args, "-map", "[aout]")
	}
	if resolution == VideoOutput2K {
		args = append(args, VideoEncoderArgs2K()...)
	} else {
		args = append(args, VideoEncoderArgs()...)
	}
	args = append(args, "-fps_mode", "passthrough", "-pix_fmt", "yuv420p")
	if hasAudio {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-movflags", "+faststart")
	tmp := dstVideo + ".partial.mp4"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	args = append(args, tmp)
	if err := ffmpegRun(ctx, args...); err != nil {
		return fmt.Errorf("upscale %s master: %w", strings.ToUpper(resolution), err)
	}
	outWidth, outHeight := ProbeDims(ctx, tmp)
	if outWidth != width || outHeight != height {
		return fmt.Errorf("upscale: đầu ra %dx%d, cần %dx%d", outWidth, outHeight, width, height)
	}
	outputDuration := probeDuration(ctx, tmp)
	if outputDuration <= 0 || math.Abs(outputDuration-expectedDuration) > 0.25 {
		return fmt.Errorf("upscale: thời lượng đầu ra %.3fs lệch tổng shot %.3fs", outputDuration, expectedDuration)
	}
	_ = os.Remove(dstVideo)
	if err := os.Rename(tmp, dstVideo); err != nil {
		return fmt.Errorf("upscale: lưu master: %w", err)
	}
	return nil
}

// UpscaleVideo creates a 1080p or 2K master after generation. It preserves the full
// frame, aspect ratio, timing, and audio. Upscaling increases pixel dimensions;
// it cannot recover details absent from a 360p/480p source.
func UpscaleVideo(ctx context.Context, srcVideo, dstVideo, aspect, resolution string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if srcVideo == "" || dstVideo == "" || filepath.Clean(srcVideo) == filepath.Clean(dstVideo) {
		return errors.New("upscale: đường dẫn nguồn và đích phải khác nhau")
	}
	resolution, err := NormalizeVideoOutputResolution(resolution)
	if err != nil {
		return err
	}
	width, height, err := VideoOutputDims(aspect, resolution)
	if err != nil {
		return err
	}
	sourceWidth, sourceHeight := ProbeDims(ctx, srcVideo)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return fmt.Errorf("upscale: nguồn video không đọc được: %s", filepath.Base(srcVideo))
	}
	sourceDuration := probeDuration(ctx, srcVideo)
	if sourceDuration <= 0 {
		return fmt.Errorf("upscale: không đọc được thời lượng nguồn: %s", filepath.Base(srcVideo))
	}
	sourceExt := strings.ToLower(filepath.Ext(srcVideo))
	if sourceWidth == width && sourceHeight == height && (sourceExt == ".mp4" || sourceExt == ".m4v") {
		return copyMasterVideo(srcVideo, dstVideo)
	}
	if dstVideo == "" || filepath.Clean(srcVideo) == filepath.Clean(dstVideo) {
		return errors.New("upscale: đường dẫn nguồn và đích phải khác nhau")
	}
	if err := os.MkdirAll(filepath.Dir(dstVideo), 0o755); err != nil {
		return fmt.Errorf("upscale: tạo thư mục đầu ra: %w", err)
	}
	tmp := dstVideo + ".partial.mp4"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	args := []string{"-i", srcVideo, "-map", "0:v:0", "-map", "0:a?", "-vf", VideoMasterFilter(width, height)}
	if resolution == VideoOutput2K {
		args = append(args, VideoEncoderArgs2K()...)
	} else {
		args = append(args, VideoEncoderArgs()...)
	}
	args = append(args, "-fps_mode", "passthrough", "-pix_fmt", "yuv420p", "-c:a", "copy", "-movflags", "+faststart", tmp)
	if err := ffmpegRun(ctx, args...); err != nil {
		return fmt.Errorf("upscale %s master: %w", strings.ToUpper(resolution), err)
	}
	outWidth, outHeight := ProbeDims(ctx, tmp)
	if outWidth != width || outHeight != height {
		return fmt.Errorf("upscale: đầu ra %dx%d, cần %dx%d", outWidth, outHeight, width, height)
	}
	outputDuration := probeDuration(ctx, tmp)
	if outputDuration <= 0 || math.Abs(outputDuration-sourceDuration) > 0.15 {
		return fmt.Errorf("upscale: thời lượng đầu ra %.3fs lệch nguồn %.3fs", outputDuration, sourceDuration)
	}
	if err := os.Rename(tmp, dstVideo); err != nil {
		return fmt.Errorf("upscale: lưu master: %w", err)
	}
	return nil
}

func copyMasterVideo(src, dst string) error {
	if src == "" || dst == "" || filepath.Clean(src) == filepath.Clean(dst) {
		return errors.New("upscale: đường dẫn nguồn và đích phải khác nhau")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".aicos-master-*.mp4")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	if _, err := in.WriteTo(out); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Chmod(0o644); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

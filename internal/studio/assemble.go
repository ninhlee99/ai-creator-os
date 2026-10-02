package studio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	return []string{"-c:v", "libx264", "-preset", "medium", "-crf", "18"}
})

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

// AssemblePhotoList builds the photo-list spot with the quality bar Ninh
// demands: each photo gets a slow Ken Burns drift (no dead-static holds),
// 0.6s crossfades between photos, and the trending track loudness-matched
// to -14 LUFS with fades. No text, no voiceover — Ninh's fashion format.
//
// aspect is "9:16" (1080x1920) or "16:9" (1920x1080) — the delivery frame
// comes from the job, never a package constant.
//
// When musicPath == "" the output is a silent video (no bogus audio input).
func AssemblePhotoList(ctx context.Context, photos []string, secsPer float64, musicPath string, musicStart float64, outPath, aspect string) error {
	if len(photos) == 0 {
		return fmt.Errorf("no photos")
	}
	if secsPer < 1 {
		secsPer = 4
	}
	var args []string
	for _, p := range photos {
		args = append(args, "-i", p) // single image each; zoompan multiplies frames
	}
	w, h := AspectDims(aspect)
	filter, total := photoListFilter(len(photos), secsPer, w, h)

	if musicPath != "" {
		args = append(args, "-ss", fmt.Sprintf("%.1f", musicStart), "-i", musicPath)
		filter += ";" + fmt.Sprintf("[%d:a]atrim=0:%.2f,afade=t=in:st=0:d=1,"+
			"afade=t=out:st=%.1f:d=1,loudnorm=I=-14:TP=-1.5:LRA=11[aout]",
			len(photos), total, total-1)
	}
	args = append(args,
		"-filter_complex", filter,
		"-map", "[vout]",
		"-t", fmt.Sprintf("%.2f", total),
		"-r", fmt.Sprint(outFPS),
	)
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p")
	if musicPath != "" {
		args = append(args, "-map", "[aout]", "-c:a", "aac", "-b:a", "160k")
	}
	args = append(args, outPath)
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx, args...)
}

// photoListFilter builds the Ken Burns + xfade graph for n photos at the
// given output dimensions. Returns the filter string and the total output
// duration.
func photoListFilter(n int, secsPer float64, outW, outH int) (string, float64) {
	const fade = 0.6
	frames := int(secsPer*outFPS + 0.5)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		// Alternate drift direction so the piece breathes instead of
		// pushing in forever: even = slow push-in, odd = slow pull-out.
		zoom := fmt.Sprintf("min(1+%.6f*on,1.16)", 0.16/float64(frames))
		if i%2 == 1 {
			zoom = fmt.Sprintf("max(1.16-%.6f*on,1.0)", 0.16/float64(frames))
		}
		fmt.Fprintf(&sb,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,"+
				"crop=%d:%d,setsar=1,"+
				"zoompan=z='%s':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=%d[v%d];",
			i, outW*2, outH*2, outW*2, outH*2, zoom, frames, outW, outH, outFPS, i)
	}
	cur := "[v0]"
	for i := 1; i < n; i++ {
		offset := float64(i)*secsPer - float64(i)*fade
		out := fmt.Sprintf("[x%d]", i)
		if i == n-1 {
			out = "[vout]"
		}
		fmt.Fprintf(&sb, "%s[v%d]xfade=transition=fade:duration=%.1f:offset=%.2f%s;",
			cur, i, fade, offset, out)
		cur = out
	}
	if n == 1 {
		sb.WriteString("[v0]null[vout];")
	}
	total := float64(n)*secsPer - float64(n-1)*fade
	return strings.TrimSuffix(sb.String(), ";"), total
}

// AssembleBeatBounce builds the CapCut-style "giật giật" affiliate spot
// Ninh asked for (2026-10-01): a handful of photos of the model using the
// product, each photo bouncing (zoom pulse) on every beat with a slight
// positional shake, hard cuts between photos — no crossfades. Music is
// loudness-matched to -14 LUFS with fades. No text, no voiceover.
//
// aspect is "9:16" (1080x1920) or "16:9" (1920x1080).
//
// When musicPath == "" the output is a silent video.
func AssembleBeatBounce(ctx context.Context, photos []string, secsPer float64, bpm int, musicPath string, musicStart float64, outPath, aspect string) error {
	if len(photos) == 0 {
		return fmt.Errorf("no photos")
	}
	if secsPer < 1 {
		secsPer = 3
	}
	if bpm < 60 || bpm > 200 {
		bpm = 120 // default: the common TikTok trending tempo
	}
	var args []string
	for _, p := range photos {
		args = append(args, "-i", p)
	}
	filter, total := beatBounceFilter(len(photos), secsPer, bpm, aspect)

	if musicPath != "" {
		args = append(args, "-ss", fmt.Sprintf("%.1f", musicStart), "-i", musicPath)
		filter += ";" + fmt.Sprintf("[%d:a]atrim=0:%.2f,afade=t=in:st=0:d=1,"+
			"afade=t=out:st=%.1f:d=1,loudnorm=I=-14:TP=-1.5:LRA=11[aout]",
			len(photos), total, total-1)
	}
	args = append(args,
		"-filter_complex", filter,
		"-map", "[vout]",
		"-t", fmt.Sprintf("%.2f", total),
		"-r", fmt.Sprint(outFPS),
	)
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p")
	if musicPath != "" {
		args = append(args, "-map", "[aout]", "-c:a", "aac", "-b:a", "160k")
	}
	args = append(args, outPath)
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx, args...)
}

// beatBounceFilter builds the per-photo beat-bounce graph and hard-cut
// concat. Each photo: supersampled fill, zoompan with a zoom pulse peaking
// on every beat (sharpened with pow 2 so the "giật" feels snappy), then a
// crop with an oscillating offset for the handheld shake. Base zoom 1.06
// keeps the shake inside the frame (no black corners).
func beatBounceFilter(n int, secsPer float64, bpm int, aspect string) (string, float64) {
	frames := int(secsPer*outFPS + 0.5)
	beat := float64(outFPS) * 60 / float64(bpm) // frames per beat
	// Output frame + overscan headroom: zoompan renders slightly larger
	// than the output so the shake crop never reveals edges (overscan is
	// ~11% per axis, the old 1200x2133 constants generalized).
	outW, outH := AspectDims(aspect)
	overW, overH := outW*10/9, outH*10/9
	var sb strings.Builder
	for i := 0; i < n; i++ {
		zoom := fmt.Sprintf("1.06+0.14*pow(max(0,sin(2*PI*on/%.4f)),2)", beat)
		fmt.Fprintf(&sb,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,"+
				"crop=%d:%d,setsar=1,"+
				"zoompan=z='%s':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=%d,"+
				"crop=%d:%d:x='(in_w-%d)/2+30*sin(2*PI*n/%.4f)':y='(in_h-%d)/2+24*cos(2*PI*2*n/%.4f)'[v%d];",
			i, outW*2, outH*2, outW*2, outH*2,
			zoom, frames, overW, overH, outFPS,
			outW, outH, outW, beat, outH, beat, i)
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "[v%d]", i)
	}
	fmt.Fprintf(&sb, "concat=n=%d:v=1:a=0[vout];", n)
	total := float64(n) * secsPer
	return strings.TrimSuffix(sb.String(), ";"), total
}

// ProbeDims returns a video/image's width×height in pixels (0,0 on failure).
func ProbeDims(ctx context.Context, path string) (int, int) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, 0
	}
	var w, h int
	fmt.Sscanf(strings.TrimSpace(out.String()), "%d,%d", &w, &h)
	return w, h
}

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

// MuxMusic replaces/adds the music bed on a silent edit.
func MuxMusic(ctx context.Context, videoPath, musicPath string, musicStart float64, outPath string) error {
	return ffmpegRun(ctx,
		"-i", videoPath, "-i", musicPath,
		"-ss", fmt.Sprintf("%.1f", musicStart),
		"-map", "0:v", "-map", "1:a",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "160k",
		"-shortest", outPath)
}

// ProbeWidth returns the video/image width in pixels (0 on failure).
func ProbeWidth(ctx context.Context, path string) int {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width", "-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}
	var w int
	fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &w)
	return w
}

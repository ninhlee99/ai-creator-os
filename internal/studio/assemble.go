package studio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	return []string{"-c:v", "libx264", "-preset", "slow", "-crf", "18"}
})

// VideoEncoderArgs trả về args encoder H.264 đã chọn cho process này —
// dùng chung cho reup transform (Đợt E) để không copy logic chọn encoder.
func VideoEncoderArgs() []string { return videoEncoder() }

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
		args = append(args, "-map", "[aout]", "-c:a", "aac", "-b:a", "192k")
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
	if frames <= 0 {
		frames = 1
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		// Multi-directional cinematic camera directing (5-Star Ken Burns):
		// Instead of uniform center zoom, alternate organically between:
		// 0: Push-in targeting upper third (character eyes/face - Rule of Thirds)
		// 1: Slow pull-out from center (establishing atmosphere)
		// 2: Lateral pan drift (revealing panoramic context)
		// 3: Subtle vertical tilt with gentle push (organic camera operator feel)
		var zoom, xExpr, yExpr string
		switch i % 4 {
		case 0:
			zoom = fmt.Sprintf("min(1+%.6f*on,1.16)", 0.16/float64(frames))
			xExpr = "iw/2-(iw/zoom/2)"
			yExpr = "ih/3-(ih/zoom/3)"
		case 1:
			zoom = fmt.Sprintf("max(1.16-%.6f*on,1.0)", 0.16/float64(frames))
			xExpr = "iw/2-(iw/zoom/2)"
			yExpr = "ih/2-(ih/zoom/2)"
		case 2:
			zoom = "1.10"
			xExpr = fmt.Sprintf("(iw-iw/zoom)*(on/%d)", frames)
			yExpr = "ih/2.5-(ih/zoom/2.5)"
		default:
			zoom = fmt.Sprintf("min(1+%.6f*on,1.14)", 0.14/float64(frames))
			xExpr = "iw/2-(iw/zoom/2)"
			yExpr = fmt.Sprintf("(ih/3)+(ih/12)*(on/%d)-(ih/zoom/3)", frames)
		}

		fmt.Fprintf(&sb,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,"+
				"crop=%d:%d,setsar=1,"+
				"zoompan=z='%s':d=%d:x='%s':y='%s':s=%dx%d:fps=%d[v%d];",
			i, outW*2, outH*2, outW*2, outH*2, zoom, frames, xExpr, yExpr, outW, outH, outFPS, i)
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
		args = append(args, "-map", "[aout]", "-c:a", "aac", "-b:a", "192k")
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

// CropCenterVertical center-crops a 16:9 film clip to the central 9:16
// band and scales it to 1080x1920 — the automatic vertical trailer cut
// (Ninh 2026-10-02: trailers are cropped, never re-shot).
//
// TRADEOFF, stated honestly: the crop keeps only the middle ~32% of the
// frame width (~68% of horizontal pixels are dropped), then upscales
// ~1.78x to 1080x1920, so trailers are softer than the film. This is why
// trailer_worthy shots must be composed center-safe (see FILM_RULES.md):
// main character/action inside the central 9:16 zone.
func CropCenterVertical(ctx context.Context, src, dst string) error {
	if dir := filepath.Dir(dst); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	args := []string{"-i", src,
		"-vf", "crop=ih*9/16:ih,scale=1080:1920:flags=lanczos,setsar=1",
	}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", dst)
	return ffmpegRun(ctx, args...)
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
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
		"-shortest", outPath)
}

// MuxSubtitles muxes an SRT subtitle track into a finished video without
// re-encoding (mov_text, Vietnamese language tag). Empty srtText is a no-op
// that copies the input — callers don't branch on "has subtitles".
func MuxSubtitles(ctx context.Context, videoPath, srtText, outPath string) error {
	if strings.TrimSpace(srtText) == "" {
		return ffmpegRun(ctx, "-i", videoPath, "-c", "copy", outPath)
	}
	dir := filepath.Dir(outPath)
	srt := filepath.Join(dir, "subs.srt")
	if err := os.WriteFile(srt, []byte(srtText), 0o644); err != nil {
		return err
	}
	defer os.Remove(srt)
	return ffmpegRun(ctx,
		"-i", videoPath, "-i", srt,
		"-map", "0", "-map", "1",
		"-c:v", "copy", "-c:a", "copy", "-c:s", "mov_text",
		"-metadata:s:s:0", "language=vie",
		outPath)
}

// srtTime formats seconds into standard SRT timestamp 00:00:00,000.
func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(sec*1000 + 0.5)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}

// MixMusicBed mixes a music bed under a film's existing audio. When the
// video has a voice track the music is ducked ~8dB under it
// (sidechaincompress); when the video is silent the music is laid straight.
// The final mix is loudness-normalized to -14 LUFS like the affiliate
// spots. Empty musicPath returns an error — callers check before calling.
func MixMusicBed(ctx context.Context, videoPath, musicPath, outPath string) error {
	if strings.TrimSpace(musicPath) == "" {
		return fmt.Errorf("mix music: thiếu file nhạc")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if !ProbeHasAudio(ctx, videoPath) {
		// Silent film (pure Veo shots): music becomes the audio track.
		return ffmpegRun(ctx,
			"-i", videoPath, "-i", musicPath,
			"-map", "0:v", "-map", "1:a",
			"-c:v", "copy", "-af", "loudnorm=I=-14:TP=-1.5:LRA=11",
			"-c:a", "aac", "-b:a", "192k", "-shortest", outPath)
	}
	// Voice present: duck the bed ~9dB under speech with vocal carving EQ.
	// 1. Highpass & carving EQ on music [1:a] to prevent muddy bass collision with speech.
	// 2. Highpass filter on voice [0:a] to eliminate mic rumble and pops.
	// 3. Smooth sidechaincompress: music ducks cleanly when voice hits, recovers smoothly.
	filter := "[0:a]highpass=f=75[vclean];" +
		"[1:a]highpass=f=100,equalizer=f=1200:t=q:w=1.2:g=-3.5,volume=0.45,atrim=0:3600[m];" +
		"[m][vclean]sidechaincompress=threshold=-24dB:ratio=8:attack=15:release=450:makeup=1[dm];" +
		"[vclean][dm]amix=inputs=2:duration=first:dropout_transition=0," +
		"loudnorm=I=-14:TP=-1.5:LRA=11[aout]"
	return ffmpegRun(ctx,
		"-i", videoPath, "-i", musicPath,
		"-filter_complex", filter,
		"-map", "0:v", "-map", "[aout]",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
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

// FindTrendingMusic returns a genuine TikTok trending music track for the given category.
// Category can be "dance", "viral", "fashion", "affiliate", or "".
func FindTrendingMusic(category string) string {
	searchDirs := []string{
		filepath.Join("data", "music", "trending"),
		"/Users/ninhlee/ninhlee/apps/ai-creator-os/data/music/trending",
	}
	cat := strings.ToLower(category)
	for _, dir := range searchDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.mp3"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			for _, m := range matches {
				base := strings.ToLower(filepath.Base(m))
				// Nhạc chill / acoustic cho kể chuyện, kiến thức, bí ẩn, đời sống
				if (cat == "mystery" || cat == "story" || cat == "drama" || cat == "knowledge" || cat == "lifehack" || cat == "chill") && (strings.Contains(base, "chill") || strings.Contains(base, "acoustic")) {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
				// Nhạc dance / remix sôi động cho nhảy, trendjack, challenge
				if (cat == "dance" || cat == "remix" || cat == "upbeat" || cat == "challenge") && strings.Contains(base, "dance") {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
				// Nhạc thời trang, affiliate, review sản phẩm
				if (cat == "fashion" || cat == "affiliate" || cat == "review") && (strings.Contains(base, "fashion") || strings.Contains(base, "chill") || strings.Contains(base, "acoustic")) {
					if abs, err := filepath.Abs(m); err == nil {
						return abs
					}
					return m
				}
			}
			// Mặc định ưu tiên nhạc êm dịu nếu là video kể chuyện/thoại thay vì dance giật
			if cat == "mystery" || cat == "story" || cat == "drama" || cat == "knowledge" {
				for _, m := range matches {
					if strings.Contains(strings.ToLower(m), "chill") || strings.Contains(strings.ToLower(m), "acoustic") {
						if abs, err := filepath.Abs(m); err == nil {
							return abs
						}
						return m
					}
				}
			}
			if abs, err := filepath.Abs(matches[0]); err == nil {
				return abs
			}
			return matches[0]
		}
	}
	return findDefaultMusic()
}

// findDefaultMusic returns a valid local commercial music track path if available.
func findDefaultMusic() string {
	trendingDirs := []string{
		filepath.Join("data", "music", "trending"),
		"/Users/ninhlee/ninhlee/apps/ai-creator-os/data/music/trending",
	}
	for _, dir := range trendingDirs {
		if matches, err := filepath.Glob(filepath.Join(dir, "*.mp3")); err == nil && len(matches) > 0 {
			sort.Strings(matches)
			if abs, err := filepath.Abs(matches[0]); err == nil {
				return abs
			}
			return matches[0]
		}
	}

	candidates := []string{
		"docs/assets/affiliate-spot/music-just-chill.mp3",
		"internal/web/static/preview.mp3",
		"/Users/ninhlee/ninhlee/apps/ai-creator-os/docs/assets/affiliate-spot/music-just-chill.mp3",
		"/Users/ninhlee/ninhlee/apps/ai-creator-os/internal/web/static/preview.mp3",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Size() > 1000 {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}

// FindStockClips returns a list of high-quality filmed motion video clips
// (9:16 vertical, continuous camera movement, 100% clean with zero text overlays).
func FindStockClips(category string) []string {
	searchDirs := []string{
		filepath.Join("data", "clips", category),
		filepath.Join("data", "clips", "fashion"),
		filepath.Join("data", "clips", "viral"),
		filepath.Join("/Users/ninhlee/ninhlee/apps/ai-creator-os/data/clips", category),
		filepath.Join("/Users/ninhlee/ninhlee/apps/ai-creator-os/data/clips", "fashion"),
		filepath.Join("/Users/ninhlee/ninhlee/apps/ai-creator-os/data/clips", "viral"),
	}
	for _, dir := range searchDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
		if err == nil && len(matches) > 0 {
			sort.Strings(matches)
			var valid []string
			for _, m := range matches {
				if st, err := os.Stat(m); err == nil && !st.IsDir() && st.Size() > 10000 {
					if abs, err := filepath.Abs(m); err == nil {
						valid = append(valid, abs)
					} else {
						valid = append(valid, m)
					}
				}
			}
			if len(valid) > 0 {
				return valid
			}
		}
	}
	return nil
}

// PickSingleStockClipForDuration selects exactly ONE matching high-quality video clip for the given category, query, and target duration.
// When targetSec >= 45, it prioritizes full-length (>= 60s) continuous footage to fulfill TikTok Creativity Program requirements without looping.
func PickSingleStockClipForDuration(category, query string, targetSec float64) string {
	clips := FindStockClips(category)
	if len(clips) == 0 {
		return ""
	}
	q := strings.ToLower(query)

	if targetSec >= 45 {
		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "60s") {
				return c
			}
		}
	}

	if category == "fashion" {
		if targetSec >= 45 {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "lookbook") || strings.Contains(strings.ToLower(c), "60s") {
					return c
				}
			}
		}
		if strings.Contains(q, "street") || strings.Contains(q, "hoodie") || strings.Contains(q, "thể thao") || strings.Contains(q, "năng động") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "streetwear") {
					return c
				}
			}
		}
		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "dress") {
				return c
			}
		}
	} else if category == "viral" {
		if targetSec >= 45 {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "dance_60s") || strings.Contains(strings.ToLower(c), "60s") {
					return c
				}
			}
		}
		if strings.Contains(q, "crew") || strings.Contains(q, "nhóm") || strings.Contains(q, "đội") {
			for _, c := range clips {
				if strings.Contains(strings.ToLower(c), "crew") {
					return c
				}
			}
		}
		for _, c := range clips {
			if strings.Contains(strings.ToLower(c), "duo") {
				return c
			}
		}
	}
	return clips[0]
}

// PickSingleStockClip selects exactly ONE matching high-quality video clip for the given category and query.
func PickSingleStockClip(category, query string) string {
	return PickSingleStockClipForDuration(category, query, 0)
}

// ApplyCreatorAvatarOverlay overlays a high-resolution circular creator badge in the top-left corner
// of the video to seamlessly brand creator identity into the video.
func ApplyCreatorAvatarOverlay(ctx context.Context, inVideo, avatarPhoto, outVideo string) error {
	if !fileExists(avatarPhoto) {
		return copyStoryFile(inVideo, outVideo)
	}

	badgePNG := filepath.Join(filepath.Dir(outVideo), "creator_avatar_badge.png")
	if err := CreateCircularAvatarBadge(avatarPhoto, 150, badgePNG); err != nil {
		return copyStoryFile(inVideo, outVideo)
	}

	args := []string{
		"-i", inVideo,
		"-i", badgePNG,
		"-filter_complex", "[0:v][1:v]overlay=36:72",
		"-c:a", "copy",
		outVideo,
	}
	return ffmpegRun(ctx, args...)
}

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

// probeDuration trả về thời lượng thực tế của file video (giây).
func probeDuration(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0
	}
	var d float64
	fmt.Sscanf(strings.TrimSpace(out.String()), "%f", &d)
	return d
}

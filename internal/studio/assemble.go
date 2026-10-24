package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AssemblePhotoList builds the photo-list spot with the quality bar Ninh
// demands: each photo gets a slow Ken Burns drift (no dead-static holds),
// 0.6s crossfades between photos, and an optional operator-provided track
// to -14 LUFS with fades. No text, no voiceover — Ninh's fashion format.
//
// aspect is "9:16" (1080x1920) or "16:9" (1920x1080) — the delivery frame
// comes from the job, never a package constant.
//
// When musicPath == "" the output is a silent video (no bogus audio input).
func AssemblePhotoList(ctx context.Context, photos []string, secsPer float64, musicPath string, musicStart float64, outPath, aspect string) error {
	return assemblePhotoSequence(ctx, photos, secsPer, musicPath, musicStart, outPath, aspect, photoListFilter)
}

// AssembleStillPhotoList builds a stable photo montage with soft dissolves.
// It applies no zoom, pan, shake, or beat-synced motion.
func AssembleStillPhotoList(ctx context.Context, photos []string, secsPer float64, musicPath string, musicStart float64, outPath, aspect string) error {
	return assemblePhotoSequence(ctx, photos, secsPer, musicPath, musicStart, outPath, aspect, stillPhotoListFilter)
}

func assemblePhotoSequence(ctx context.Context, photos []string, secsPer float64, musicPath string, musicStart float64, outPath, aspect string, filterBuilder func(int, float64, int, int) (string, float64)) error {
	if len(photos) == 0 {
		return fmt.Errorf("no photos")
	}
	if secsPer < 1 {
		secsPer = 4
	}
	var args []string
	for _, p := range photos {
		args = append(args, "-i", p)
	}
	w, h := AspectDims(aspect)
	filter, total := filterBuilder(len(photos), secsPer, w, h)

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

// stillPhotoListFilter holds each photo steady and dissolves between them.
func stillPhotoListFilter(n int, secsPer float64, outW, outH int) (string, float64) {
	const fade = 0.6
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb,
			"[%d:v]loop=loop=-1:size=1:start=0,scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,"+
				"crop=%d:%d,setsar=1,fps=%d,trim=duration=%.3f,setpts=PTS-STARTPTS[v%d];",
			i, outW, outH, outW, outH, outFPS, secsPer, i)
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
		switch i % 2 {
		case 0:
			zoom = fmt.Sprintf("min(1+%.6f*on,1.04)", 0.04/float64(frames))
			xExpr = "trunc(iw/2-(iw/zoom/2))"
			yExpr = "trunc(ih/2.5-(ih/zoom/2.5))"
		default:
			zoom = fmt.Sprintf("max(1.04-%.6f*on,1.0)", 0.04/float64(frames))
			xExpr = "trunc(iw/2-(iw/zoom/2))"
			yExpr = "trunc(ih/2-(ih/zoom/2))"
		}

		fmt.Fprintf(&sb,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,"+
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

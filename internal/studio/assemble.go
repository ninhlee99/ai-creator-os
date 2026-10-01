package studio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	outW   = 1080
	outH   = 1920
	outFPS = 30
)

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

// AssemblePhotoList builds the photo-list spot: each photo held still for
// secsPer seconds (TikTok photo-mode style: no motion, simple cuts), with
// the trending track mixed underneath from musicStart. No text, no
// voiceover — the format Ninh chose for fashion affiliate.
func AssemblePhotoList(ctx context.Context, photos []string, secsPer float64, musicPath string, musicStart float64, outPath string) error {
	if len(photos) == 0 {
		return fmt.Errorf("no photos")
	}
	if secsPer < 1 {
		secsPer = 4
	}
	var args []string
	var filter strings.Builder
	for i, p := range photos {
		args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.2f", secsPer), "-i", p)
		fmt.Fprintf(&filter, "[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[v%d];",
			i, outW, outH, outW, outH, i)
	}
	var concat strings.Builder
	for i := range photos {
		fmt.Fprintf(&concat, "[v%d]", i)
	}
	fmt.Fprintf(&filter, "%sconcat=n=%d:v=1:a=0[v]", concat.String(), len(photos))

	total := secsPer * float64(len(photos))
	args = append(args,
		"-i", musicPath,
		"-filter_complex", filter.String(),
		"-map", "[v]",
		"-map", fmt.Sprintf("%d:a", len(photos)),
		"-ss", fmt.Sprintf("%.1f", musicStart),
		"-t", fmt.Sprintf("%.2f", total),
		"-r", fmt.Sprint(outFPS),
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "160k",
		"-af", "afade=t=in:st=0:d=1,afade=t=out:st="+fmt.Sprintf("%.1f", total-1)+":d=1",
		"-shortest", outPath,
	)
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx, args...)
}

// ConcatClips joins per-shot clips end to end (multi-shot mode).
func ConcatClips(ctx context.Context, clips []string, outPath string) error {
	if len(clips) == 0 {
		return fmt.Errorf("no clips")
	}
	dir := filepath.Dir(outPath)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	lst := filepath.Join(dir, "concat.txt")
	var sb strings.Builder
	for _, c := range clips {
		abs, err := filepath.Abs(c)
		if err != nil {
			return err
		}
		fmt.Fprintf(&sb, "file '%s'\n", abs)
	}
	if err := os.WriteFile(lst, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	return ffmpegRun(ctx,
		"-f", "concat", "-safe", "0", "-i", lst,
		"-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", outPath)
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

// ExtractFrame grabs a still at seconds for the storyboard keyframe.
func ExtractFrame(ctx context.Context, videoPath string, at float64, outPath string) error {
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx,
		"-ss", fmt.Sprintf("%.2f", at), "-i", videoPath,
		"-frames:v", "1", "-q:v", "3", outPath)
}

// ProbeDuration returns media duration in seconds (0 on failure).
func ProbeDuration(ctx context.Context, path string) float64 {
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

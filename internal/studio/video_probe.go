package studio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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

// ProbeDuration trả về thời lượng thực tế của file video/audio (giây).
func ProbeDuration(ctx context.Context, path string) float64 {
	return probeDuration(ctx, path)
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

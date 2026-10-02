package studio

import (
	"context"
	"os/exec"
	"strings"
)

// ProbeHasAudio báo file có track audio không.
func ProbeHasAudio(ctx context.Context, path string) bool {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// Trailer dài 45–60s, cắt từ các shot trailer_worthy.

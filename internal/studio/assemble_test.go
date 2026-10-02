package studio

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBeatBounceFilter(t *testing.T) {
	filter, total := beatBounceFilter(4, 3.0, 120)
	if total != 12.0 {
		t.Fatalf("total = %v, want 12", total)
	}
	// beat bounce: zoompan with a per-beat pulse, no crossfades.
	if !strings.Contains(filter, "zoompan") {
		t.Fatal("filter missing zoompan")
	}
	if !strings.Contains(filter, "sin(2*PI*on/15.0000)") {
		t.Fatal("filter missing per-beat zoom pulse (120bpm -> 15 frames/beat)")
	}
	if strings.Contains(filter, "xfade") {
		t.Fatal("beat bounce must use hard cuts, not xfade")
	}
	if !strings.Contains(filter, "concat=n=4:v=1:a=0[vout]") {
		t.Fatal("filter missing hard-cut concat")
	}
	// shake: oscillating crop offset.
	if !strings.Contains(filter, "crop=1080:1920") {
		t.Fatal("filter missing shake crop")
	}
}

func TestBeatBounceFilterBPM(t *testing.T) {
	f120, _ := beatBounceFilter(2, 3.0, 120)
	f100, _ := beatBounceFilter(2, 3.0, 100)
	if f120 == f100 {
		t.Fatal("bpm must change the beat period")
	}
	if !strings.Contains(f100, "sin(2*PI*on/18.0000)") {
		t.Fatal("100bpm -> 18 frames/beat")
	}
}

// makeTestPNG renders a textured PNG via ffmpeg (test fixture). Solid
// colors can't reveal motion, so use pattern sources instead.
func makeTestPNG(t *testing.T, dir, name, src string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", src+"=s=1080x1920:d=1",
		"-frames:v", "1", out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return out
}

// TestAssembleBeatBounceRender is a real ffmpeg render: 3 photos x 2s at
// 120bpm, silent. Checks duration and that the bounce actually moves
// pixels between beat phases.
func TestAssembleBeatBounceRender(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	photos := []string{
		makeTestPNG(t, dir, "a.png", "testsrc"),
		makeTestPNG(t, dir, "b.png", "testsrc2"),
		makeTestPNG(t, dir, "c.png", "gradients"),
	}
	out := filepath.Join(dir, "beat.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := AssembleBeatBounce(ctx, photos, 2.0, 120, "", 0, out); err != nil {
		t.Fatalf("render: %v", err)
	}
	dur := probeDuration(ctx, out)
	if dur < 5.5 || dur > 6.5 {
		t.Fatalf("duration = %v, want ~6s", dur)
	}
	// Beat phase check: zoom peaks mid-beat (t=0.15) vs trough (t=0.4).
	// Frames must differ — otherwise the "giật" isn't happening.
	f1 := filepath.Join(dir, "f1.png")
	f2 := filepath.Join(dir, "f2.png")
	if err := extractFrame(ctx, out, 0.15, f1); err != nil {
		t.Fatal(err)
	}
	if err := extractFrame(ctx, out, 0.40, f2); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(f1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(f2)
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) == string(b2) {
		t.Fatal("frames at beat peak and trough are identical — bounce not moving")
	}
}

// TestAssembleBeatBounceMusic renders with a generated tone as the music
// bed and checks the output has an audio stream.
func TestAssembleBeatBounceMusic(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	photos := []string{makeTestPNG(t, dir, "a.png", "testsrc")}
	music := filepath.Join(dir, "tone.m4a")
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=8",
		"-c:a", "aac", music)
	if err := cmd.Run(); err != nil {
		t.Fatalf("tone: %v", err)
	}
	out := filepath.Join(dir, "beat-music.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := AssembleBeatBounce(ctx, photos, 2.0, 120, music, 0, out); err != nil {
		t.Fatalf("render: %v", err)
	}
	probe := exec.Command("ffprobe", "-v", "error", "-show_entries",
		"stream=codec_type", "-of", "csv=p=0", out)
	var sb strings.Builder
	probe.Stdout = &sb
	if err := probe.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "audio") {
		t.Fatal("output missing audio stream")
	}
}

// extractFrame/probeDuration are test-only ffmpeg helpers (moved here from
// assemble.go when the production API no longer needed them): the render
// verification tests below still need stills and durations.
func extractFrame(ctx context.Context, videoPath string, at float64, outPath string) error {
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx,
		"-ss", fmt.Sprintf("%.2f", at), "-i", videoPath,
		"-frames:v", "1", "-q:v", "3", outPath)
}

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

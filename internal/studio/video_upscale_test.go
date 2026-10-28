package studio

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoOutputDims(t *testing.T) {
	for _, tc := range []struct {
		aspect, resolution string
		width, height      int
	}{
		{"9:16", VideoOutput4K, 2160, 3840},
		{"16:9", VideoOutput4K, 3840, 2160},
		{"1:1", VideoOutput4K, 3840, 3840},
		{"9:16", VideoOutput8K, 4320, 7680},
		{"16:9", VideoOutput8K, 7680, 4320},
		{"1:1", VideoOutput8K, 7680, 7680},
		{"9:16", VideoOutput1080P, 1080, 1920},
		{"16:9", VideoOutput1080P, 1920, 1080},
		{"1:1", VideoOutput1080P, 1080, 1080},
	} {
		w, h, err := VideoOutputDims(tc.aspect, tc.resolution)
		if err != nil || w != tc.width || h != tc.height {
			t.Errorf("VideoOutputDims(%q, %q) = %dx%d, %v; want %dx%d", tc.aspect, tc.resolution, w, h, err, tc.width, tc.height)
		}
	}
	if got, err := NormalizeVideoOutputResolution(""); err != nil || got != VideoOutput1080P {
		t.Errorf("empty resolution = %q, %v; want default %q", got, err, VideoOutput1080P)
	}
}

func TestVideoMasterFilterPreservesFrame(t *testing.T) {
	filter := VideoMasterFilter(2160, 3840)
	for _, want := range []string{
		"hqdn3d=0.6:0.5:0:0",
		"force_original_aspect_ratio=decrease",
		"flags=lanczos+accurate_rnd+full_chroma_int",
		"pad=2160:3840",
		"setsar=1",
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("filter missing %q: %s", want, filter)
		}
	}
	for _, unwanted := range []string{"crop=", "zoompan", "fps="} {
		if strings.Contains(filter, unwanted) {
			t.Errorf("filter must preserve source framing and timing; found %q in %s", unwanted, filter)
		}
	}
}

func TestUpscaleVideoPreservesOutputGeometryAndDuration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is unavailable")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	output := filepath.Join(dir, "master.mp4")
	cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i",
		"testsrc2=size=360x640:rate=4:duration=1", "-an", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "30", source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create source video: %v: %s", err, output)
	}

	ctx := context.Background()
	sourceDuration := probeDuration(ctx, source)
	if err := UpscaleVideo(ctx, source, output, "9:16", VideoOutput4K); err != nil {
		t.Fatalf("UpscaleVideo: %v", err)
	}
	w, h := ProbeDims(ctx, output)
	if w != 2160 || h != 3840 {
		t.Fatalf("master = %dx%d; want 2160x3840", w, h)
	}
	if outputDuration := probeDuration(ctx, output); outputDuration <= 0 || absFloat(outputDuration-sourceDuration) > 0.15 {
		t.Fatalf("master duration = %.3fs; source = %.3fs", outputDuration, sourceDuration)
	}
}

func TestConcatClipsToMasterUpscalesSourcesOnce(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is unavailable")
	}

	dir := t.TempDir()
	clips := []string{filepath.Join(dir, "one.mp4"), filepath.Join(dir, "two.mp4")}
	for i, clip := range clips {
		cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i",
			"testsrc2=size=360x640:rate=8:duration=0.5", "-an", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "30", clip)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create source clip %d: %v: %s", i+1, err, output)
		}
	}
	master := filepath.Join(dir, "master.mp4")
	if err := ConcatClipsToMaster(context.Background(), clips, []float64{0.5, 0.5}, "9:16", VideoOutput4K, master, false); err != nil {
		t.Fatalf("ConcatClipsToMaster: %v", err)
	}
	w, h := ProbeDims(context.Background(), master)
	if w != 2160 || h != 3840 {
		t.Fatalf("master = %dx%d; want 2160x3840", w, h)
	}
	if duration := probeDuration(context.Background(), master); duration <= 0 || absFloat(duration-1.0) > 0.25 {
		t.Fatalf("master duration = %.3fs; want 1s", duration)
	}
}

func TestConcatClipsToMasterPreservesAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is unavailable")
	}

	dir := t.TempDir()
	clips := []string{filepath.Join(dir, "one.mp4"), filepath.Join(dir, "two.mp4")}
	for i, clip := range clips {
		cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i",
			"testsrc2=size=360x640:rate=8:duration=0.5", "-f", "lavfi", "-i",
			"sine=frequency=440:sample_rate=48000:duration=0.5", "-shortest", "-c:v", "libx264",
			"-preset", "ultrafast", "-crf", "30", "-c:a", "aac", "-b:a", "64k", clip)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create source clip %d: %v: %s", i+1, err, output)
		}
	}
	master := filepath.Join(dir, "master-with-audio.mp4")
	if err := ConcatClipsToMaster(context.Background(), clips, []float64{0.5, 0.5}, "9:16", VideoOutput4K, master, true); err != nil {
		t.Fatalf("ConcatClipsToMaster with audio: %v", err)
	}
	if !ProbeHasAudio(context.Background(), master) {
		t.Fatal("master lost source audio")
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

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
	filter, total := beatBounceFilter(4, 3.0, 120, "9:16")
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
	f120, _ := beatBounceFilter(2, 3.0, 120, "9:16")
	f100, _ := beatBounceFilter(2, 3.0, 100, "9:16")
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
	if err := AssembleBeatBounce(ctx, photos, 2.0, 120, "", 0, out, "9:16"); err != nil {
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
	if err := AssembleBeatBounce(ctx, photos, 2.0, 120, music, 0, out, "9:16"); err != nil {
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


// probeDims returns the (width, height) of the first video stream.
func probeDims(t *testing.T, path string) (int, int) {
	t.Helper()
	cmd := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffprobe dims: %v", err)
	}
	var w, h int
	fmt.Sscanf(strings.TrimSpace(out.String()), "%d,%d", &w, &h)
	return w, h
}

// TestAssemble16x9 is the R2-W5 acceptance test: a 16:9 job (YouTube
// long-form) must render a true 1920x1080 file — never a vertical cut
// mislabeled as long-form.
func TestAssemble16x9(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	photo := makeTestPNG(t, dir, "a.png", "testsrc")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pl := filepath.Join(dir, "long-photolist.mp4")
	if err := AssemblePhotoList(ctx, []string{photo}, 2.0, "", 0, pl, "16:9"); err != nil {
		t.Fatalf("photo-list 16:9 render: %v", err)
	}
	if w, h := probeDims(t, pl); w != 1920 || h != 1080 {
		t.Fatalf("photo-list 16:9 = %dx%d, want 1920x1080", w, h)
	}

	bb := filepath.Join(dir, "long-beat.mp4")
	if err := AssembleBeatBounce(ctx, []string{photo}, 2.0, 120, "", 0, bb, "16:9"); err != nil {
		t.Fatalf("beat-bounce 16:9 render: %v", err)
	}
	if w, h := probeDims(t, bb); w != 1920 || h != 1080 {
		t.Fatalf("beat-bounce 16:9 = %dx%d, want 1920x1080", w, h)
	}

	// 9:16 (the default for every pre-existing flow) is unchanged.
	v := filepath.Join(dir, "vert-beat.mp4")
	if err := AssembleBeatBounce(ctx, []string{photo}, 2.0, 120, "", 0, v, "9:16"); err != nil {
		t.Fatalf("beat-bounce 9:16 render: %v", err)
	}
	if w, h := probeDims(t, v); w != 1080 || h != 1920 {
		t.Fatalf("beat-bounce 9:16 = %dx%d, want 1080x1920", w, h)
	}
}

// TestAspectDims pins the aspect -> dimension contract both renderers and
// the engine path rely on. Unknown aspects fall back to vertical (the
// old behavior) instead of failing the job.
func TestAspectDims(t *testing.T) {
	if w, h := AspectDims("16:9"); w != 1920 || h != 1080 {
		t.Fatalf("16:9 = %dx%d", w, h)
	}
	for _, a := range []string{"9:16", "", "1:1", "weird"} {
		if w, h := AspectDims(a); w != 1080 || h != 1920 {
			t.Fatalf("aspect %q = %dx%d, want 1080x1920 fallback", a, w, h)
		}
	}
	if veoAspect("16:9") != "16:9" || veoAspect("") != "9:16" || veoAspect("x") != "9:16" {
		t.Fatal("veoAspect normalization wrong")
	}
}

// P1-6: ConcatClips probes every clip and normalizes mismatched frames to
// the job aspect instead of emitting a broken concat.
func TestConcatClipsNormalizesAspect(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	ctx := context.Background()
	dir := t.TempDir()
	mk := func(name string, w, h int) string {
		p := filepath.Join(dir, name)
		cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=red:s=%dx%d:d=1:r=30", w, h),
			"-pix_fmt", "yuv420p", p)
		if err := cmd.Run(); err != nil {
			t.Fatalf("make %s: %v", name, err)
		}
		return p
	}
	vertical := mk("v.mp4", 1080, 1920)
	horizontal := mk("h.mp4", 1920, 1080) // wrong frame for a 9:16 job
	out := filepath.Join(dir, "final.mp4")
	if err := ConcatClips(ctx, []string{vertical, horizontal}, "9:16", out); err != nil {
		t.Fatalf("concat: %v", err)
	}
	w, h := ProbeDims(ctx, out)
	if w != 1080 || h != 1920 {
		t.Fatalf("final = %dx%d, want 1080x1920", w, h)
	}
}

// P1-6: an unreadable clip fails loudly instead of a silent bad edit.
func TestConcatClipsBadClip(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.mp4")
	if err := os.WriteFile(bad, []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ConcatClips(context.Background(), []string{bad}, "9:16", filepath.Join(dir, "out.mp4"))
	if err == nil {
		t.Fatal("want error for unreadable clip, got nil")
	}
}

func TestFindStockClips(t *testing.T) {
	fashionClips := FindStockClips("fashion")
	if len(fashionClips) == 0 {
		t.Skip("không có clip mẫu trên đĩa (đã xoá theo yêu cầu người dùng)")
	}
	viralClips := FindStockClips("viral")
	if len(viralClips) == 0 {
		t.Skip("không có viral clip mẫu trên đĩa")
	}
}

func TestPickSingleStockClipAndTrim(t *testing.T) {
	fashionClips := FindStockClips("fashion")
	if len(fashionClips) == 0 {
		t.Skip("không có clip mẫu trên đĩa (đã xoá theo yêu cầu người dùng)")
	}
	clipDress := PickSingleStockClip("fashion", "Đầm dạ hội đen quyến rũ")
	if !strings.Contains(clipDress, "dress") {
		t.Fatalf("expected dress clip, got %s", clipDress)
	}

	clipStreet := PickSingleStockClip("fashion", "Set đồ streetwear phong cách năng động")
	if !strings.Contains(clipStreet, "streetwear") {
		t.Fatalf("expected streetwear clip, got %s", clipStreet)
	}

	clipDuo := PickSingleStockClip("viral", "Trend nhảy sexy vũ đạo đôi")
	if !strings.Contains(clipDuo, "duo") {
		t.Fatalf("expected duo dance clip, got %s", clipDuo)
	}

	clipCrew := PickSingleStockClip("viral", "Vũ đạo nhóm đường phố")
	if !strings.Contains(clipCrew, "crew") {
		t.Fatalf("expected crew dance clip, got %s", clipCrew)
	}

	// Test TrimVideo on a single clip
	dir := t.TempDir()
	out := filepath.Join(dir, "trimmed.mp4")
	if err := TrimVideo(t.Context(), clipDress, 2.0, "9:16", out); err != nil {
		t.Fatalf("TrimVideo: %v", err)
	}
	w, h := ProbeDims(t.Context(), out)
	if w != 1080 || h != 1920 {
		t.Fatalf("expected 1080x1920, got %dx%d", w, h)
	}
}

func TestMaster4KArchitecture(t *testing.T) {
	// 1. Kiểm tra tỷ lệ 4K UHD 9:16 và 16:9
	w916, h916 := AspectDims4K("9:16")
	if w916 != 2160 || h916 != 3840 {
		t.Errorf("expected 2160x3840, got %dx%d", w916, h916)
	}

	w169, h169 := AspectDims4K("16:9")
	if w169 != 3840 || h169 != 2160 {
		t.Errorf("expected 3840x2160, got %dx%d", w169, h169)
	}

	// 2. Kiểm tra bộ lọc 2 tầng chống vỡ hình (hqdn3d + lanczos + unsharp)
	f := Master4KFilter(2160, 3840)
	if !strings.Contains(f, "hqdn3d") {
		t.Errorf("Master4KFilter must contain hqdn3d denoiser")
	}
	if !strings.Contains(f, "scale=2160:3840") {
		t.Errorf("Master4KFilter must scale to 2160:3840")
	}
	if !strings.Contains(f, "lanczos+accurate_rnd") {
		t.Errorf("Master4KFilter must use lanczos+accurate_rnd")
	}
	if !strings.Contains(f, "unsharp") {
		t.Errorf("Master4KFilter must use unsharp micro-contrast mask")
	}

	// 3. Kiểm tra encoder args 4K bitrate
	args := VideoEncoderArgs4K()
	if len(args) == 0 {
		t.Errorf("VideoEncoderArgs4K cannot be empty")
	}
	foundBitrate := false
	for _, a := range args {
		if a == "35M" {
			foundBitrate = true
			break
		}
	}
	if !foundBitrate {
		t.Errorf("VideoEncoderArgs4K must specify 35M bitrate for 4K mastering")
	}
}


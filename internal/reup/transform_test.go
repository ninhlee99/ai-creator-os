package reup

// Test Đợt E: transform 2 mức bằng FFmpeg thật (sandbox có ffmpeg).
// TTS mock bằng file WAV sine tạo bởi ffmpeg — không gọi VieNeu thật.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("thiếu ffmpeg — bỏ qua test transform thật")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("thiếu ffprobe — bỏ qua test transform thật")
	}
}

// fakeTTS tạo WAV sine 4s (đủ dài cho voiceover test).
type fakeTTS struct{ dir string }

func (f fakeTTS) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text rỗng")
	}
	out := filepath.Join(f.dir, fmt.Sprintf("vo-%d.wav", time.Now().UnixNano()))
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=4",
		"-c:a", "pcm_s16le", "-ar", "24000", "-ac", "1", out)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

// makeTestClip tạo clip testsrc 5s 720x1280 (9:16).
func makeTestClip(t *testing.T, dir, name string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=720x1280:duration=5:rate=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("tạo clip test: %v", err)
	}
	return out
}

func newTestTransformer(t *testing.T, voice VoiceSynth) *Transformer {
	t.Helper()
	dir := t.TempDir()
	wd := filepath.Join(dir, "transform")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Transformer{Store: newTestStore(t), Voice: voice, WorkDir: wd}
}

func queueDownloadedVideo(t *testing.T, tr *Transformer, clip, title, author string, plays int64) Video {
	t.Helper()
	v, err := tr.Store.QueueVideo(Video{
		DouyinID: "d" + strings.TrimSuffix(filepath.Base(clip), ".mp4"),
		Title:    title, Author: author, PlayCount: plays,
	})
	if err != nil {
		t.Fatal(err)
	}
	sha := "sha-" + v.DouyinID
	if err := tr.Store.MarkDownloaded(v.ID, clip, sha, "ytdlp", false, 5, "720x1280", "ffprobe"); err != nil {
		t.Fatal(err)
	}
	v, err = tr.Store.GetVideo(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ------------------------------------------------------------ unit: filter

func TestVideoChainFilter(t *testing.T) {
	f, out := videoChainFilter(20, 2) // seed chẵn → nhanh hơn 5%
	if !strings.Contains(f, "setpts=0.952381*PTS") {
		t.Errorf("seed chẵn phải setpts nhanh 5%%: %s", f)
	}
	if got := 20 / 1.05; out-got > 0.001 || got-out > 0.001 {
		t.Errorf("outDur=%f, muốn %f", out, got)
	}
	f2, out2 := videoChainFilter(20, 3) // seed lẻ → chậm hơn 5%
	if !strings.Contains(f2, "setpts=1.052632*PTS") {
		t.Errorf("seed lẻ phải setpts chậm 5%%: %s", f2)
	}
	if got := 20 / 0.95; out2-got > 0.001 || got-out2 > 0.001 {
		t.Errorf("outDur=%f, muốn %f", out2, got)
	}
	for _, want := range []string{"crop=", "eq=", "1080", "1920", "scale="} {
		if !strings.Contains(f, want) {
			t.Errorf("filter thiếu %q: %s", want, f)
		}
	}
}

// TestVideoChainFilterDirections: 4 seed liên tiếp → 4 hướng Ken Burns
// khác nhau (fingerprint đa dạng).
func TestVideoChainFilterDirections(t *testing.T) {
	got := map[string]bool{}
	for seed := int64(0); seed < 4; seed++ {
		f, _ := videoChainFilter(20, seed)
		// Trích biểu thức zoompan để so sánh hướng.
		start := strings.Index(f, "zoompan=d=1:")
		if start < 0 {
			t.Fatalf("thiếu zoompan: %s", f)
		}
		end := strings.Index(f[start:], ",setpts=")
		got[f[start:start+end]] = true
	}
	if len(got) != 4 {
		t.Errorf("4 seed phải cho 4 hướng khác nhau, được %d", len(got))
	}
}

func TestFinalFilter(t *testing.T) {
	f, total := finalFilter(10, "Tiêu đề", 1, 2)
	want := cardDur + 10 + cardDur
	if total-want > 0.001 || want-total > 0.001 {
		t.Errorf("total=%f, muốn %f", total, want)
	}
	for _, want := range []string{"concat=n=3", "adelay=", "amix=", "loudnorm=", "drawtext="} {
		if !strings.Contains(f, want) {
			t.Errorf("final filter thiếu %q: %s", want, f)
		}
	}
	// Không voice, không nhạc → không có map audio.
	f2, _ := finalFilter(10, "", -1, -1)
	if strings.Contains(f2, "[aout]") {
		t.Errorf("không audio input mà vẫn có [aout]: %s", f2)
	}
}

// ------------------------------------------------------------ unit: text

func TestTemplateCommentaryHonest(t *testing.T) {
	s := templateCommentary([]Video{{Title: "Tiên kiếm", Author: "than_tien", PlayCount: 1500000}}, 1)
	for _, want := range []string{"Tiên kiếm", "than_tien", "1500000"} {
		if !strings.Contains(s, want) {
			t.Errorf("commentary thiếu sự thật %q: %s", want, s)
		}
	}
	// Không bịa tình tiết.
	for _, bad := range []string{"kiếm khách", "đánh nhau", "cứu"} {
		if strings.Contains(s, bad) {
			t.Errorf("commentary bịa tình tiết %q: %s", bad, s)
		}
	}
	// Metadata trống → vẫn có câu hợp lệ.
	s2 := templateCommentary([]Video{{}}, 2)
	if strings.TrimSpace(s2) == "" {
		t.Errorf("metadata trống vẫn phải có lời bình")
	}
	s3 := templateCommentary([]Video{{}, {}, {}}, 3)
	if !strings.Contains(s3, "3") {
		t.Errorf("compilation phải nhắc số clip: %s", s3)
	}
}

// TestTemplateCommentaryRotates: seed khác nhau → biến thể khác nhau
// (không đọc cùng một câu cho mọi video).
func TestTemplateCommentaryRotates(t *testing.T) {
	v := []Video{{Title: "T", Author: "A"}}
	seen := map[string]bool{}
	for seed := int64(0); seed < 6; seed++ {
		seen[templateCommentary(v, seed)] = true
	}
	if len(seen) < 3 {
		t.Errorf("phải có ≥3 biến thể theo seed, được %d", len(seen))
	}
}

func TestSanitizeCommentary(t *testing.T) {
	s := sanitizeCommentary("  chào\n\nbạn  ")
	if s != "chào bạn" {
		t.Errorf("sanitize sai: %q", s)
	}
	long := strings.Repeat("a", 500)
	if got := len([]rune(sanitizeCommentary(long))); got > 300 {
		t.Errorf("phải cắt 300 ký tự, được %d", got)
	}
}

func TestBuildSRT(t *testing.T) {
	srt := buildSRT("Xin chào", 2, 5.5)
	if !strings.Contains(srt, "00:00:02,000 --> 00:00:05,500") {
		t.Errorf("SRT time sai: %q", srt)
	}
	if !strings.Contains(srt, "Xin chào") {
		t.Errorf("SRT thiếu text: %q", srt)
	}
	if buildSRT("  ", 0, 1) != "" {
		t.Errorf("text rỗng phải trả SRT rỗng")
	}
}

// ------------------------------------------------------------ ffmpeg thật

func TestTransformVideoLevel1(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dir := t.TempDir()
	tr := newTestTransformer(t, fakeTTS{dir: dir})
	clip := makeTestClip(t, dir, "a.mp4")
	v := queueDownloadedVideo(t, tr, clip, "Tiên kiếm kỳ hiệp", "than_tien", 1500000)

	out, err := tr.TransformVideo(ctx, v, TransformOptions{
		Level: 1, Seed: v.ID, Title: v.Title, Voiceover: true,
	})
	if err != nil {
		t.Fatalf("TransformVideo: %v", err)
	}
	// Output 9:16 + có audio + dài > 3s (QC đã chạy trong TransformVideo).
	w, h := studio.ProbeDims(ctx, out)
	if w != 1080 || h != 1920 {
		t.Errorf("output %dx%d, muốn 1080x1920", w, h)
	}
	dur, err := qcTransform(out)
	if err != nil {
		t.Fatalf("qcTransform output: %v", err)
	}
	// 5s nhanh 5% + 2 card 1.2s ≈ 7.16s.
	if dur < 6 || dur > 9 {
		t.Errorf("dur=%.2f, muốn khoảng 7.2s", dur)
	}
}

func TestTransformVideoVoiceoverFailClosed(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	// Voice=nil + Voiceover=true → fail-closed, không tạo video câm.
	tr := newTestTransformer(t, nil)
	clip := makeTestClip(t, dir, "a.mp4")
	v := queueDownloadedVideo(t, tr, clip, "T", "a", 0)
	if _, err := tr.TransformVideo(ctx, v, TransformOptions{Level: 1, Seed: 1, Voiceover: true}); err == nil {
		t.Fatalf("thiếu TTS mà voiceover bật phải lỗi (fail-closed)")
	}
}

func TestQCTransformFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	// File rỗng → rớt.
	empty := filepath.Join(dir, "empty.mp4")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := qcTransform(empty); err == nil {
		t.Errorf("file rỗng phải rớt QC")
	}
	// Video không audio → rớt (thiếu voiceover/nhạc).
	noaudio := filepath.Join(dir, "noaudio.mp4")
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:duration=5:rate=10",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-an", noaudio)
	if err := cmd.Run(); err != nil {
		t.Fatalf("tạo clip: %v", err)
	}
	if _, err := qcTransform(noaudio); err == nil {
		t.Errorf("video thiếu audio phải rớt QC")
	}
	// File không tồn tại → rớt.
	if _, err := qcTransform(filepath.Join(dir, "nope.mp4")); err == nil {
		t.Errorf("file không tồn tại phải rớt QC")
	}
}

func TestTransformCompilationLevel2(t *testing.T) {
	requireFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	dir := t.TempDir()
	tr := newTestTransformer(t, fakeTTS{dir: dir})
	var vs []Video
	for i, name := range []string{"a.mp4", "b.mp4", "c.mp4"} {
		clip := makeTestClip(t, dir, name)
		vs = append(vs, queueDownloadedVideo(t, tr, clip,
			fmt.Sprintf("Clip %d", i+1), "than_tien", int64(1000*(i+1))))
	}
	out, err := tr.TransformCompilation(ctx, vs, TransformOptions{
		Level: 2, Seed: 7, Title: "Thần Tiên", Voiceover: true,
	})
	if err != nil {
		t.Fatalf("TransformCompilation: %v", err)
	}
	w, h := studio.ProbeDims(ctx, out)
	if w != 1080 || h != 1920 {
		t.Errorf("output %dx%d, muốn 1080x1920", w, h)
	}
	dur, err := qcTransform(out)
	if err != nil {
		t.Fatalf("qcTransform compilation: %v", err)
	}
	// 3 clip 5s (seed lẻ/chẵn) - 2 xfade 0.5s + 2 card ≈ 15-16s.
	if dur < 12 || dur > 20 {
		t.Errorf("dur=%.2f, muốn khoảng 15s", dur)
	}
	// Sai số lượng clip → lỗi rõ ràng.
	if _, err := tr.TransformCompilation(ctx, vs[:2], TransformOptions{Level: 2}); err == nil {
		t.Errorf("compilation 2 clip phải lỗi")
	}
}

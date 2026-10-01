package engines

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubFilmLLM struct {
	text string
	err  error
}

func (s stubFilmLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	return s.text, s.err
}
func (s stubFilmLLM) Name() string                     { return "stub" }
func (s stubFilmLLM) Healthy(ctx context.Context) bool { return true }

// writeTestWav writes a minimal 24kHz mono s16le WAV.
func writeTestWav(t *testing.T, path string, seconds float64) {
	t.Helper()
	n := int(24000*seconds) * 2
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+n))
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 1)
	binary.LittleEndian.PutUint32(hdr[24:], 24000)
	binary.LittleEndian.PutUint32(hdr[28:], 24000*2)
	binary.LittleEndian.PutUint16(hdr[32:], 2)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(n))
	data := append(hdr, make([]byte, n)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSRT(t *testing.T) {
	got := BuildSRT([]string{"Chào bạn", "Tạm biệt"}, []float64{2.5, 3.0})
	want := "1\n00:00:00,000 --> 00:00:02,500\nChào bạn\n\n" +
		"2\n00:00:02,500 --> 00:00:05,500\nTạm biệt\n\n"
	if got != want {
		t.Fatalf("SRT mismatch:\n%s", got)
	}
	// missing duration entries default to 0
	got = BuildSRT([]string{"a", "b"}, []float64{1})
	if !strings.Contains(got, "00:00:01,000 --> 00:00:01,000\nb") {
		t.Fatalf("zero-duration fallback wrong:\n%s", got)
	}
}

func TestWavSeconds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.wav")
	writeTestWav(t, p, 2.5)
	d, err := WavSeconds(p)
	if err != nil {
		t.Fatalf("WavSeconds: %v", err)
	}
	if d < 2.49 || d > 2.51 {
		t.Fatalf("d = %f, want ~2.5", d)
	}
	if _, err := WavSeconds(filepath.Join(t.TempDir(), "nope.wav")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPlanFilm(t *testing.T) {
	llm := stubFilmLLM{text: "```json\n" + `{
  "title": "Bí quyết 5 giây",
  "scenes": [
    {"narration": "Mở đầu hấp dẫn", "image_prompt": "a sunrise", "seconds": 3},
    {"narration": "", "image_prompt": "skipped", "seconds": 2},
    {"narration": "Kết thúc", "image_prompt": "", "seconds": 4}
  ]
}` + "\n```"}
	plan, err := PlanFilm(context.Background(), llm, "chủ đề test", 60, 6)
	if err != nil {
		t.Fatalf("PlanFilm: %v", err)
	}
	if plan.Title != "Bí quyết 5 giây" {
		t.Fatalf("title = %q", plan.Title)
	}
	if len(plan.Scenes) != 2 {
		t.Fatalf("scenes = %d, want 2 (empty narration dropped)", len(plan.Scenes))
	}
	if plan.Scenes[0].ImagePrompt != "a sunrise" || plan.Scenes[0].Seconds != 3 {
		t.Fatalf("scene0 = %+v", plan.Scenes[0])
	}
	// maxScenes cap
	plan2, err := PlanFilm(context.Background(), llm, "x", 60, 1)
	if err != nil {
		t.Fatalf("PlanFilm: %v", err)
	}
	if len(plan2.Scenes) != 1 {
		t.Fatalf("scenes = %d, want capped at 1", len(plan2.Scenes))
	}
	// empty title falls back to topic
	llm2 := stubFilmLLM{text: `{"scenes":[{"narration":"x","seconds":5}]}`}
	plan3, err := PlanFilm(context.Background(), llm2, "chủ đề", 60, 6)
	if err != nil || plan3.Title != "chủ đề" {
		t.Fatalf("title fallback: %q err=%v", plan3.Title, err)
	}
}

func TestPlanFilmBadJSON(t *testing.T) {
	llm := stubFilmLLM{text: "no json here"}
	if _, err := PlanFilm(context.Background(), llm, "x", 60, 6); err == nil {
		t.Fatal("expected JSON error")
	}
}

func TestRenderScene(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	img := filepath.Join(dir, "slide.png")
	if err := exec.Command("ffmpeg", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=red:s=640x1136:d=2",
		"-frames:v", "1", img).Run(); err != nil {
		t.Fatalf("make image: %v", err)
	}
	wav := filepath.Join(dir, "nar.wav")
	writeTestWav(t, wav, 2.0)
	out := filepath.Join(dir, "scene.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := RenderScene(ctx, img, wav, 2.0, out); err != nil {
		t.Fatalf("RenderScene: %v", err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatalf("out missing/empty: %v", err)
	}
}

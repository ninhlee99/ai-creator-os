package studio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

type fakeStoryLLM struct{ scenes int }

func (f *fakeStoryLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	if strings.Contains(prompt, `"scenes"`) || strings.Contains(prompt, "Chia truyện") {
		n := f.scenes
		if n < 3 {
			n = 3
		}
		var parts []string
		for i := 0; i < n; i++ {
			parts = append(parts, fmt.Sprintf(
				`{"text": "Cảnh %d: tôi bước vào căn phòng tối và nghe tiếng mưa rơi ngoài hiên.", "image_prompt": "dark room interior, rain on window, cinematic light"}`,
				i+1))
		}
		return `{"scenes": [` + strings.Join(parts, ",") + `]}`, nil
	}
	return `{"title": "Mưa đêm", "logline": "Một đêm mưa thay đổi đời tôi.", "text": "Tôi chưa bao giờ quên đêm mưa ấy. Tôi bước vào căn phòng tối."}`, nil
}

func (f *fakeStoryLLM) Name() string                     { return "fake" }
func (f *fakeStoryLLM) Healthy(ctx context.Context) bool { return true }

// fakeStoryMG vẽ ảnh bằng ffmpeg color source (cần ffmpeg thật).
type fakeStoryMG struct{}

func (fakeStoryMG) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	return ffmpegRun(ctx, "-f", "lavfi", "-i", "color=c=0x2b2f4a:s=1920x1080:d=1",
		"-frames:v", "1", outPath)
}
func (fakeStoryMG) GenerateVideo(ctx context.Context, prompt, firstFrame string, seconds int, aspect, outPath string) error {
	return fmt.Errorf("fake: no video")
}
func (fakeStoryMG) Name() string                     { return "fake" }
func (fakeStoryMG) Healthy(ctx context.Context) bool { return true }

// silenceWAV tạo wav câm dài sec giây (8000Hz mono 16-bit).
func silenceWAV(sec int) []byte {
	n := 8000 * sec
	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+2*n))
	buf.WriteString("WAVEfmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint32(8000))
	binary.Write(buf, binary.LittleEndian, uint32(16000))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(2*n))
	buf.Write(make([]byte, 2*n))
	return buf.Bytes()
}

func ffmpegPresent() bool {
	return exec.Command("ffmpeg", "-version").Run() == nil &&
		exec.Command("ffprobe", "-version").Run() == nil
}

func waitStoryDone(t *testing.T, st *Studio, id string) Job {
	t.Helper()
	// Pipeline render FFmpeg thật (~80s khi chạy riêng); nới timeout để
	// không flake khi máy tải nặng (VD: chạy full suite song song).
	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		j, ok := st.GetJob(id)
		if !ok {
			t.Fatalf("job %s biến mất", id)
		}
		if j.Status == StatusDone || j.Status == StatusFailed {
			return j
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("job %s không xong sau 240s", id)
	return Job{}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestCreateStoryJobValidation(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.CreateStoryJob(StoryParams{}); err == nil {
		t.Fatal("chủ đề rỗng phải lỗi")
	}
}

func TestStoryJobsFilter(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.insertJob(KindAffiliate, "aff", AffiliateParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.insertJob(KindStory, "story", StoryParams{}); err != nil {
		t.Fatal(err)
	}
	jobs := st.StoryJobs(10)
	if len(jobs) != 1 || jobs[0].Kind != KindStory {
		t.Fatalf("StoryJobs lọc sai: %+v", jobs)
	}
}

func TestBuildSRT(t *testing.T) {
	scenes := []storyScene{{Text: "Một."}, {Text: "Hai."}}
	srt := buildSRT(scenes, []float64{2.5, 3.0})
	if !strings.Contains(srt, "00:00:00,000 --> 00:00:02,500") {
		t.Fatalf("timing cảnh 1 sai:\n%s", srt)
	}
	if !strings.Contains(srt, "00:00:02,500 --> 00:00:05,500") {
		t.Fatalf("timing cảnh 2 sai:\n%s", srt)
	}
}

func TestSplitScenesFake(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), &fakeStoryLLM{scenes: 5}, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	scenes, err := st.splitScenes(context.Background(),
		StoryParams{Scenes: 5}, &storyDoc{Title: "T", Text: "Truyện mẫu."})
	if err != nil {
		t.Fatal(err)
	}
	if len(scenes) != 5 {
		t.Fatalf("got %d cảnh, want 5", len(scenes))
	}
	for _, sc := range scenes {
		if sc.Text == "" || sc.ImagePrompt == "" {
			t.Fatalf("cảnh thiếu text/image_prompt: %+v", sc)
		}
	}
}

// Pipeline đầy đủ với provider fake + ffmpeg thật: 3 cảnh × 12s = 36s > 30s QC.
func TestStoryPipelineSuccess(t *testing.T) {
	if !ffmpegPresent() {
		t.Skip("thiếu ffmpeg/ffprobe")
	}
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	narr := Narrator(func(ctx context.Context, text string) ([]byte, error) {
		return silenceWAV(12), nil
	})
	st, err := New(filepath.Join(dir, "studio.db"), &fakeStoryLLM{scenes: 3}, fakeStoryMG{}, narr, outDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateStoryJob(StoryParams{Topic: "mưa đêm", Genre: "tâm lý", Words: 450, Scenes: 3})
	if err != nil {
		t.Fatal(err)
	}
	j := waitStoryDone(t, st, id)
	if j.Status != StatusDone {
		t.Fatalf("status=%s log:\n%s", j.Status, j.Log)
	}
	if j.Output == "" {
		t.Fatal("thiếu output")
	}
	w, h := ProbeDims(context.Background(), j.Output)
	if w != 1920 || h != 1080 {
		t.Fatalf("khổ %dx%d, cần 1920x1080", w, h)
	}
	if !ProbeHasAudio(context.Background(), j.Output) {
		t.Fatal("thiếu audio")
	}
	if d := storyProbeDuration(context.Background(), j.Output); d < 30 {
		t.Fatalf("dài %.1fs, cần > 30s", d)
	}
	// Phụ đề phải được mux (mov_text).
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "s",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", j.Output)
	if out, _ := cmd.Output(); !strings.Contains(string(out), "mov_text") {
		t.Fatalf("thiếu track phụ đề mov_text: %q", string(out))
	}
	if len(st.ListAssets(id)) != 3 {
		t.Fatalf("assets = %d, want 3", len(st.ListAssets(id)))
	}
}

// QC rớt khi video quá ngắn: 3 cảnh × 2s = 6s < 30s → failed + lý do.
func TestStoryPipelineQCFail(t *testing.T) {
	if !ffmpegPresent() {
		t.Skip("thiếu ffmpeg/ffprobe")
	}
	dir := t.TempDir()
	narr := Narrator(func(ctx context.Context, text string) ([]byte, error) {
		return silenceWAV(2), nil
	})
	st, err := New(filepath.Join(dir, "studio.db"), &fakeStoryLLM{scenes: 3}, fakeStoryMG{}, narr, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateStoryJob(StoryParams{Topic: "ngắn", Words: 450, Scenes: 3})
	if err != nil {
		t.Fatal(err)
	}
	j := waitStoryDone(t, st, id)
	if j.Status != StatusFailed {
		t.Fatalf("status=%s, cần failed (QC)", j.Status)
	}
	if !strings.Contains(j.Log, "QC") {
		t.Fatalf("log thiếu lý do QC:\n%s", j.Log)
	}
}

// TTS chết → fail-closed, không tạo video câm.
func TestStoryPipelineTTSError(t *testing.T) {
	if !ffmpegPresent() {
		t.Skip("thiếu ffmpeg/ffprobe")
	}
	dir := t.TempDir()
	narr := Narrator(func(ctx context.Context, text string) ([]byte, error) {
		return nil, fmt.Errorf("tts down")
	})
	st, err := New(filepath.Join(dir, "studio.db"), &fakeStoryLLM{scenes: 3}, fakeStoryMG{}, narr, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateStoryJob(StoryParams{Topic: "câm", Words: 450, Scenes: 3})
	if err != nil {
		t.Fatal(err)
	}
	j := waitStoryDone(t, st, id)
	if j.Status != StatusFailed {
		t.Fatalf("status=%s, cần failed", j.Status)
	}
	if j.Output != "" {
		t.Fatal("không được có output khi TTS lỗi")
	}
}

// Image gen chết → fail-closed, không dựng thiếu cảnh.
func TestStoryPipelineImageError(t *testing.T) {
	if !ffmpegPresent() {
		t.Skip("thiếu ffmpeg/ffprobe")
	}
	dir := t.TempDir()
	narr := Narrator(func(ctx context.Context, text string) ([]byte, error) {
		return silenceWAV(12), nil
	})
	st, err := New(filepath.Join(dir, "studio.db"), &fakeStoryLLM{scenes: 3},
		&brokenMG{}, narr, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateStoryJob(StoryParams{Topic: "mù", Words: 450, Scenes: 3})
	if err != nil {
		t.Fatal(err)
	}
	j := waitStoryDone(t, st, id)
	if j.Status != StatusFailed || j.Output != "" {
		t.Fatalf("status=%s output=%q, cần failed không output", j.Status, j.Output)
	}
}

type brokenMG struct{ fakeStoryMG }

func (brokenMG) GenerateImage(ctx context.Context, prompt string, refs []ImageRef, outPath string) error {
	return fmt.Errorf("image gen down")
}

func TestDeleteStoryJob(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	// Job story thật (sẽ fail ở pipeline vì không có providers, nhưng row
	// đã được tạo với kind=story).
	id, err := st.CreateStoryJob(StoryParams{Topic: "xoá tôi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteStoryJob(id); err != nil {
		t.Fatalf("DeleteStoryJob: %v", err)
	}
	if _, ok := st.GetJob(id); ok {
		t.Fatal("job vẫn còn sau khi xoá")
	}
	// Xoá job không tồn tại → lỗi.
	if err := st.DeleteStoryJob("khong-co"); err == nil {
		t.Fatal("xoá job không tồn tại phải lỗi")
	}
	// Xoá job kind khác → bị chặn.
	other, err := st.insertJob(KindAffiliate, "aff", nil)
	if err != nil {
		t.Fatalf("insertJob affiliate: %v", err)
	}
	if err := st.DeleteStoryJob(other); err == nil {
		t.Fatal("xoá job không phải story phải bị chặn")
	}
	if _, ok := st.GetJob(other); !ok {
		t.Fatal("job affiliate bị xoá nhầm")
	}
}

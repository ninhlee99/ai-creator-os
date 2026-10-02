package engines

// AI short-film factory — port of agents/content/film.py.
//
// A "short film" = 60-180s vertical narrative video:
// LLM script -> scene breakdown -> image per scene (injected imageFn) ->
// Ken Burns motion -> TTS voiceover -> subtitles -> 1080x1920 mp4.
//
// Fully assembled with FFmpeg (no GPU needed). The imageFn is injected: in
// production it calls an image model; in tests it renders color slides.
// Nothing here pretends to be Sora/Veo-class video generation — this is the
// honest, shippable v1: illustrated narration films.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
)

const (
	filmW   = 1080
	filmH   = 1920
	filmFPS = 30
)

// filmDims maps an aspect to scene render dimensions ("16:9" ->
// 1920x1080, anything else -> 1080x1920).
func filmDims(aspect string) (w, h int) {
	if aspect == "16:9" {
		return 1920, 1080
	}
	return filmW, filmH
}

// FilmScene is one scene of a film plan.
type FilmScene struct {
	Narration   string `json:"narration"`
	ImagePrompt string `json:"image_prompt"`
	Seconds     int    `json:"seconds"`
}

// FilmPlan is the LLM-generated scene-by-scene plan.
type FilmPlan struct {
	Title  string      `json:"title"`
	Scenes []FilmScene `json:"scenes"`
}

// FilmResult is the outcome of MakeFilm.
type FilmResult struct {
	Path    string
	Title   string
	Seconds float64
	Scenes  int
}

// ImageFunc renders an image for prompt into outPath (PNG).
// Production: an image model. Tests: a color slide.
type ImageFunc func(ctx context.Context, prompt, outPath string) error

// PlanFilm asks the LLM for a scene-by-scene film plan in Vietnamese.
func PlanFilm(ctx context.Context, llm LLMProvider, topic string, targetSeconds, maxScenes int) (FilmPlan, error) {
	n := targetSeconds / 15
	if n < 2 {
		n = 2
	}
	if n > maxScenes {
		n = maxScenes
	}
	if n < 2 {
		n = 2
	}
	text, err := llm.Complete(ctx,
		"Bạn viết kịch bản phim ngắn dọc TikTok, tiếng Việt. "+
			"Chỉ trả lời JSON thuần: "+
			`{"title": "...", "scenes": [{"narration": "lời dẫn khoảng 20-30 từ", `+
			`"image_prompt": "mô tả hình ảnh minh hoạ bằng tiếng Anh", "seconds": 15}]}}.`,
		fmt.Sprintf("Chủ đề phim: %s. Viết kịch bản %d cảnh, tổng khoảng %d giây, "+
			"có mở đầu gây tò mò và kết thúc đọng lại.", topic, n, targetSeconds),
	)
	if err != nil {
		return FilmPlan{}, fmt.Errorf("LLM failed: %w", err)
	}
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "json"))
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var plan FilmPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &plan); err != nil {
		return FilmPlan{}, fmt.Errorf("bad film plan JSON: %w", err)
	}
	scenes := make([]FilmScene, 0, len(plan.Scenes))
	for _, s := range plan.Scenes {
		if strings.TrimSpace(s.Narration) != "" {
			scenes = append(scenes, s)
		}
		if len(scenes) >= maxScenes {
			break
		}
	}
	if len(scenes) == 0 {
		return FilmPlan{}, fmt.Errorf("LLM returned no scenes")
	}
	title := plan.Title
	if title == "" {
		title = topic
	}
	if len(title) > 100 {
		title = title[:100]
	}
	return FilmPlan{Title: title, Scenes: scenes}, nil
}

// WavSeconds returns the duration of a WAV file in seconds.
func WavSeconds(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	rate, ch, bits, err := wavParams(data)
	if err != nil {
		return 0, err
	}
	dataLen, err := wavDataLen(data)
	if err != nil {
		return 0, err
	}
	if rate == 0 || ch == 0 || bits == 0 {
		rate = 24000
	}
	bytesPerSec := float64(rate*ch*bits) / 8
	if bytesPerSec == 0 {
		return 0, fmt.Errorf("bad WAV params")
	}
	return float64(dataLen) / bytesPerSec, nil
}

// wavParams parses a WAV header: sample rate, channels, bits per sample.
func wavParams(wav []byte) (sampleRate, channels, bits int, err error) {
	if len(wav) < 44 {
		return 0, 0, 0, fmt.Errorf("too short for WAV header")
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return 0, 0, 0, fmt.Errorf("not a WAV file")
	}
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		size := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "fmt " {
			if off+24 > len(wav) {
				return 0, 0, 0, fmt.Errorf("truncated fmt chunk")
			}
			channels = int(binary.LittleEndian.Uint16(wav[off+10 : off+12]))
			sampleRate = int(binary.LittleEndian.Uint32(wav[off+12 : off+16]))
			bits = int(binary.LittleEndian.Uint16(wav[off+22 : off+24]))
			return sampleRate, channels, bits, nil
		}
		off += 8 + size
		if size%2 == 1 {
			off++
		}
	}
	return 0, 0, 0, fmt.Errorf("fmt chunk not found")
}

// wavDataLen returns the size of the WAV data chunk.
func wavDataLen(wav []byte) (int, error) {
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		size := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "data" {
			return size, nil
		}
		off += 8 + size
		if size%2 == 1 {
			off++
		}
	}
	return 0, fmt.Errorf("data chunk not found")
}

// SynthNarration synthesizes text and writes the WAV file to outPath.
func SynthNarration(ctx context.Context, t tts.TTSProvider, text, voice, outPath string) (string, error) {
	wav, err := t.Synthesize(ctx, text, voice)
	if err != nil {
		return "", fmt.Errorf("TTS failed: %w", err)
	}
	if err := os.WriteFile(outPath, wav, 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}

func ffmpegRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := stderr.String()
		if len(out) > 400 {
			out = out[len(out)-400:]
		}
		return fmt.Errorf("ffmpeg failed: %v: %s", err, out)
	}
	return nil
}

// RenderScene builds a scene mp4: still image + Ken Burns slow zoom +
// narration audio. aspect is "9:16" (1080x1920) or "16:9" (1920x1080).
func RenderScene(ctx context.Context, imagePath, audioPath string, seconds float64, aspect, outPath string) error {
	dur := seconds
	if dur < 1.0 {
		dur = 1.0
	}
	fw, fh := filmDims(aspect)
	return ffmpegRun(ctx,
		"-loop", "1", "-framerate", fmt.Sprint(filmFPS), "-i", imagePath,
		"-i", audioPath,
		"-filter_complex",
		fmt.Sprintf("[0:v]scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d,"+
			"zoompan=z='min(zoom+0.0012,1.25)':d=1:"+
			"x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':"+
			"s=%dx%d:fps=%d[v]", fw, fh, fw, fh, fw, fh, filmFPS),
		"-map", "[v]", "-map", "1:a",
		"-t", fmt.Sprintf("%.2f", dur),
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", outPath)
}

func srtTime(sec float64) string {
	ms := int(sec * 1000)
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// BuildSRT builds subtitles: one entry per scene, timed to the narration.
func BuildSRT(narrations []string, durations []float64) string {
	var sb strings.Builder
	t := 0.0
	for i := range narrations {
		d := 0.0
		if i < len(durations) {
			d = durations[i]
		}
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(t), srtTime(t+d), strings.TrimSpace(narrations[i]))
		t += d
	}
	return sb.String()
}

// AssembleFilm concats scene mp4s and muxes subtitles -> final vertical film.
func AssembleFilm(ctx context.Context, scenes []string, srtText, outPath string) error {
	dir := filepath.Dir(outPath)
	lst := filepath.Join(dir, "concat.txt")
	var sb strings.Builder
	for _, s := range scenes {
		abs, err := filepath.Abs(s)
		if err != nil {
			return err
		}
		fmt.Fprintf(&sb, "file '%s'\n", abs)
	}
	if err := os.WriteFile(lst, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	srt := filepath.Join(dir, "subs.srt")
	if err := os.WriteFile(srt, []byte(srtText), 0o644); err != nil {
		return err
	}
	return ffmpegRun(ctx,
		"-f", "concat", "-safe", "0", "-i", lst,
		"-i", srt,
		"-c:v", "copy", "-c:a", "aac", "-c:s", "mov_text",
		"-metadata:s:s:0", "language=vie",
		outPath)
}

// MakeFilm is the end-to-end pipeline: plan -> images -> narration ->
// scenes -> final mp4. Returns path, title, total seconds, scene count.
func MakeFilm(ctx context.Context, topic string, llm LLMProvider, t tts.TTSProvider, imageFn ImageFunc, workdir, voice string, targetSeconds int) (FilmResult, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return FilmResult{}, err
	}
	plan, err := PlanFilm(ctx, llm, topic, targetSeconds, 6)
	if err != nil {
		return FilmResult{}, err
	}
	var scenes, narrations []string
	var durations []float64
	for i, sc := range plan.Scenes {
		img := filepath.Join(workdir, fmt.Sprintf("scene%d.png", i))
		imgPrompt := sc.ImagePrompt
		if imgPrompt == "" {
			imgPrompt = topic // fall back to topic when the LLM skipped the prompt
		}
		if err := imageFn(ctx, imgPrompt, img); err != nil {
			return FilmResult{}, fmt.Errorf("image scene %d: %w", i, err)
		}
		wavPath := filepath.Join(workdir, fmt.Sprintf("scene%d.wav", i))
		if _, err := SynthNarration(ctx, t, sc.Narration, voice, wavPath); err != nil {
			return FilmResult{}, err
		}
		wavSec, err := WavSeconds(wavPath)
		if err != nil {
			return FilmResult{}, fmt.Errorf("wav scene %d: %w", i, err)
		}
		dur := float64(sc.Seconds)
		if wavSec > dur {
			dur = wavSec
		}
		mp4 := filepath.Join(workdir, fmt.Sprintf("scene%d.mp4", i))
		if err := RenderScene(ctx, img, wavPath, dur, "9:16", mp4); err != nil {
			return FilmResult{}, err
		}
		scenes = append(scenes, mp4)
		narrations = append(narrations, sc.Narration)
		durations = append(durations, dur)
	}
	out := filepath.Join(workdir, "film.mp4")
	if err := AssembleFilm(ctx, scenes, BuildSRT(narrations, durations), out); err != nil {
		return FilmResult{}, err
	}
	total := 0.0
	for _, d := range durations {
		total += d
	}
	return FilmResult{Path: out, Title: plan.Title, Seconds: total, Scenes: len(scenes)}, nil
}

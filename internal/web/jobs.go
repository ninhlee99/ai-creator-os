package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Job is one kinetic-typography video render request. Port of the Python
// content-job dict stored in data/content_jobs.json.
type Job struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Captions   []string `json:"captions"`
	Narration  string   `json:"narration"`
	Status     string   `json:"status"` // queued | running | done | failed
	CreatedAt  string   `json:"created_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	Output     string   `json:"output,omitempty"`
	Log        string   `json:"log"`
}

// NewJobID returns an 8-hex-char job id (port of uuid4().hex[:8]).
func NewJobID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

// JobStore is the Go port of _load_jobs/_save_jobs: a JSON file guarded by
// a mutex, written atomically (tmp file + rename).
type JobStore struct {
	mu   sync.Mutex
	path string
	jobs []Job
}

// NewJobStore opens (or creates) the job file at path.
func NewJobStore(path string) *JobStore {
	js := &JobStore{path: path}
	js.jobs = js.load()
	return js
}

func (js *JobStore) load() []Job {
	b, err := os.ReadFile(js.path)
	if err != nil {
		return nil
	}
	var jobs []Job
	if err := json.Unmarshal(b, &jobs); err != nil {
		return nil
	}
	return jobs
}

func (js *JobStore) saveLocked() {
	if dir := filepath.Dir(js.path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	b, err := json.MarshalIndent(js.jobs, "", " ")
	if err != nil {
		return
	}
	tmp := js.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, js.path)
}

// All returns a snapshot of all jobs, newest first.
func (js *JobStore) All() []Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	out := make([]Job, len(js.jobs))
	copy(out, js.jobs)
	return out
}

// Add inserts a job at the front (newest first) and persists.
func (js *JobStore) Add(j Job) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.jobs = append([]Job{j}, js.jobs...)
	js.saveLocked()
}

// Get returns a copy of the job with id, or false when missing.
func (js *JobStore) Get(id string) (Job, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.jobs {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}

// Update mutates the job with id in place and persists.
func (js *JobStore) Update(id string, fn func(*Job)) {
	js.mu.Lock()
	defer js.mu.Unlock()
	for i := range js.jobs {
		if js.jobs[i].ID == id {
			fn(&js.jobs[i])
			js.saveLocked()
			return
		}
	}
}

// AppendLog appends one line to the job's log and persists.
func (js *JobStore) AppendLog(id, line string) {
	js.Update(id, func(j *Job) {
		if j.Log != "" {
			j.Log += "\n"
		}
		j.Log += line
	})
}

// Count returns the number of stored jobs.
func (js *JobStore) Count() int {
	js.mu.Lock()
	defer js.mu.Unlock()
	return len(js.jobs)
}

// RenderResult is the outcome of rendering one job.
type RenderResult struct {
	Output   string  // file name under the output dir
	Seconds  float64 // video duration
	HasAudio bool    // narration track was synthesized
}

// Renderer renders a content job into a video file. The default
// implementation (ffmpegRenderer) shells out to ffmpeg directly; the
// wiring worker may swap in engines.film later without touching the web
// layer.
type Renderer interface {
	Render(ctx context.Context, job Job, tts TTSChainAPI, logf func(string)) (RenderResult, error)
}

// ffmpegRenderer is the direct Go port of Python's _render_kinetic:
// 1080x1920 kinetic-typography video from captions (+optional AI voice),
// blurred background image or dark color, per-caption drawtext, progress
// bar and the "AI CREATOR OS" watermark.
type ffmpegRenderer struct {
	s *Server
}

func (r ffmpegRenderer) Render(ctx context.Context, job Job, tts TTSChainAPI, logf func(string)) (RenderResult, error) {
	outDir := r.s.OutDir
	work := filepath.Join(outDir, job.ID)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return RenderResult{}, err
	}

	captions := make([]string, 0, len(job.Captions))
	for _, c := range job.Captions {
		if strings.TrimSpace(c) != "" {
			captions = append(captions, c)
		}
	}
	if len(captions) == 0 {
		captions = []string{job.Title}
	}
	narration := strings.TrimSpace(job.Narration)

	var audioPath string
	if narration != "" {
		logf("Đang tạo lồng tiếng AI…")
		if tts != nil {
			wav, err := tts.Synthesize(ctx, narration, "default")
			if err == nil && len(wav) > 0 {
				audioPath = filepath.Join(work, "vo.wav")
				if err := os.WriteFile(audioPath, wav, 0o644); err != nil {
					audioPath = ""
				}
			}
		}
		if audioPath == "" {
			logf("Không có TTS khả dụng — dựng bản phụ đề (không lời).")
		}
	}

	var total float64
	if audioPath != "" {
		total = wavSeconds(ctx, audioPath)
		if total < 3.0 {
			total = 3.0
		}
	} else {
		total = float64(len(captions)) * 4.0
		if total < 6.0 {
			total = 6.0
		}
	}
	per := total / float64(len(captions))

	font := findFont()
	if font == "" {
		return RenderResult{}, fmt.Errorf("Không tìm thấy font hệ thống để vẽ phụ đề.")
	}
	var bgImg string
	for _, cand := range []string{"docs/assets/hero-network.webp", "docs/assets/personas-team.webp"} {
		if _, err := os.Stat(cand); err == nil {
			bgImg = cand
			break
		}
	}

	var vf []string
	base := ""
	if bgImg != "" {
		vf = append(vf, "[0:v]scale=1080:1920:force_original_aspect_ratio=increase,"+
			"crop=1080:1920,boxblur=18:2,eq=brightness=-0.5:saturation=0.6,"+
			"fps=30,format=yuv420p[bg]")
		base = "[bg]"
	} else {
		vf = append(vf, fmt.Sprintf("color=c=#0d1117:s=1080x1920:r=30:d=%.2f,format=yuv420p[bg]", total))
		base = "[bg]"
	}
	for i, cap := range captions {
		t0, t1 := float64(i)*per, float64(i+1)*per
		color := "white"
		if i%2 == 1 {
			color = "#FFD166"
		}
		vf = append(vf, fmt.Sprintf(
			"%sdrawtext=fontfile=%s:text='%s':fontsize=68:fontcolor=%s:borderw=3:bordercolor=black:"+
				"x=(w-text_w)/2:y=860:enable='between(t,%.2f,%.2f)'[t%d]",
			base, font, dtEscape(cap), color, t0, t1, i))
		base = fmt.Sprintf("[t%d]", i)
	}
	vf = append(vf, fmt.Sprintf(
		"%sdrawtext=fontfile=%s:text='AI CREATOR OS':fontsize=28:"+
			"fontcolor=white@0.7:x=(w-text_w)/2:y=60,"+
			"drawbox=x=0:y=1900:w='1080*t/%.2f':h=20:c=#EF476F:t=fill[v]",
		base, font, total))
	filt := strings.Join(vf, ";")

	outName := job.ID + ".mp4"
	outMP4 := filepath.Join(outDir, outName)
	cmd := []string{"-y", "-v", "error"}
	if bgImg != "" {
		cmd = append(cmd, "-loop", "1", "-i", bgImg)
	}
	audioIdx := 0
	if audioPath != "" {
		cmd = append(cmd, "-i", audioPath)
		if bgImg != "" {
			audioIdx = 1
		}
	}
	cmd = append(cmd, "-filter_complex", filt, "-map", "[v]")
	if audioPath != "" {
		cmd = append(cmd, "-map", fmt.Sprintf("%d:a", audioIdx))
	}
	cmd = append(cmd, "-t", fmt.Sprintf("%.2f", total),
		"-c:v", "libx264", "-preset", "medium", "-crf", "20")
	if audioPath != "" {
		cmd = append(cmd, "-c:a", "aac", "-b:a", "128k")
	}
	cmd = append(cmd, "-movflags", "+faststart", outMP4)

	rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(rctx, "ffmpeg", cmd...).CombinedOutput(); err != nil {
		return RenderResult{}, fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	logf(fmt.Sprintf("Xong: %s (%.1fs)", outName, total))
	return RenderResult{Output: outName, Seconds: total, HasAudio: audioPath != ""}, nil
}

// dtEscape escapes text for ffmpeg drawtext (port of _dt_escape).
func dtEscape(text string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`, `%`, `%%`)
	return r.Replace(text)
}

// findFont locates a bold system font (port of _find_font).
func findFont() string {
	for _, c := range []string{
		"/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-list", ":style=Bold", "file").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
		if (strings.HasSuffix(p, ".ttf") || strings.HasSuffix(p, ".otf")) && p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// wavSeconds returns the duration of a wav file via ffprobe.
func wavSeconds(ctx context.Context, path string) float64 {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return f
}

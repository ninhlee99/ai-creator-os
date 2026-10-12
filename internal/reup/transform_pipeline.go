package reup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (t *Transformer) TransformVideo(ctx context.Context, v Video, opts TransformOptions) (string, error) {
	if err := t.ensureWorkDir(); err != nil {
		return "", err
	}
	if strings.TrimSpace(v.FilePath) == "" {
		return "", fmt.Errorf("reup: video #%d chưa có file gốc", v.ID)
	}
	if _, err := os.Stat(v.FilePath); err != nil {
		return "", fmt.Errorf("reup: file gốc không tồn tại: %s", v.FilePath)
	}
	opts.Level = Level1

	// Pass 1: video-only (zoom/tốc độ/grade).
	mainPath, mainDur, err := t.renderMainVideo(ctx, v.FilePath, opts.Seed,
		fmt.Sprintf("seg-%d-%d", v.ID, time.Now().UnixNano()))
	if err != nil {
		return "", err
	}
	defer os.Remove(mainPath)

	// Pass 2: voiceover + nhạc (không chèn chữ).
	voiceText := ""
	if opts.Voiceover {
		voiceText = t.voiceoverText(ctx, []Video{v})
	}
	outPath := filepath.Join(t.WorkDir,
		fmt.Sprintf("reup_%d_l1_%d.mp4", v.ID, time.Now().UnixNano()))
	if err := t.finalizeTransform(ctx, mainPath, mainDur, voiceText, opts, outPath); err != nil {
		os.Remove(outPath)
		return "", err
	}
	if _, qerr := qcTransform(outPath); qerr != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("reup: QC transform rớt: %w", qerr)
	}
	return outPath, nil
}

// TransformCompilation chạy transform MỨC 2: 3 clip cùng nguồn/chủ đề →
// xfade chuyển cảnh → 1 video cấu trúc mới + voiceover chung + caption.
func (t *Transformer) TransformCompilation(ctx context.Context, vs []Video, opts TransformOptions) (string, error) {
	if err := t.ensureWorkDir(); err != nil {
		return "", err
	}
	if len(vs) != 3 {
		return "", fmt.Errorf("reup: compilation cần đúng 3 clip, được %d", len(vs))
	}
	for i, v := range vs {
		if strings.TrimSpace(v.FilePath) == "" {
			return "", fmt.Errorf("reup: clip %d (video #%d) chưa có file gốc", i+1, v.ID)
		}
	}
	opts.Level = Level2
	tag := fmt.Sprintf("comp-%d", time.Now().UnixNano())

	// Pass 1: transform video-only từng clip (seed lệch nhau cho đa dạng).
	type seg struct {
		path string
		dur  float64
	}
	segs := make([]seg, 0, 3)
	for i, v := range vs {
		p, d, err := t.renderMainVideo(ctx, v.FilePath, opts.Seed+int64(i),
			fmt.Sprintf("%s-s%d", tag, i))
		if err != nil {
			for _, s := range segs {
				os.Remove(s.path)
			}
			return "", fmt.Errorf("reup: render clip %d: %w", i+1, err)
		}
		segs = append(segs, seg{p, d})
		defer os.Remove(p)
	}

	// Pass 2: xfade nối 3 clip (T9).
	xfadePath := filepath.Join(t.WorkDir, tag+"-x.mp4")
	const xd = 0.5
	xf := fmt.Sprintf(
		"[0:v][1:v]xfade=transition=fade:duration=%.1f:offset=%.3f[x1];"+
			"[x1][2:v]xfade=transition=fade:duration=%.1f:offset=%.3f[vx]",
		xd, segs[0].dur-xd, xd, segs[0].dur+segs[1].dur-2*xd)
	xargs := []string{"-i", segs[0].path, "-i", segs[1].path, "-i", segs[2].path,
		"-filter_complex", xf, "-map", "[vx]", "-r", fmt.Sprint(reupFPS), "-an"}
	xargs = append(xargs, encoderArgs()...)
	xargs = append(xargs, "-pix_fmt", "yuv420p", xfadePath)
	if err := ffmpegRun(ctx, xargs...); err != nil {
		os.Remove(xfadePath)
		return "", fmt.Errorf("reup: xfade compilation: %w", err)
	}
	defer os.Remove(xfadePath)
	compDur := segs[0].dur + segs[1].dur + segs[2].dur - 2*xd

	// Pass 3: voiceover chung + nhạc (không chèn chữ).
	voiceText := ""
	if opts.Voiceover {
		voiceText = t.voiceoverText(ctx, vs)
	}
	outPath := filepath.Join(t.WorkDir, fmt.Sprintf("reup_%s_l2.mp4", tag))
	if err := t.finalizeTransform(ctx, xfadePath, compDur, voiceText, opts, outPath); err != nil {
		os.Remove(outPath)
		return "", err
	}
	if _, qerr := qcTransform(outPath); qerr != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("reup: QC transform rớt: %w", qerr)
	}
	return outPath, nil
}

// ---------------------------------------------------------------------------
// QC sau transform: file tồn tại + có video stream + có audio stream
// (voiceover/nhạc) + độ dài > 3s. Rớt → failed + lý do, không đăng.
// ---------------------------------------------------------------------------

// qcTransform kiểm tra file transform. Trả về độ dài (giây).
func qcTransform(path string) (float64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("file không tồn tại: %s", path)
	}
	if st.Size() == 0 {
		return 0, fmt.Errorf("file rỗng (0 byte)")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, fmt.Errorf("thiếu ffprobe — không QC được transform (fail-closed)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Streams: cần cả video và audio.
	sout, serr := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path).Output()
	if serr != nil {
		return 0, fmt.Errorf("ffprobe không đọc được stream: %v", serr)
	}
	var hasV, hasA bool
	for _, line := range strings.Split(strings.TrimSpace(string(sout)), "\n") {
		switch strings.TrimSpace(line) {
		case "video":
			hasV = true
		case "audio":
			hasA = true
		}
	}
	if !hasV {
		return 0, fmt.Errorf("thiếu video stream")
	}
	if !hasA {
		return 0, fmt.Errorf("thiếu audio stream (voiceover/nhạc)")
	}
	dur, derr := probeDuration(path)
	if derr != nil {
		return 0, derr
	}
	if dur <= 3 {
		return 0, fmt.Errorf("video quá ngắn (%.1fs ≤ 3s)", dur)
	}
	return dur, nil
}

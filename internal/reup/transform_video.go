package reup

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func videoChainFilter(durSec float64, seed int64) (string, float64) {
	if durSec <= 0 {
		durSec = 5
	}
	// T3: tốc độ ±5% — chẵn nhanh hơn 5%, lẻ chậm hơn 5% (deterministic theo seed).
	speed := 1.05
	setpts := "0.952381*PTS"
	if seed%2 != 0 {
		speed = 0.95
		setpts = "1.052632*PTS"
	}
	outDur := durSec / speed
	frames := int(outDur*reupFPS + 0.5)
	if frames < 1 {
		frames = 1
	}
	// T5: 4 hướng Ken Burns theo seed (ổn định, không random runtime).
	var zpParams []string
	switch ((seed % 4) + 4) % 4 {
	case 1: // pull-out: 1.12 → 1.0
		zpParams = []string{
			fmt.Sprintf("z='max(1.12-0.12*on/%d,1.0)'", frames),
			"x='iw/2-(iw/zoom/2)'", "y='ih/2-(ih/zoom/2)'",
		}
	case 2: // pan trái→phải (giữ zoom 1.12)
		zpParams = []string{
			"z='1.12'",
			fmt.Sprintf("x='(iw-iw/zoom)*on/%d'", frames),
			"y='ih/2-(ih/zoom/2)'",
		}
	case 3: // pan phải→trái (giữ zoom 1.12)
		zpParams = []string{
			"z='1.12'",
			fmt.Sprintf("x='(iw-iw/zoom)*(1-on/%d)'", frames),
			"y='ih/2-(ih/zoom/2)'",
		}
	default: // push-in: 1.0 → 1.12
		zpParams = []string{
			fmt.Sprintf("z='min(1+0.12*on/%d,1.12)'", frames),
			"x='iw/2-(iw/zoom/2)'", "y='ih/2-(ih/zoom/2)'",
		}
	}
	zp := "zoompan=d=1:" + strings.Join(zpParams, ":") +
		fmt.Sprintf(":s=%dx%d:fps=%d", reupOutW, reupOutH, reupFPS)
	// T4: grade nhẹ.
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,"+
			"%s,"+
			"setpts=%s,eq=contrast=1.06:saturation=1.12:brightness=0.01",
		reupOutW*2, reupOutH*2, reupOutW*2, reupOutH*2,
		zp, setpts)
	return filter, outDur
}

// probeDuration đọc độ dài video bằng ffprobe (giây). Thiếu ffprobe → lỗi
// (transform cần số này để dựng filter — fail-closed, không đoán).
func probeDuration(path string) (float64, error) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, fmt.Errorf("reup: thiếu ffprobe — không dựng được transform")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("reup: ffprobe không đọc được %s: %v", filepath.Base(path), err)
	}
	var d float64
	if _, serr := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d); serr != nil || d <= 0 {
		return 0, fmt.Errorf("reup: ffprobe trả độ dài không hợp lệ cho %s", filepath.Base(path))
	}
	return d, nil
}

// renderMainVideo transform video-only 1 clip (không audio, không
// intro/outro) → temp mp4. Dùng cho cả mức 1 (1 clip) và mức 2 (3 clip).
func (t *Transformer) renderMainVideo(ctx context.Context, srcPath string, seed int64, name string) (outPath string, outDur float64, err error) {
	dur, err := probeDuration(srcPath)
	if err != nil {
		return "", 0, err
	}
	filter, outDur := videoChainFilter(dur, seed)
	outPath = filepath.Join(t.WorkDir, name+".mp4")
	args := []string{"-i", srcPath,
		"-vf", filter,
		"-r", fmt.Sprint(reupFPS),
		"-an",
	}
	args = append(args, encoderArgs()...)
	args = append(args, "-pix_fmt", "yuv420p", "-t", fmt.Sprintf("%.3f", outDur), outPath)
	if err := ffmpegRun(ctx, args...); err != nil {
		return "", 0, fmt.Errorf("reup: render clip: %w", err)
	}
	return outPath, outDur, nil
}

// ---------------------------------------------------------------------------
// Voiceover T8: lời bình luận tiếng Việt.
//
// LLM có → nhờ viết ngắn (≤300 ký tự), chỉ dùng thông tin đã cho.
// Không có/không viết được → template TRUNG THỰC: chỉ dùng sự thật có
// trong metadata (tiêu đề/tác giả/lượt xem), không bịa tình tiết.
// ---------------------------------------------------------------------------

// voiceoverText viết lời bình luận cho 1..3 video.
func (t *Transformer) voiceoverText(ctx context.Context, vs []Video) string {
	if t.LLM != nil {
		if s := t.llmCommentary(ctx, vs); s != "" {
			return s
		}
	}
	var seed int64
	for _, v := range vs {
		seed += v.ID
	}
	return templateCommentary(vs, seed)
}

// llmCommentary nhờ LLM viết lời bình luận ngắn. Lỗi → "" (caller dùng template).

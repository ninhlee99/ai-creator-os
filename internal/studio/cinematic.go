package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Film Wave 3 — "Điện ảnh từ ảnh" là CHẾ ĐỘ DỰNG CHÍNH: key Gemini của Ninh
// không tạo được video (Veo trả lỗi), nên đường dựng từ keyframe phải đạt
// chất lượng điện ảnh thật — Veo chỉ còn là tuỳ chọn.
//
// Mỗi shot: keyframe (đạo diễn viết riêng cho chế độ ảnh — xem
// prompts/film_keyframe.txt) → chuyển động điện ảnh (zoompan có easing theo
// đúng chỉ đạo camera_move của đạo diễn, không random) → film grain nhẹ +
// vignette + grade màu theo mood → chuyển cảnh xfade giữa các shot.
//
// Chế độ dựng (FilmParams.RenderMode):
//   - "auto" (mặc định, khuyên dùng): Veo nếu key quay được (probe/lịch
//     sử), ngược lại đi thẳng điện ảnh từ ảnh — không thử Veo, đỡ tốn
//     thời gian poll.
//   - "cinematic": luôn dựng điện ảnh từ ảnh (miễn phí — chỉ tốn image gen).
//   - "veo": luôn thử Veo từng shot (tốn phí); lỗi thì rơi về điện ảnh.

const (
	RenderModeAuto      = "auto"
	RenderModeCinematic = "cinematic"
	RenderModeVeo       = "veo"

	cineFPS      = 30  // = outFPS — Ninh chốt 24/30, giữ 30
	cineXfadeDur = 0.7 // chuyển cảnh 0.7s
	cineGrain    = 7   // noise alls — hạt phim nhẹ, temporal
)

// renderModeLabel renders the mode for the job log (Vietnamese).
func renderModeLabel(mode string) string {
	switch mode {
	case RenderModeCinematic:
		return "Điện ảnh từ ảnh (miễn phí)"
	case RenderModeVeo:
		return "Veo (tốn phí)"
	default:
		return "Tự động"
	}
}

// resolveRenderMode normalizes the requested mode. "auto" becomes "veo"
// only when the key is known to shoot video (capability probe/history);
// otherwise — and always for Ninh's key, which cannot shoot video — it
// becomes "cinematic" so the job never wastes minutes polling a dead API.
func (s *Studio) resolveRenderMode(p FilmParams) string {
	switch p.RenderMode {
	case RenderModeCinematic:
		return RenderModeCinematic
	case RenderModeVeo:
		return RenderModeVeo
	default:
		if s.VideoCapOK() {
			return RenderModeVeo
		}
		return RenderModeCinematic
	}
}

// CinematicShot describes one shot's still-image render.
type CinematicShot struct {
	ImagePath  string
	Seconds    float64
	CameraMove string // chỉ đạo của đạo diễn — không random
	Mood       string // atmosphere của cảnh → grade màu
}

// cineMoveKind normalizes the director's camera_move (EN/VI) to a motion
// kind. Unknown moves become a slow breathing drift — never a dead hold.
func cineMoveKind(cameraMove string) string {
	m := strings.ToLower(cameraMove)
	switch {
	case strings.Contains(m, "dolly-in") || strings.Contains(m, "push") ||
		strings.Contains(m, "zoom in") || strings.Contains(m, "tiến") ||
		strings.Contains(m, "lại gần"):
		return "in"
	case strings.Contains(m, "dolly-out") || strings.Contains(m, "pull") ||
		strings.Contains(m, "zoom out") || strings.Contains(m, "rút") ||
		strings.Contains(m, "ra xa"):
		return "out"
	case strings.Contains(m, "pan left") || strings.Contains(m, "tracking left") ||
		strings.Contains(m, "lia trái"):
		return "panleft"
	case strings.Contains(m, "pan right") || strings.Contains(m, "tracking right") ||
		strings.Contains(m, "lia phải"):
		return "panright"
	case strings.Contains(m, "tilt up") || strings.Contains(m, "ngước") ||
		strings.Contains(m, "ngẩng"):
		return "tiltup"
	case strings.Contains(m, "tilt down") || strings.Contains(m, "cúi") ||
		strings.Contains(m, "gục"):
		return "tiltdown"
	default:
		return "drift"
	}
}

// cineZoompan builds the eased zoompan expressions for a motion kind.
// e = (1-cos(PI*on/NF))/2 is the ease-in-out progress 0→1 over the shot.
// zoompan's "zoom" variable is usable inside x/y (as in assemble.go).
func cineZoompan(kind string, nf int) (z, x, y string) {
	e := fmt.Sprintf("(1-cos(PI*on/%d))/2", nf)
	cx, cy := "iw/2-(iw/zoom/2)", "ih/2-(ih/zoom/2)"
	switch kind {
	case "in": // dolly-in chậm 1.00 → 1.18
		return "1+0.18*" + e, cx, cy
	case "out": // dolly-out chậm 1.18 → 1.00
		return "1.18-0.18*" + e, cx, cy
	case "panleft": // lia trái: cửa sổ quét phải dần (z cố định 1.12)
		return "1.12", fmt.Sprintf("(iw-iw/1.12)*%s", e), cy
	case "panright":
		return "1.12", fmt.Sprintf("(iw-iw/1.12)*(1-%s)", e), cy
	case "tiltup": // ngước lên: cửa sổ đi từ dưới lên trên
		return "1.12", cx, fmt.Sprintf("(ih-ih/1.12)*(1-%s)", e)
	case "tiltdown":
		return "1.12", cx, fmt.Sprintf("(ih-ih/1.12)*%s", e)
	default: // drift — thở nhẹ, không bao giờ đứng yên
		return "1+0.06*" + e, cx, cy
	}
}

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// cineGrade maps the scene atmosphere/mood to a simple cinematic color
// grade. Keywords cover the director's English atmosphere text and
// Vietnamese UI text — first match wins, default is a gentle contrast lift.
func cineGrade(mood string) string {
	m := strings.ToLower(mood)
	switch {
	case containsAny(m, "golden", "sunset", "warm", "ấm", "hoàng hôn", "nắng chiều", "nắng"):
		return "eq=contrast=1.06:saturation=1.18:brightness=0.02,colorbalance=rs=0.10:gs=0.03:bs=-0.10"
	case containsAny(m, "cold", "blue", "night", "lạnh", "đêm", "buốt", "xanh"):
		return "eq=contrast=1.08:saturation=0.92:brightness=-0.01,colorbalance=rs=-0.08:bs=0.12"
	case containsAny(m, "dark", "noir", "moody", "tối", "u ám", "âm u", "đen tối"):
		return "eq=brightness=-0.05:contrast=1.18:saturation=0.82"
	case containsAny(m, "bright", "daylight", "sunny", "sáng", "ban ngày", "tươi"):
		return "eq=contrast=1.05:saturation=1.10:brightness=0.02"
	default:
		return "eq=contrast=1.05:saturation=1.05"
	}
}

// RenderCinematicShot renders one shot's keyframe into a cinematic clip:
// eased camera move → supersampled motion → grade → film grain + vignette.
// Output is video-only (1920x1080, 30fps); voice is muxed separately by
// AddShotAudio so every shot mp4 carries an audio track for the xfade
// assembly.
func RenderCinematicShot(ctx context.Context, in CinematicShot, outPath string) error {
	secs := in.Seconds
	if secs < 1 {
		secs = 1
	}
	frames := int(secs*float64(cineFPS) + 0.5)
	nf := frames - 1
	if nf < 1 {
		nf = 1
	}
	z, x, y := cineZoompan(cineMoveKind(in.CameraMove), nf)
	filter := fmt.Sprintf(
		"[0:v]scale=3840:2160:force_original_aspect_ratio=increase,crop=3840:2160,setsar=1,"+
			"zoompan=z='%s':d=%d:x='%s':y='%s':s=3840x2160:fps=%d,"+
			"scale=1920:1080:flags=lanczos,setsar=1,"+
			"%s,noise=alls=%d:allf=t,vignette=PI/5[v]",
		z, frames, x, y, cineFPS, cineGrade(in.Mood), cineGrain)
	args := []string{"-framerate", fmt.Sprint(cineFPS), "-i", in.ImagePath,
		"-filter_complex", filter, "-map", "[v]",
		"-frames:v", fmt.Sprint(frames), "-r", fmt.Sprint(cineFPS)}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", outPath)
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return ffmpegRun(ctx, args...)
}

// AddShotAudio muxes the shot's voice onto the cinematic clip. Empty
// wavPath → a silent track of the shot length, so EVERY shot mp4 has an
// audio track and the final xfade assembly can acrossfade uniformly.
func AddShotAudio(ctx context.Context, videoPath, wavPath string, seconds float64, outPath string) error {
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if strings.TrimSpace(wavPath) != "" {
		return ffmpegRun(ctx, "-i", videoPath, "-i", wavPath,
			"-map", "0:v", "-map", "1:a",
			"-c:v", "copy", "-c:a", "aac", "-b:a", "160k",
			"-shortest", outPath)
	}
	return ffmpegRun(ctx, "-i", videoPath,
		"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=24000:cl=mono:d=%.2f", seconds),
		"-map", "0:v", "-map", "1:a",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "96k",
		"-shortest", outPath)
}

// ApplyLetterbox masks a 16:9 film to 2.35:1 scope bars — the cinema look.
// Applied once at final assembly (never on per-shot clips, so trailers
// cropped from the shots stay clean).
func ApplyLetterbox(ctx context.Context, inPath, outPath string) error {
	// 1920/2.35 ≈ 817 → bar = (1080-817)/2 ≈ 131 px mỗi viền.
	bar := 131
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	vf := fmt.Sprintf("drawbox=x=0:y=0:w=iw:h=%d:c=black:t=fill,"+
		"drawbox=x=0:y=ih-%d:w=iw:h=%d:c=black:t=fill", bar, bar, bar)
	args := []string{"-i", inPath, "-vf", vf}
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", outPath)
	return ffmpegRun(ctx, args...)
}

// CinematicClip is one rendered shot mp4 (with audio) for the xfade assembly.
type CinematicClip struct {
	Path       string
	CameraMove string
}

// pickTransition chooses the cut between two shots: a whip (smoothleft)
// when either side pans/tracks, otherwise a soft fade. Deterministic —
// the edit rhythm is reproducible.
func pickTransition(moveA, moveB string) string {
	k1, k2 := cineMoveKind(moveA), cineMoveKind(moveB)
	if strings.HasPrefix(k1, "pan") || strings.HasPrefix(k2, "pan") {
		return "smoothleft"
	}
	return "fade"
}

// AssembleCinematic joins cinematic shot clips with xfade transitions plus
// a matched acrossfade on audio. Inputs are normalized to 1920x1080/30fps
// (each must carry an audio track — see AddShotAudio). Output duration =
// sum − (n−1)×0.7s.
func AssembleCinematic(ctx context.Context, clips []CinematicClip, outPath string) error {
	if len(clips) == 0 {
		return fmt.Errorf("no clips")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if len(clips) == 1 {
		return copyFile(clips[0].Path, outPath)
	}
	durs := make([]float64, len(clips))
	for i, c := range clips {
		d := ProbeDuration(ctx, c.Path)
		if d <= 0 {
			return fmt.Errorf("clip không đọc được: %s", filepath.Base(c.Path))
		}
		durs[i] = d
	}
	var args []string
	for _, c := range clips {
		args = append(args, "-i", c.Path)
	}
	var sb strings.Builder
	// Chuẩn hoá mọi input về 1920×1080/30fps/stereo-48kHz trước khi nối —
	// shot Veo (24fps) có thể trộn với shot cinematic fallback (30fps),
	// audio TTS/silence khác sample rate.
	for i := range clips {
		fmt.Fprintf(&sb, "[%d:v]scale=1920:1080,fps=30,format=yuv420p,settb=AVTB[nv%d];", i, i)
		fmt.Fprintf(&sb, "[%d:a]aformat=sample_fmts=fltp:channel_layouts=stereo,aresample=48000[na%d];", i, i)
	}
	cum := durs[0]
	prev := "[nv0]"
	for i := 1; i < len(clips); i++ {
		offset := cum - cineXfadeDur
		if offset < 0 {
			offset = 0
		}
		out := fmt.Sprintf("[x%d]", i)
		if i == len(clips)-1 {
			out = "[vout]"
		}
		fmt.Fprintf(&sb, "%s[nv%d]xfade=transition=%s:duration=%.1f:offset=%.2f%s;",
			prev, i, pickTransition(clips[i-1].CameraMove, clips[i].CameraMove),
			cineXfadeDur, offset, out)
		prev = out
		cum = cum + durs[i] - cineXfadeDur
	}
	aprev := "[na0]"
	for i := 1; i < len(clips); i++ {
		out := fmt.Sprintf("[a%d]", i)
		if i == len(clips)-1 {
			out = "[aout]"
		}
		fmt.Fprintf(&sb, "%s[na%d]acrossfade=d=%.1f:c1=tri:c2=tri%s;",
			aprev, i, cineXfadeDur, out)
		aprev = out
	}
	args = append(args, "-filter_complex", strings.TrimSuffix(sb.String(), ";"),
		"-map", "[vout]", "-map", "[aout]", "-r", fmt.Sprint(cineFPS))
	args = append(args, videoEncoder()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k", outPath)
	return ffmpegRun(ctx, args...)
}

// subCue is one subtitle line with explicit timing. The cinematic assembly
// overlaps shots by the xfade duration, so sequential per-shot durations
// don't apply — cues are laid on the real timeline.
type subCue struct {
	Text       string
	Start, End float64
}

// cinematicCues lays subtitle texts on the xfade timeline: shot i starts
// after the previous shots minus the overlaps. Small 0.35s gap avoids
// overlapping lines on screen.
func cinematicCues(texts []string, durs []float64) []subCue {
	var out []subCue
	start := 0.0
	for i, t := range texts {
		if i >= len(durs) {
			break
		}
		if strings.TrimSpace(t) == "" {
			continue
		}
		end := start + durs[i] - 0.35
		if end <= start {
			end = start + 0.5
		}
		out = append(out, subCue{Text: strings.TrimSpace(t), Start: start, End: end})
		start += durs[i] - cineXfadeDur
	}
	return out
}

func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(sec*1000 + 0.5)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}

func formatSRT(cues []subCue) string {
	var sb strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.Start), srtTime(c.End), c.Text)
	}
	return sb.String()
}

// keyframeCompBlock returns the keyframe composition guidance for the
// cinematic stills mode (prompts/film_keyframe.txt): headroom for motion,
// depth layers, cinematic light. Fail-soft — empty on error, the shot
// still renders.
func keyframeCompBlock() string {
	b, err := directorPrompt("film_keyframe.txt", nil)
	if err != nil {
		return ""
	}
	return "\n" + b
}

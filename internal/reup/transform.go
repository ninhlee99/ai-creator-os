package reup

// Transform (Đợt E): 2 mức transform cho video Douyin trước khi đăng.
//
// TRUNG THỰC — đọc trước khi sửa file này:
//   - KHÔNG kỹ thuật nào đảm bảo 100% không bị đánh bản quyền (Content ID /
//     perceptual hash ngày càng mạnh; retroactive claim có thể xảy ra).
//     Mọi comment, UI, doc của transform chỉ được ghi "giảm rủi ro",
//     KHÔNG BAO GIỜ ghi "an toàn bản quyền" / "qua Content ID".
//   - Nguy cơ lớn nhất của reup năm 2026 là video 0-view/de-boost (policy
//     "unoriginal content" của TikTok/YouTube), không phải strike.
//   - KHÔNG BAO GIỜ xuất bản video 0 transform: transform lỗi → failed,
//     không có đường nào đăng bản gốc.
//
// Mức 1 (mọi video): T5 zoom động + T3 tốc độ ±5% + T4 grade nhẹ +
// T8 voiceover bình luận tiếng Việt (TTS VieNeu local, không tốn Gemini
// key) + T7 thay audio bằng nhạc licensed + intro/outro card + caption
// tiếng Việt. Output 9:16 1080x1920.
//
// Mức 2 (video hot): Mức 1 + T9 compilation 3 clip cùng nguồn/chủ đề
// (xfade chuyển cảnh) + cấu trúc mới.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// Kích thước output reup: 9:16 dọc.
const (
	reupOutW = 1080
	reupOutH = 1920
	reupFPS  = 30
)

// Mức transform.
const (
	Level1 = 1 // mặc định mọi video
	Level2 = 2 // compilation 3 clip (video hot)
)

// VoiceSynth tổng hợp voiceover tiếng Việt. Hợp đồng output: WAV PCM s16le
// 24kHz mono (giống TTSProvider) — VieNeu local đi qua chain, không tốn
// Gemini key.
type VoiceSynth interface {
	Synthesize(ctx context.Context, text, voice string) ([]byte, error)
}

// Commentator viết lời bình luận ngắn từ metadata (LLM). Nil = dùng
// template trung thực (chỉ dùng sự thật có trong metadata, không bịa).
type Commentator interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// TransformOptions là tham số một lượt transform.
type TransformOptions struct {
	Level     int    // Level1 | Level2
	Seed      int64  // fingerprint riêng (video id + kênh) — mỗi kênh một seed
	Title     string // chữ trên intro/outro card ("" = card trơn)
	Voiceover bool   // T8: bật voiceover bình luận tiếng Việt
	Voice     string // preset voice VieNeu ("" = mặc định)
}

// Transformer chạy transform bằng FFmpeg có sẵn (pure Go gọi binary).
type Transformer struct {
	Store *Store
	Voice VoiceSynth  // nil = không tổng hợp được → fail-closed khi Voiceover=true
	LLM   Commentator // nil = template trung thực
	// MusicPath là file nhạc licensed (kho nhạc autopilot Ninh đã upload).
	// "" = không nhạc nền (chỉ voiceover).
	MusicPath string
	// WorkDir chứa file transform output.
	WorkDir string
}

// ffmpegRun chạy ffmpeg -y; lỗi kèm đuôi stderr để debug.
func ffmpegRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-y", "-v", "error"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stderr.String())
		if len(out) > 400 {
			out = out[len(out)-400:]
		}
		return fmt.Errorf("ffmpeg: %v: %s", err, out)
	}
	return nil
}

// encoderArgs chọn encoder như studio (videotoolbox trên Mac, libx264
// nơi khác) — dùng chung qua studio.VideoEncoderArgs để không copy logic.
func encoderArgs() []string { return studio.VideoEncoderArgs() }

// ---------------------------------------------------------------------------
// Chuỗi filter video (mức 1 cho 1 clip) — pure function, test được không
// cần ffmpeg: cover 9:16 → zoom động (crop w/h theo t) → tốc độ ±5% theo
// seed → grade nhẹ.
// ---------------------------------------------------------------------------

// videoChainFilter dựng filter cho 1 clip. Trả về (filter, độ dài output giây).
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
	// T5: zoom động push-in 1.0 → 1.12 (zoompan d=1 tính theo từng frame
	// của video input — khác với crop w/h vốn chỉ tính 1 lần lúc init).
	frames := int(outDur*reupFPS + 0.5)
	if frames < 1 {
		frames = 1
	}
	// T4: grade nhẹ.
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,"+
			"zoompan=z='min(1+0.12*on/%d,1.12)':d=1:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=%d,"+
			"setpts=%s,eq=contrast=1.06:saturation=1.12:brightness=0.01",
		reupOutW*2, reupOutH*2, reupOutW*2, reupOutH*2,
		frames, reupOutW, reupOutH, reupFPS, setpts)
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
	return templateCommentary(vs)
}

// llmCommentary nhờ LLM viết lời bình luận ngắn. Lỗi → "" (caller dùng template).
func (t *Transformer) llmCommentary(ctx context.Context, vs []Video) string {
	var sb strings.Builder
	for i, v := range vs {
		fmt.Fprintf(&sb, "- Video %d: tiêu đề %q, tác giả %q", i+1, v.Title, v.Author)
		if v.PlayCount > 0 {
			fmt.Fprintf(&sb, ", %d lượt xem", v.PlayCount)
		}
		sb.WriteString("\n")
	}
	system := "Bạn viết lời bình luận tiếng Việt cho video Douyin thể loại " +
		"tiên hiệp/thần tiên. Yêu cầu: 1-2 câu ngắn gọn, tự nhiên như người xem " +
		"bình luận; CHỈ dùng thông tin đã cho, tuyệt đối không bịa tình tiết, " +
		"không khẳng định điều không có trong dữ liệu; tối đa 300 ký tự."
	n := len(vs)
	kind := "một video"
	if n > 1 {
		kind = fmt.Sprintf("tuyển tập %d video", n)
	}
	out, err := t.LLM.Complete(ctx, system,
		fmt.Sprintf("Viết lời bình luận cho %s:\n%s", kind, sb.String()))
	if err != nil {
		return ""
	}
	return sanitizeCommentary(out)
}

// templateCommentary: dự phòng trung thực khi không có LLM.
func templateCommentary(vs []Video) string {
	if len(vs) > 1 {
		return fmt.Sprintf("Tuyển tập %d khoảnh khắc tiên hiệp đang được yêu thích trên Douyin. Mời bạn xem.", len(vs))
	}
	v := vs[0]
	var sb strings.Builder
	if strings.TrimSpace(v.Title) != "" {
		fmt.Fprintf(&sb, "Video %q", strings.TrimSpace(v.Title))
	} else {
		sb.WriteString("Video này")
	}
	if strings.TrimSpace(v.Author) != "" {
		fmt.Fprintf(&sb, " của %s", strings.TrimSpace(v.Author))
	}
	if v.PlayCount > 0 {
		fmt.Fprintf(&sb, ", đang được xem nhiều trên Douyin với %d lượt xem", v.PlayCount)
	} else {
		sb.WriteString(", đang được chú ý trên Douyin")
	}
	sb.WriteString(". Mời bạn xem.")
	return sb.String()
}

// sanitizeCommentary làm sạch text LLM: gọn whitespace, cắt 300 ký tự.
func sanitizeCommentary(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, "\"“”")
	if len([]rune(s)) > 300 {
		r := []rune(s)
		s = string(r[:297]) + "…"
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// Finalize: intro/outro card + trộn voiceover/nhạc + caption.
//
// Một pass ffmpeg: [intro][main][outro] concat (video) + voiceover delay
// + nhạc nền duck nhẹ + loudnorm. Caption chạy pass riêng (burn-in thử
// trước, gãy → mux SRT mov_text như studio).
// ---------------------------------------------------------------------------

const (
	cardDur   = 1.2 // giây mỗi card intro/outro
	cardColor = "0x2b2f4a"
	voiceLead = 2.0 // voiceover bắt đầu sau intro 0.8s
)

// escapeDrawtext escape text cho filter drawtext.
func escapeDrawtext(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'", "%", "\\%")
	return r.Replace(s)
}

// finalFilter dựng filter_complex cho pass finalize.
// voiceIdx/musicIdx: -1 = không có input tương ứng.
func finalFilter(mainDur float64, title string, voiceIdx, musicIdx int) (filter string, totalDur float64) {
	totalDur = cardDur + mainDur + cardDur
	var sb strings.Builder
	// Card intro/outro: input card (index 3) split làm đôi; chữ khi có title.
	intro, outro := "[3:v]split=2[intro0][outro0]", ""
	_ = outro
	if strings.TrimSpace(title) != "" {
		intro = fmt.Sprintf("[3:v]split=2[i0][o0];"+
			"[i0]drawtext=text='%s':fontsize=52:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2[intro];"+
			"[o0]drawtext=text='Hẹn gặp lại':fontsize=44:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2[outro]",
			escapeDrawtext(strings.TrimSpace(title)))
		sb.WriteString(intro + ";")
	} else {
		sb.WriteString("[3:v]split=2[intro][outro];")
	}
	sb.WriteString("[intro][0:v][outro]concat=n=3:v=1:a=0[vcat];")
	// Audio: voiceover delay sau intro; nhạc nền nhỏ + fade; trộn + loudnorm.
	if voiceIdx >= 0 {
		fmt.Fprintf(&sb, "[%d:a]aresample=44100,adelay=%d|%d,apad[vo];",
			voiceIdx, int(voiceLead*1000), int(voiceLead*1000))
	}
	if musicIdx >= 0 {
		fmt.Fprintf(&sb, "[%d:a]atrim=0:%.2f,afade=t=in:st=0:d=1,afade=t=out:st=%.2f:d=1,volume=0.35[mu];",
			musicIdx, totalDur, totalDur-1)
	}
	switch {
	case voiceIdx >= 0 && musicIdx >= 0:
		sb.WriteString("[vo][mu]amix=inputs=2:duration=first:dropout_transition=0," +
			"loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	case voiceIdx >= 0:
		sb.WriteString("[vo]loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	case musicIdx >= 0:
		sb.WriteString("[mu]loudnorm=I=-14:TP=-1.5:LRA=11[aout]")
	}
	return strings.TrimSuffix(sb.String(), ";"), totalDur
}

// finalizeTransform pass cuối: card + audio mix → outPath, rồi caption.
func (t *Transformer) finalizeTransform(ctx context.Context, mainPath string, mainDur float64,
	voiceText string, opts TransformOptions, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("reup: tạo thư mục output: %w", err)
	}
	tmp, err := os.CreateTemp(t.WorkDir, "final-*.mp4")
	if err != nil {
		return fmt.Errorf("reup: tạo file tạm: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	// Voiceover T8.
	var voiceWav string
	voiceIdx := -1
	if opts.Voiceover {
		if t.Voice == nil {
			return fmt.Errorf("reup: voiceover bật nhưng VieNeu chưa sẵn sàng — dừng (fail-closed)")
		}
		if strings.TrimSpace(voiceText) == "" {
			return fmt.Errorf("reup: không có lời bình luận để đọc")
		}
		wav, werr := t.Voice.Synthesize(ctx, voiceText, opts.Voice)
		if werr != nil {
			return fmt.Errorf("reup: TTS voiceover thất bại: %w", werr)
		}
		f, ferr := os.CreateTemp(t.WorkDir, "vo-*.wav")
		if ferr != nil {
			return fmt.Errorf("reup: ghi wav tạm: %w", ferr)
		}
		voiceWav = f.Name()
		if _, werr := f.Write(wav); werr != nil {
			f.Close()
			os.Remove(voiceWav)
			return fmt.Errorf("reup: ghi wav tạm: %w", werr)
		}
		f.Close()
		defer os.Remove(voiceWav)
	}

	// Input: 0=main, 1=voice (nếu có), 2=nhạc (nếu có), cuối=color card.
	args := []string{"-i", mainPath}
	next := 1
	if voiceWav != "" {
		args = append(args, "-i", voiceWav)
		voiceIdx = next
		next++
	}
	musicIdx := -1
	musicUsed := ""
	if strings.TrimSpace(t.MusicPath) != "" {
		if _, serr := os.Stat(t.MusicPath); serr == nil {
			args = append(args, "-i", t.MusicPath)
			musicUsed = t.MusicPath
			musicIdx = next
			next++
		}
	}
	if voiceIdx < 0 && musicIdx < 0 {
		return fmt.Errorf("reup: thiếu cả voiceover lẫn nhạc nền — không tạo video câm")
	}
	cardIdx := next
	args = append(args, "-f", "lavfi", "-i",
		fmt.Sprintf("color=c=%s:s=%dx%d:r=%d:d=%.1f", cardColor, reupOutW, reupOutH, reupFPS, cardDur))

	filter, total := finalFilter(mainDur, opts.Title, voiceIdx, musicIdx)
	// Đổi index card trong filter: finalFilter giả định card ở index 3.
	filter = strings.ReplaceAll(filter, "[3:v]", fmt.Sprintf("[%d:v]", cardIdx))
	args = append(args,
		"-filter_complex", filter,
		"-map", "[vcat]", "-map", "[aout]",
		"-r", fmt.Sprint(reupFPS),
		"-t", fmt.Sprintf("%.2f", total),
	)
	args = append(args, encoderArgs()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k",
		"-shortest", tmpPath)
	if err := ffmpegRun(ctx, args...); err != nil {
		// drawtext có thể gãy khi máy thiếu font — thử lại card trơn.
		if strings.TrimSpace(opts.Title) != "" {
			if rerr := t.finalizeNoText(ctx, mainPath, mainDur, voiceWav, musicUsed, tmpPath); rerr != nil {
				return fmt.Errorf("reup: finalize (kể cả card trơn): %w", rerr)
			}
		} else {
			return fmt.Errorf("reup: finalize: %w", err)
		}
	}

	// Caption tiếng Việt: burn-in thử trước, gãy → mux SRT (không re-encode).
	srt := buildSRT(voiceText, voiceLead, total-cardDur)
	if strings.TrimSpace(voiceText) != "" {
		if cerr := burnCaptions(ctx, tmpPath, srt, outPath); cerr != nil {
			if merr := studio.MuxSubtitles(ctx, tmpPath, srt, outPath); merr != nil {
				return fmt.Errorf("reup: caption: %w", merr)
			}
		}
	} else if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("reup: chốt file: %w", err)
	}
	return nil
}

// finalizeNoText thử lại finalize với card trơn (khi drawtext gãy vì thiếu font).
func (t *Transformer) finalizeNoText(ctx context.Context, mainPath string, mainDur float64,
	voiceWav, musicUsed, outPath string) error {
	args := []string{"-i", mainPath}
	next := 1
	voiceIdx := -1
	if voiceWav != "" {
		args = append(args, "-i", voiceWav)
		voiceIdx = next
		next++
	}
	musicIdx := -1
	if musicUsed != "" {
		args = append(args, "-i", musicUsed)
		musicIdx = next
		next++
	}
	cardIdx := next
	args = append(args, "-f", "lavfi", "-i",
		fmt.Sprintf("color=c=%s:s=%dx%d:r=%d:d=%.1f", cardColor, reupOutW, reupOutH, reupFPS, cardDur))
	filter, total := finalFilter(mainDur, "", voiceIdx, musicIdx)
	filter = strings.ReplaceAll(filter, "[3:v]", fmt.Sprintf("[%d:v]", cardIdx))
	args = append(args,
		"-filter_complex", filter,
		"-map", "[vcat]", "-map", "[aout]",
		"-r", fmt.Sprint(reupFPS),
		"-t", fmt.Sprintf("%.2f", total),
	)
	args = append(args, encoderArgs()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k",
		"-shortest", outPath)
	return ffmpegRun(ctx, args...)
}

// buildSRT tạo phụ đề từ lời bình luận (1 cue).
func buildSRT(text string, startSec, endSec float64) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return fmt.Sprintf("1\n%s --> %s\n%s\n", srtTime(startSec), srtTime(endSec), text)
}

func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(sec*1000 + 0.5)
	h, ms := ms/3600000, ms%3600000
	m, ms := ms/60000, ms%60000
	s, ms := ms/1000, ms%1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// burnCaptions burn-in SRT bằng filter subtitles (libass). Gãy → caller mux.
func burnCaptions(ctx context.Context, inPath, srtText, outPath string) error {
	if strings.TrimSpace(srtText) == "" {
		return fmt.Errorf("reup: caption rỗng")
	}
	f, err := os.CreateTemp(filepath.Dir(outPath), "cap-*.srt")
	if err != nil {
		return err
	}
	srtPath := f.Name()
	if _, err := f.WriteString(srtText); err != nil {
		f.Close()
		os.Remove(srtPath)
		return err
	}
	f.Close()
	defer os.Remove(srtPath)
	abs, err := filepath.Abs(srtPath)
	if err != nil {
		return err
	}
	// Escape cho filter subtitles: \ : '
	esc := strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'").Replace(abs)
	args := []string{"-i", inPath, "-vf", "subtitles=" + esc}
	args = append(args, encoderArgs()...)
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "copy", outPath)
	return ffmpegRun(ctx, args...)
}

// ---------------------------------------------------------------------------
// Entry points: TransformVideo (mức 1), TransformCompilation (mức 2).
// ---------------------------------------------------------------------------

// ensureWorkDir chuẩn bị WorkDir.
func (t *Transformer) ensureWorkDir() error {
	if strings.TrimSpace(t.WorkDir) == "" {
		return fmt.Errorf("reup: chưa cấu hình thư mục transform")
	}
	return os.MkdirAll(t.WorkDir, 0o755)
}

// TransformVideo chạy transform MỨC 1 cho 1 video đã tải.
// Trả về đường dẫn file output. Lỗi → caller đánh failed; KHÔNG BAO GIỜ
// trả về bản gốc hay file nửa vời để đăng.
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

	// Pass 2: voiceover + nhạc + intro/outro + caption.
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

	// Pass 3: voiceover chung + nhạc + intro/outro + caption.
	voiceText := ""
	if opts.Voiceover {
		voiceText = t.voiceoverText(ctx, vs)
	}
	if strings.TrimSpace(opts.Title) == "" {
		opts.Title = "Tuyển tập"
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

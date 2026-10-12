package reup

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"os"
	"path/filepath"
	"strings"
)

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
	// Style caption (Đợt J): chữ trắng viền đen mờ, canh giữa-dưới —
	// đọc được trên nền video sáng/tối mà không che nhân vật giữa khung.
	style := "force_style='FontSize=24,PrimaryColour=&H00FFFFFF," +
		"OutlineColour=&H99000000,BorderStyle=1,Outline=2,Shadow=0," +
		"MarginV=70,Alignment=2'"
	args := []string{"-i", inPath, "-vf", "subtitles=" + esc + ":" + style}
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

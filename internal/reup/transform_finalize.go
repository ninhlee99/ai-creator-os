package reup

import (
	"context"
	"fmt"
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
	if strings.TrimSpace(t.MusicPath) != "" {
		if _, serr := os.Stat(t.MusicPath); serr == nil {
			args = append(args, "-i", t.MusicPath)
			musicIdx = next
		}
	}
	if voiceIdx < 0 && musicIdx < 0 {
		return fmt.Errorf("reup: thiếu cả voiceover lẫn nhạc nền — không tạo video câm")
	}
	filter := finalFilter(mainDur, voiceIdx, musicIdx)
	// Hình giữ nguyên (đã encode ở pass trước) — chỉ thay audio.
	args = append(args,
		"-filter_complex", filter,
		"-map", "0:v", "-map", "[aout]",
		"-t", fmt.Sprintf("%.2f", mainDur),
		"-c:v", "copy", "-c:a", "aac", "-b:a", "160k",
		"-shortest", tmpPath)
	if err := ffmpegRun(ctx, args...); err != nil {
		return fmt.Errorf("reup: finalize: %w", err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("reup: chốt file: %w", err)
	}
	return nil
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

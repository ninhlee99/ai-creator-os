package studio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var errTikTokSourceTooLarge = errors.New("video TikTok vượt giới hạn tải xuống")

// DownloadTikTokSourceVideo downloads one combined TikTok stream with a hard
// byte cap, including responses whose size is not advertised by the server.
func DownloadTikTokSourceVideo(ctx context.Context, sourceURL, destination string) error {
	sourceURL = strings.TrimSpace(sourceURL)
	if !IsTikTokVideoURL(sourceURL) {
		return errors.New("nguồn video không phải URL TikTok hợp lệ")
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("đường dẫn lưu video không hợp lệ")
	}
	return downloadTikTokSourceVideoWithLimit(ctx, sourceURL, destination, MaxTikTokSourceBytes)
}

func downloadTikTokSourceVideoWithLimit(ctx context.Context, sourceURL, destination string, maxBytes int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes <= 0 {
		return errors.New("giới hạn tải video phải lớn hơn 0")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("tạo thư mục tải video: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".tiktok-source-*.part")
	if err != nil {
		return fmt.Errorf("tạo file tải video tạm: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := &cappedCommandOutput{maxBytes: 64 << 10}
	limited := &boundedTikTokSourceWriter{destination: temp, limit: maxBytes, onLimit: cancel}
	cmd := exec.CommandContext(downloadCtx, "yt-dlp", "--quiet", "--no-progress", "--no-warnings", "--no-playlist", "-f", "b", "-o", "-", sourceURL)
	cmd.Stdout = limited
	cmd.Stderr = output
	commandErr := cmd.Run()
	closeErr := temp.Close()
	if limited.exceeded {
		return fmt.Errorf("%w (%d MiB)", errTikTokSourceTooLarge, maxBytes/(1<<20))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if commandErr != nil {
		message := strings.TrimSpace(output.String())
		if message != "" {
			return fmt.Errorf("yt-dlp tải video thất bại: %s", message)
		}
		return fmt.Errorf("yt-dlp tải video thất bại: %w", commandErr)
	}
	if closeErr != nil {
		return fmt.Errorf("đóng file tải video: %w", closeErr)
	}
	if limited.written == 0 {
		return errors.New("yt-dlp không trả dữ liệu video")
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("thay file video đích: %w", err)
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return fmt.Errorf("lưu video tải xuống: %w", err)
	}
	keepTemp = true
	return nil
}

type boundedTikTokSourceWriter struct {
	destination io.Writer
	limit       int64
	written     int64
	exceeded    bool
	onLimit     func()
}

func (w *boundedTikTokSourceWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.limit-w.written {
		w.exceeded = true
		if w.onLimit != nil {
			w.onLimit()
		}
		return 0, errTikTokSourceTooLarge
	}
	n, err := w.destination.Write(p)
	w.written += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

type cappedCommandOutput struct {
	bytes.Buffer
	maxBytes int
}

func (b *cappedCommandOutput) Write(p []byte) (int, error) {
	written := len(p)
	remaining := b.maxBytes - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return written, nil
}

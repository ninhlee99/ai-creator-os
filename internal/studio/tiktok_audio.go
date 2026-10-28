package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func downloadTikTokRadarAudio(ctx context.Context, sourceURL, workDir string) (string, error) {
	if !IsTikTokVideoURL(sourceURL) {
		return "", fmt.Errorf("nguồn sound phải là video TikTok từ Radar")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	taskCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	videoPath := filepath.Join(workDir, "radar-source.mp4")
	if err := DownloadTikTokSourceVideo(taskCtx, sourceURL, videoPath); err != nil {
		_ = os.Remove(videoPath)
		return "", fmt.Errorf("không tải được video âm thanh từ Radar: %w", err)
	}
	if err := ValidateTikTokSourceFile(videoPath); err != nil {
		_ = os.Remove(videoPath)
		return "", fmt.Errorf("video Radar không hợp lệ để trích âm thanh: %w", err)
	}
	if !ProbeHasAudio(taskCtx, videoPath) {
		_ = os.Remove(videoPath)
		return "", fmt.Errorf("video Radar không có track âm thanh")
	}
	audioPath := filepath.Join(workDir, "radar-source-audio.m4a")
	if err := ffmpegRun(taskCtx, "-y", "-i", videoPath, "-vn", "-c:a", "aac", "-b:a", "192k", audioPath); err != nil {
		return "", fmt.Errorf("không tách được sound từ video Radar: %w", err)
	}
	if !fileExists(audioPath) {
		return "", fmt.Errorf("không tạo được file sound từ video Radar")
	}
	return audioPath, nil
}

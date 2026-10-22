package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MuxMusic replaces/adds the music bed on a silent edit.
func MuxMusic(ctx context.Context, videoPath, musicPath string, musicStart float64, outPath string) error {
	return ffmpegRun(ctx,
		"-i", videoPath, "-stream_loop", "-1", "-ss", fmt.Sprintf("%.1f", musicStart), "-i", musicPath,
		"-map", "0:v", "-map", "1:a",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
		"-shortest", outPath)
}

// MuxSubtitles muxes an SRT subtitle track into a finished video without
// re-encoding (mov_text, Vietnamese language tag). Empty srtText is a no-op
// that copies the input — callers don't branch on "has subtitles".
func MuxSubtitles(ctx context.Context, videoPath, srtText, outPath string) error {
	if strings.TrimSpace(srtText) == "" {
		return ffmpegRun(ctx, "-i", videoPath, "-c", "copy", outPath)
	}
	dir := filepath.Dir(outPath)
	srt := filepath.Join(dir, "subs.srt")
	if err := os.WriteFile(srt, []byte(srtText), 0o644); err != nil {
		return err
	}
	defer os.Remove(srt)
	return ffmpegRun(ctx,
		"-i", videoPath, "-i", srt,
		"-map", "0", "-map", "1",
		"-c:v", "copy", "-c:a", "copy", "-c:s", "mov_text",
		"-metadata:s:s:0", "language=vie",
		outPath)
}

// srtTime formats seconds into standard SRT timestamp 00:00:00,000.
func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(sec*1000 + 0.5)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}

// MixMusicBed mixes a music bed under a film's existing audio. When the
// video has a voice track the music is ducked ~8dB under it
// (sidechaincompress); when the video is silent the music is laid straight.
// The final mix is loudness-normalized to -14 LUFS like the affiliate
// spots. Empty musicPath returns an error — callers check before calling.
func MixMusicBed(ctx context.Context, videoPath, musicPath, outPath string) error {
	if strings.TrimSpace(musicPath) == "" {
		return fmt.Errorf("mix music: thiếu file nhạc")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if !ProbeHasAudio(ctx, videoPath) {
		// Silent film (pure Veo shots): music becomes the audio track.
		return ffmpegRun(ctx,
			"-i", videoPath, "-stream_loop", "-1", "-i", musicPath,
			"-map", "0:v", "-map", "1:a",
			"-c:v", "copy", "-af", "loudnorm=I=-14:TP=-1.5:LRA=11",
			"-c:a", "aac", "-b:a", "192k", "-shortest", outPath)
	}
	// Voice present: duck the bed ~9dB under speech with vocal carving EQ.
	// 1. Highpass & carving EQ on music [1:a] to prevent muddy bass collision with speech.
	// 2. Highpass filter on voice [0:a] to eliminate mic rumble and pops.
	// 3. Smooth sidechaincompress: music ducks cleanly when voice hits, recovers smoothly.
	filter := "[0:a]highpass=f=75[vclean];" +
		"[1:a]highpass=f=100,equalizer=f=1200:t=q:w=1.2:g=-3.5,volume=0.45,atrim=0:3600[m];" +
		"[m][vclean]sidechaincompress=threshold=-24dB:ratio=8:attack=15:release=450:makeup=1[dm];" +
		"[vclean][dm]amix=inputs=2:duration=first:dropout_transition=0," +
		"loudnorm=I=-14:TP=-1.5:LRA=11[aout]"
	return ffmpegRun(ctx,
		"-i", videoPath, "-stream_loop", "-1", "-i", musicPath,
		"-filter_complex", filter,
		"-map", "0:v", "-map", "[aout]",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
		"-shortest", outPath)
}

package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
)

// ffmpegBin locates ffmpeg, which the chain needs for audio conversion
// (Edge MP3 -> WAV, VieNeu resample fallback).
func ffmpegBin() (string, error) {
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH (needed for audio conversion)")
	}
	return p, nil
}

// runFFmpeg runs ffmpeg with -y; on failure it returns the tail of stderr.
func runFFmpeg(ctx context.Context, args ...string) error {
	bin, err := ffmpegBin()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := stderr.String()
		if len(out) > 400 {
			out = out[len(out)-400:]
		}
		return fmt.Errorf("ffmpeg failed: %v: %s", err, out)
	}
	return nil
}

// withTempIO writes in to a temp file, runs fn(inPath, outPath), reads the
// output file and cleans up.
func withTempIO(ctx context.Context, in, inSuffix, outSuffix string, data []byte, fn func(ctx context.Context, inPath, outPath string) error) ([]byte, error) {
	inf, err := os.CreateTemp("", in+"-*"+inSuffix)
	if err != nil {
		return nil, err
	}
	inPath := inf.Name()
	outPath := inPath + outSuffix
	defer os.Remove(inPath)
	defer os.Remove(outPath)
	if _, err := inf.Write(data); err != nil {
		inf.Close()
		return nil, err
	}
	if err := inf.Close(); err != nil {
		return nil, err
	}
	if err := fn(ctx, inPath, outPath); err != nil {
		return nil, err
	}
	return os.ReadFile(outPath)
}

// mp3ToWav24kMono converts MP3 bytes to WAV PCM s16le 24kHz mono.
func mp3ToWav24kMono(ctx context.Context, mp3 []byte) ([]byte, error) {
	return withTempIO(ctx, "edge", ".mp3", ".wav", mp3,
		func(ctx context.Context, inPath, outPath string) error {
			return runFFmpeg(ctx, "-v", "error", "-y", "-i", inPath,
				"-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", outPath)
		})
}

// wavTo24kMono resamples any WAV to PCM s16le 24kHz mono.
func wavTo24kMono(ctx context.Context, wav []byte) ([]byte, error) {
	return withTempIO(ctx, "vieneu", ".wav", ".wav", wav,
		func(ctx context.Context, inPath, outPath string) error {
			return runFFmpeg(ctx, "-v", "error", "-y", "-i", inPath,
				"-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", outPath)
		})
}

// wavParams parses a WAV header: sample rate, channels, bits per sample.
func wavParams(wav []byte) (sampleRate, channels, bits int, err error) {
	if len(wav) < 44 {
		return 0, 0, 0, fmt.Errorf("too short for WAV header (%d bytes)", len(wav))
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return 0, 0, 0, fmt.Errorf("not a WAV file")
	}
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		size := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "fmt " {
			if off+24 > len(wav) {
				return 0, 0, 0, fmt.Errorf("truncated fmt chunk")
			}
			channels = int(binary.LittleEndian.Uint16(wav[off+10 : off+12]))
			sampleRate = int(binary.LittleEndian.Uint32(wav[off+12 : off+16]))
			bits = int(binary.LittleEndian.Uint16(wav[off+22 : off+24]))
			return sampleRate, channels, bits, nil
		}
		off += 8 + size
		if size%2 == 1 {
			off++ // chunk padding
		}
	}
	return 0, 0, 0, fmt.Errorf("fmt chunk not found")
}

// pcmToWav wraps raw PCM s16le mono bytes in a WAV header (port of the
// Python pcm_to_wav helper).
func pcmToWav(pcm []byte, sampleRate int) []byte {
	n := len(pcm)
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+n))
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(hdr[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(hdr[22:], 1)  // mono
	binary.LittleEndian.PutUint32(hdr[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(hdr[32:], 2)                    // block align
	binary.LittleEndian.PutUint16(hdr[34:], 16)                   // bits
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(n))
	return append(hdr, pcm...)
}

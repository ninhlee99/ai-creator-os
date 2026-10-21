package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SayTTSProvider uses the macOS speech engine. It makes no network requests
// and consumes no provider quota. macOS currently exposes the Vietnamese
// Linh voice on the production host.
type SayTTSProvider struct{}

func NewSayTTSProvider() *SayTTSProvider { return &SayTTSProvider{} }

func (p *SayTTSProvider) Name() string { return "say" }

func (p *SayTTSProvider) Healthy(context.Context) bool {
	_, sayErr := exec.LookPath("say")
	_, ffmpegErr := ffmpegBin()
	return sayErr == nil && ffmpegErr == nil
}

func (p *SayTTSProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("say: empty text")
	}
	sayBin, err := exec.LookPath("say")
	if err != nil {
		return nil, fmt.Errorf("say: macOS speech engine unavailable: %w", err)
	}
	if _, err := ffmpegBin(); err != nil {
		return nil, fmt.Errorf("say: %w", err)
	}

	input, err := os.CreateTemp("", "aicos-say-*.txt")
	if err != nil {
		return nil, fmt.Errorf("say: create input: %w", err)
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	if _, err := input.WriteString(text); err != nil {
		input.Close()
		return nil, fmt.Errorf("say: write input: %w", err)
	}
	if err := input.Close(); err != nil {
		return nil, fmt.Errorf("say: close input: %w", err)
	}

	output, err := os.CreateTemp("", "aicos-say-*.aiff")
	if err != nil {
		return nil, fmt.Errorf("say: create output: %w", err)
	}
	outputPath := output.Name()
	defer os.Remove(outputPath)
	if err := output.Close(); err != nil {
		return nil, fmt.Errorf("say: close output: %w", err)
	}

	voiceName := "Linh"
	cmd := exec.CommandContext(ctx, sayBin, "-v", voiceName, "-f", inputPath, "-o", outputPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("say: %w: %s", err, message)
		}
		return nil, fmt.Errorf("say: %w", err)
	}
	aiff, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("say: read output: %w", err)
	}
	return withTempIO(ctx, "aicos-say", ".aiff", ".wav", aiff,
		func(ctx context.Context, inPath, outPath string) error {
			return runFFmpeg(ctx, "-v", "error", "-y", "-i", inPath,
				"-ar", "24000", "-ac", "1", "-c:a", "pcm_s16le", outPath)
		})
}

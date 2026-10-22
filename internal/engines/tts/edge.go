package tts

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	edgeHost     = "speech.platform.bing.com"
	edgeBasePath = "/consumer/speech/synthesize/readaloud/edge/v1"
	edgeUA       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	edgeOrigin   = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfoldn"
)

// persona voice style -> Edge neural voice (port of VI_VOICES).
var edgeVoices = map[string]string{
	"default":          "vi-VN-HoaiMyNeural",
	"warm-female":      "vi-VN-HoaiMyNeural",
	"clear-teacher":    "vi-VN-HoaiMyNeural",
	"singer":           "vi-VN-HoaiMyNeural",
	"upbeat-host":      "vi-VN-HoaiMyNeural",
	"energetic-caster": "vi-VN-NamMinhNeural",
	"male":             "vi-VN-NamMinhNeural",
}

// EdgeTTSProvider is the tier-3 TTS provider: the unofficial Edge read-aloud
// endpoint. No key; last resort only.
type EdgeTTSProvider struct {
	// Rate and Pitch are SSML prosody values, e.g. "+0%", "+0Hz".
	Rate  string
	Pitch string
}

// NewEdgeTTSProvider builds the tier-3 TTS provider.
func NewEdgeTTSProvider() *EdgeTTSProvider {
	return &EdgeTTSProvider{Rate: "+0%", Pitch: "+0Hz"}
}

func (e *EdgeTTSProvider) Name() string { return "edge" }

// Healthy is true when ffmpeg is available for the MP3 -> WAV conversion.
// (Network reachability is only known when actually synthesizing.)
func (e *EdgeTTSProvider) Healthy(ctx context.Context) bool {
	_, err := ffmpegBin()
	return err == nil
}

func (e *EdgeTTSProvider) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("edge: empty text")
	}
	if _, err := ffmpegBin(); err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	var mp3 []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		mp3, err = edgeSynthesize(ctx, text, voice, e.Rate, e.Pitch)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 400 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	wav, err := mp3ToWav24kMono(ctx, mp3)
	if err != nil {
		return nil, fmt.Errorf("edge: %w", err)
	}
	return wav, nil
}

// ---------------------------------------------------------------------------
// minimal WebSocket client (handshake, masked text frames, recv loop)

//go:build parked

package avatar

import (
	"context"
	"time"
)

// RenderOpts controls one offline clip render.
type RenderOpts struct {
	Width  int `json:"width"`  // default 720
	Height int `json:"height"` // default 1280
	FPS    int `json:"fps"`    // default 25
	// Seed overrides the character seed for this render (0 = character seed).
	Seed int64 `json:"seed"`
	// Emotions are expression cues timed to the audio (see Timed).
	Emotions []TimedEmotionCue `json:"emotions"`
	// OutDir receives the finished mp4. "" = provider default (<data>/output).
	OutDir string `json:"-"`
	// MaxDuration caps the clip; longer audio is rejected rather than cut.
	MaxDuration time.Duration `json:"-"`
}

// StreamOpts controls a realtime frame stream (LIVE).
type StreamOpts struct {
	Width  int
	Height int
	FPS    int
	Seed   int64
}

// FrameStream is the realtime contract for LIVE: audio goes in as PCM
// chunks, video frames come out one by one. Only providers with
// SupportsRealtime() == true can open one.
//
// Honest note: on a Mac M1 Pro no local model reaches realtime fps today
// (see docs/RESEARCH.md §6); OpenStream on the local provider
// works when the sidecar runs but frames arrive slower than wall-clock.
// The stream engine must treat it as paced, not realtime, until a GPU
// tier is attached.
type FrameStream interface {
	// WriteAudio feeds one PCM chunk (s16le, mono, 24 kHz).
	WriteAudio(pcm []byte) error
	// ReadFrame returns the next JPEG-encoded frame at the stream's FPS.
	ReadFrame(ctx context.Context) ([]byte, error)
	// Close releases the session (also called on context cancel).
	Close() error
}

// AvatarVideoProvider renders audio-driven talking-head video.
//
// QUALITY CONTRACT: every output frame's motion (lips, eyes, head,
// expression) is synthesized from the audio at that frame. A provider that
// returns a static image with pan/zoom or crossfades violates the contract
// and must fail instead.
type AvatarVideoProvider interface {
	// RenderClip renders a full clip offline: reference portrait + WAV
	// audio (PCM s16le 24kHz mono) + emotion cues -> finished mp4 path.
	// The provider verifies the character's identity lock first.
	RenderClip(ctx context.Context, ch Character, audioWAV []byte, opts RenderOpts) (mp4Path string, err error)
	// SupportsRealtime reports whether OpenStream is available.
	SupportsRealtime() bool
	// OpenStream opens a frame-by-frame stream for LIVE. Only call when
	// SupportsRealtime() is true.
	OpenStream(ctx context.Context, ch Character, opts StreamOpts) (FrameStream, error)
	// Name is the stable provider id used in config, logs and the ledger.
	Name() string
	// Healthy reports whether the provider can serve right now (cheap check).
	Healthy(ctx context.Context) bool
}

// apiKeySetter is implemented by providers whose API keys come from the
// chain config (paid tiers).
type apiKeySetter interface {
	SetAPIKey(string)
	SetAPIKeys([]string)
}

// enabledSetter lets the chain toggle a provider without rebuilding it.
type enabledSetter interface {
	SetEnabled(bool)
}

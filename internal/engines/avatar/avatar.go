// Package avatar is the Go port of engines/avatar/provider.py.
//
// Avatar engine: local stylized realtime -> paid streaming API.
//
// HONEST CONSTRAINT (see docs/ARCHITECTURE.md §5):
// photorealistic + realtime + frame-coherent + free is not production-ready
// today. v1 ships a high-quality STYLIZED realtime avatar with viseme-accurate
// lip-sync driven by TTS timing. The interface is swappable: a paid streaming
// avatar API plugs in later with zero changes to show logic.
//
// v1 "local-stylized" renders a consistent 2D/3D character to a video device
// or frame pipe consumed by the stream engine. Lip-sync uses phoneme/viseme
// timing from the TTS engine, never guessing.
//
// Difference from Python: the Python build_avatar returned a Chain of
// providers. The Go port keeps that fallback in a small internal chain type
// (avatarChain) with identical semantics: try each provider in order, use
// the first whose Speak succeeds.
package avatar

import (
	"context"
	"fmt"
	"strings"
)

// VisemeFrame is one lip-sync keyframe: which mouth shape, when, how
// strongly. Visemes: "A", "E", "MBP", "O", "U", "rest", ...
type VisemeFrame struct {
	AtMs      int
	Viseme    string
	Intensity float64 // 0..1
}

// AvatarProvider renders an avatar speaking the given audio.
type AvatarProvider interface {
	// Speak renders the avatar speaking audio, driven by the viseme
	// timeline. It returns the frame pipe / device the stream engine
	// consumes.
	Speak(ctx context.Context, audio []byte, visemes []VisemeFrame) (pipe string, err error)
	// Name identifies the provider.
	Name() string
	// Healthy reports whether the provider can serve right now.
	Healthy(ctx context.Context) bool
}

// LocalStylizedAvatar is the v1 stylized realtime avatar. The renderer is
// not wired yet (see docs/RESEARCH/tts_avatar.md); Speak fails honestly
// instead of faking frames.
type LocalStylizedAvatar struct {
	Character string
}

// NewLocalStylizedAvatar builds the local stylized avatar for a character.
func NewLocalStylizedAvatar(character string) *LocalStylizedAvatar {
	if character == "" {
		character = "default"
	}
	return &LocalStylizedAvatar{Character: character}
}

func (a *LocalStylizedAvatar) Name() string { return "avatar-local-stylized" }

// Healthy is false until the renderer is wired (see Speak).
func (a *LocalStylizedAvatar) Healthy(_ context.Context) bool { return false }

func (a *LocalStylizedAvatar) Speak(_ context.Context, _ []byte, _ []VisemeFrame) (string, error) {
	// TODO: wire renderer (see docs/RESEARCH/tts_avatar.md).
	// Contract: consumes viseme timeline, outputs coherent frames.
	return "", fmt.Errorf("avatar renderer not wired yet")
}

// StreamingAPIAvatar is a paid streaming avatar API (HeyGen/D-ID class).
// Opt-in, capped.
type StreamingAPIAvatar struct {
	Provider string
	APIKey   string
}

// NewStreamingAPIAvatar builds the paid streaming avatar provider.
func NewStreamingAPIAvatar(provider, apiKey string) *StreamingAPIAvatar {
	return &StreamingAPIAvatar{Provider: provider, APIKey: apiKey}
}

func (a *StreamingAPIAvatar) Name() string { return "avatar-streaming-api" }

// Healthy requires an API key; the provider itself is still unwired.
func (a *StreamingAPIAvatar) Healthy(_ context.Context) bool { return a.APIKey != "" }

func (a *StreamingAPIAvatar) Speak(_ context.Context, _ []byte, _ []VisemeFrame) (string, error) {
	if a.APIKey == "" {
		return "", fmt.Errorf("no API key")
	}
	return "", fmt.Errorf("provider '%s' not wired yet", a.Provider)
}

// avatarChain tries providers in order and uses the first whose Speak
// succeeds — the Go equivalent of Python's Chain([streaming, local]).
type avatarChain struct {
	providers []AvatarProvider
}

func (c *avatarChain) Name() string { return "avatar-chain" }

func (c *avatarChain) Healthy(ctx context.Context) bool {
	for _, p := range c.providers {
		if p.Healthy(ctx) {
			return true
		}
	}
	return false
}

func (c *avatarChain) Speak(ctx context.Context, audio []byte, visemes []VisemeFrame) (string, error) {
	var errs []string
	for _, p := range c.providers {
		pipe, err := p.Speak(ctx, audio, visemes)
		if err == nil {
			return pipe, nil
		}
		errs = append(errs, p.Name()+": "+err.Error())
	}
	return "", fmt.Errorf("all avatar providers failed: %s", strings.Join(errs, "; "))
}

// BuildAvatar builds the avatar provider for the configured backend:
// "streaming-api" tries the paid provider first with the local stylized
// avatar as fallback; anything else (including the "local-stylized"
// default) returns the local stylized avatar.
func BuildAvatar(provider, apiKey, character string) AvatarProvider {
	if provider == "streaming-api" {
		return &avatarChain{providers: []AvatarProvider{
			NewStreamingAPIAvatar(provider, apiKey),
			NewLocalStylizedAvatar(character),
		}}
	}
	return NewLocalStylizedAvatar(character)
}

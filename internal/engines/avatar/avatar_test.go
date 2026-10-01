package avatar

import (
	"context"
	"strings"
	"testing"
)

func TestLocalStylizedNotWired(t *testing.T) {
	a := NewLocalStylizedAvatar("mc-1")
	if a.Name() != "avatar-local-stylized" {
		t.Errorf("name = %q", a.Name())
	}
	if _, err := a.Speak(context.Background(), []byte("wav"),
		[]VisemeFrame{{AtMs: 0, Viseme: "A", Intensity: 0.9}}); err == nil ||
		!strings.Contains(err.Error(), "avatar renderer not wired yet") {
		t.Errorf("expected honest not-wired error, got %v", err)
	}
	if a.Healthy(context.Background()) {
		t.Error("unwired renderer must not report healthy")
	}
}

func TestLocalStylizedDefaultCharacter(t *testing.T) {
	if got := NewLocalStylizedAvatar("").Character; got != "default" {
		t.Errorf("character = %q", got)
	}
}

func TestStreamingAPINoKey(t *testing.T) {
	a := NewStreamingAPIAvatar("heygen", "")
	if _, err := a.Speak(context.Background(), []byte("wav"), nil); err == nil ||
		!strings.Contains(err.Error(), "no API key") {
		t.Errorf("expected no-key error, got %v", err)
	}
	if a.Healthy(context.Background()) {
		t.Error("keyless provider must not report healthy")
	}
}

func TestStreamingAPIWithKey(t *testing.T) {
	a := NewStreamingAPIAvatar("heygen", "secret")
	if !a.Healthy(context.Background()) {
		t.Error("keyed provider should report healthy")
	}
	if _, err := a.Speak(context.Background(), []byte("wav"), nil); err == nil ||
		!strings.Contains(err.Error(), "not wired yet") {
		t.Errorf("expected not-wired error, got %v", err)
	}
}

func TestBuildAvatarLocal(t *testing.T) {
	a := BuildAvatar("local-stylized", "", "mc-1")
	if _, ok := a.(*LocalStylizedAvatar); !ok {
		t.Errorf("expected *LocalStylizedAvatar, got %T", a)
	}
	// Unknown provider falls back to local, like Python's Chain default.
	if _, ok := BuildAvatar("bogus", "", "").(*LocalStylizedAvatar); !ok {
		t.Error("unknown provider should fall back to local")
	}
}

func TestBuildAvatarStreamingChain(t *testing.T) {
	a := BuildAvatar("streaming-api", "secret", "mc-1")
	chain, ok := a.(*avatarChain)
	if !ok {
		t.Fatalf("expected *avatarChain, got %T", a)
	}
	if len(chain.providers) != 2 {
		t.Fatalf("chain length = %d, want 2", len(chain.providers))
	}
	// Both providers fail honestly; the chain reports the aggregate.
	_, err := a.Speak(context.Background(), []byte("wav"), nil)
	if err == nil || !strings.Contains(err.Error(), "all avatar providers failed") {
		t.Errorf("expected aggregate failure, got %v", err)
	}
}

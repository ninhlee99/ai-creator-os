package tts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// stubTTS is a scripted TTS provider for chain tests.
type stubTTS struct {
	name   string
	wav    []byte
	errs   []error
	calls  int
	voices []string
}

func (s *stubTTS) Name() string                     { return s.name }
func (s *stubTTS) Healthy(ctx context.Context) bool { return true }
func (s *stubTTS) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	i := s.calls
	s.calls++
	s.voices = append(s.voices, voice)
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	if s.wav != nil {
		return s.wav, nil
	}
	return []byte("WAV:" + s.name), nil
}

func testTTSConfig(names ...string) ChainConfig {
	var order []ProviderEntry
	for _, n := range names {
		order = append(order, ProviderEntry{Name: n, Enabled: true, Timeout: 5 * time.Second})
	}
	return ChainConfig{Order: order}
}

func TestTTSChainFailover(t *testing.T) {
	vieneu := &stubTTS{name: "vieneu", errs: []error{errors.New("quota exhausted")}}
	gemini := &stubTTS{name: "gemini"}
	var switched [][3]string
	c := NewTTSChain([]TTSProvider{vieneu, gemini}, func(from, to, reason string) {
		switched = append(switched, [3]string{from, to, reason})
	})
	wav, err := c.Synthesize(context.Background(), "xin chào", "default")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(wav) != "WAV:gemini" {
		t.Fatalf("got %q", wav)
	}
	if len(switched) != 1 || switched[0][0] != "vieneu" || switched[0][1] != "gemini" {
		t.Fatalf("onSwitch = %v", switched)
	}
	if switched[0][2] != "quota/rate-limit" {
		t.Fatalf("reason = %q, want quota/rate-limit", switched[0][2])
	}
}

func TestTTSChainSetConfigRuntime(t *testing.T) {
	a := &stubTTS{name: "a"}
	b := &stubTTS{name: "b"}
	c := NewTTSChain([]TTSProvider{a, b}, nil)
	wav, _ := c.Synthesize(context.Background(), "x", "default")
	if string(wav) != "WAV:a" {
		t.Fatalf("before: %q", wav)
	}
	c.SetConfig(testTTSConfig("b", "a"))
	wav, _ = c.Synthesize(context.Background(), "x", "default")
	if string(wav) != "WAV:b" {
		t.Fatalf("after SetConfig: %q, want WAV:b", wav)
	}
}

func TestTTSChainDisabledSkipped(t *testing.T) {
	a := &stubTTS{name: "a"}
	b := &stubTTS{name: "b"}
	c := NewTTSChain([]TTSProvider{a, b}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "a", Enabled: false, Timeout: 5 * time.Second},
		{Name: "b", Enabled: true, Timeout: 5 * time.Second},
	}})
	wav, err := c.Synthesize(context.Background(), "x", "default")
	if err != nil || string(wav) != "WAV:b" {
		t.Fatalf("wav=%q err=%v", wav, err)
	}
	if a.calls != 0 {
		t.Fatalf("disabled provider called %d times", a.calls)
	}
}

func TestTTSChainRetries(t *testing.T) {
	flaky := &stubTTS{name: "flaky", errs: []error{errors.New("e1"), errors.New("e2")}, wav: []byte("late")}
	next := &stubTTS{name: "next"}
	var switched int
	c := NewTTSChain([]TTSProvider{flaky, next}, func(from, to, reason string) { switched++ })
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "flaky", Enabled: true, Timeout: 5 * time.Second, Retries: 2},
		{Name: "next", Enabled: true, Timeout: 5 * time.Second},
	}})
	wav, err := c.Synthesize(context.Background(), "x", "default")
	if err != nil || string(wav) != "late" {
		t.Fatalf("wav=%q err=%v", wav, err)
	}
	if flaky.calls != 3 || switched != 0 || next.calls != 0 {
		t.Fatalf("calls flaky=%d switched=%d next=%d", flaky.calls, switched, next.calls)
	}
}

func TestTTSChainVoicePassedThrough(t *testing.T) {
	a := &stubTTS{name: "a"}
	c := NewTTSChain([]TTSProvider{a}, nil)
	_, _ = c.Synthesize(context.Background(), "x", "warm-female")
	if len(a.voices) != 1 || a.voices[0] != "warm-female" {
		t.Fatalf("voices = %v", a.voices)
	}
}

func TestTTSChainMissingKeyFailsFast(t *testing.T) {
	nokey := &stubTTS{name: "nokey", errs: []error{errors.New("gemini: missing API key")}}
	next := &stubTTS{name: "next"}
	c := NewTTSChain([]TTSProvider{nokey, next}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "nokey", Enabled: true, Timeout: 5 * time.Second, Retries: 5},
		{Name: "next", Enabled: true, Timeout: 5 * time.Second},
	}})
	wav, err := c.Synthesize(context.Background(), "x", "default")
	if err != nil || string(wav) != "WAV:next" {
		t.Fatalf("wav=%q err=%v", wav, err)
	}
	if nokey.calls != 1 {
		t.Fatalf("missing-key provider retried %d times, want 1", nokey.calls)
	}
}

func TestGeminiTTSMissingKey(t *testing.T) {
	g := NewGeminiTTSProvider("")
	if g.Name() != "gemini" {
		t.Fatalf("name = %q", g.Name())
	}
	if g.Healthy(context.Background()) {
		t.Fatal("Healthy should be false without key")
	}
	_, err := g.Synthesize(context.Background(), "xin chào", "default")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Fatalf("err = %v", err)
	}
	g.SetAPIKey("k")
	if !g.Healthy(context.Background()) {
		t.Fatal("Healthy should be true after SetAPIKey")
	}
}

func TestParseGeminiAudio(t *testing.T) {
	// "AQID" = bytes{1,2,3}
	sample := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/pcm","data":"AQID"}}]}}]}`
	pcm, err := parseGeminiAudio([]byte(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pcm) != 3 || pcm[0] != 1 || pcm[1] != 2 || pcm[2] != 3 {
		t.Fatalf("pcm = %v", pcm)
	}
	// snake_case variant
	sample2 := `{"candidates":[{"content":{"parts":[{"inline_data":{"data":"AQID"}}]}}]}`
	if _, err := parseGeminiAudio([]byte(sample2)); err != nil {
		t.Fatalf("snake_case parse: %v", err)
	}
	if _, err := parseGeminiAudio([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`)); err == nil {
		t.Fatal("expected no-audio error")
	}
}

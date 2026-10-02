package avatar

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// stubProvider is a controllable AvatarVideoProvider for chain tests.
type stubProvider struct {
	name      string
	realtime  bool
	healthy   bool
	renderErr error
	streamErr error
	rendered  int
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) SupportsRealtime() bool {
	return s.realtime
}
func (s *stubProvider) Healthy(_ context.Context) bool { return s.healthy }
func (s *stubProvider) RenderClip(_ context.Context, _ Character, _ []byte, _ RenderOpts) (string, error) {
	s.rendered++
	if s.renderErr != nil {
		return "", s.renderErr
	}
	return "/tmp/out.mp4", nil
}
func (s *stubProvider) OpenStream(_ context.Context, _ Character, _ StreamOpts) (FrameStream, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	return nil, errors.New("no stream impl in stub")
}

type decideRec struct {
	agent, action, reason string
}

type recDecider struct{ calls []decideRec }

func (r *recDecider) Decide(agent, action string, _ *string, reason string, _ map[string]any) {
	r.calls = append(r.calls, decideRec{agent, action, reason})
}

func TestChainFirstSuccessWins(t *testing.T) {
	a := &stubProvider{name: "local", healthy: true}
	b := &stubProvider{name: "heygen", healthy: true}
	c := NewAvatarChain([]AvatarVideoProvider{a, b}, nil)
	ch := Character{Name: "x"}
	path, err := c.RenderClip(context.Background(), ch, []byte("wav"), RenderOpts{})
	// character validation fails before providers run (no image) — use a
	// fully valid character instead.
	if err == nil {
		t.Fatalf("expected validation error for imageless character, got path %q", path)
	}
}

func validTestChar(t *testing.T) Character {
	t.Helper()
	path := writeTestImage(t, []byte("img"))
	img, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ch := Character{Name: "MC", ReferenceImage: path, Seed: 5}
	ch.IdentityLock = ComputeIdentityLock(img, 5)
	return ch
}

func TestChainFailoverLogsDecision(t *testing.T) {
	dec := &recDecider{}
	local := &stubProvider{name: "local", healthy: true, renderErr: errors.New("sidecar not running")}
	hey := &stubProvider{name: "heygen", healthy: true}
	c := NewAvatarChain([]AvatarVideoProvider{local, hey}, dec)
	ch := validTestChar(t)
	path, err := c.RenderClip(context.Background(), ch, []byte("wav"), RenderOpts{})
	if err != nil {
		t.Fatalf("failover: %v", err)
	}
	if path != "/tmp/out.mp4" {
		t.Errorf("path = %q", path)
	}
	if hey.rendered != 1 {
		t.Error("heygen should have rendered after local failed")
	}
	if len(dec.calls) != 1 || dec.calls[0].action != "avatar_tier_failover" {
		t.Errorf("failover decision not logged: %+v", dec.calls)
	}
	if !strings.Contains(dec.calls[0].reason, "local -> heygen") {
		t.Errorf("reason = %q", dec.calls[0].reason)
	}
}

func TestChainAllFail(t *testing.T) {
	c := NewAvatarChain([]AvatarVideoProvider{
		&stubProvider{name: "local", renderErr: errors.New("down")},
		&stubProvider{name: "heygen", renderErr: errors.New("no API key")},
	}, nil)
	_, err := c.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{})
	if err == nil || !strings.Contains(err.Error(), "all enabled providers failed") {
		t.Errorf("expected aggregate failure, got %v", err)
	}
}

func TestChainSkipsDisabled(t *testing.T) {
	local := &stubProvider{name: "local", renderErr: errors.New("down")}
	hey := &stubProvider{name: "heygen"}
	c := NewAvatarChain([]AvatarVideoProvider{local, hey}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "local", Enabled: false},
		{Name: "heygen", Enabled: true},
	}})
	if _, err := c.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{}); err != nil {
		t.Fatalf("disabled local must be skipped: %v", err)
	}
	if local.rendered != 0 {
		t.Error("disabled provider must not be called")
	}
}

func TestChainNoEnabled(t *testing.T) {
	c := NewAvatarChain([]AvatarVideoProvider{&stubProvider{name: "local"}}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{{Name: "local", Enabled: false}}})
	_, err := c.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{})
	if err == nil || !strings.Contains(err.Error(), "no enabled providers") {
		t.Errorf("expected no-enabled error, got %v", err)
	}
}

func TestChainIdentityLockEnforced(t *testing.T) {
	c := NewAvatarChain([]AvatarVideoProvider{&stubProvider{name: "local"}}, nil)
	ch := validTestChar(t)
	ch.IdentityLock = "tampered"
	_, err := c.RenderClip(context.Background(), ch, []byte("wav"), RenderOpts{})
	if err == nil || !strings.Contains(err.Error(), "identity lock mismatch") {
		t.Errorf("tampered lock must refuse render, got %v", err)
	}
}

func TestChainSupportsRealtime(t *testing.T) {
	c := NewAvatarChain([]AvatarVideoProvider{
		&stubProvider{name: "local", realtime: false},
		&stubProvider{name: "heygen", realtime: true},
	}, nil)
	if !c.SupportsRealtime() {
		t.Error("chain should report realtime when heygen (enabled) supports it")
	}
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "local", Enabled: true},
		{Name: "heygen", Enabled: false},
	}})
	if c.SupportsRealtime() {
		t.Error("disabled realtime provider must not count")
	}
}

func TestChainTimeout(t *testing.T) {
	slow := &stubProvider{name: "local", renderErr: errors.New("slow")}
	// wrap: make RenderClip block until ctx done
	c := NewAvatarChain([]AvatarVideoProvider{&blockingProvider{err: errors.New("boom")}}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{{Name: "slow", Enabled: true, Timeout: 50 * time.Millisecond}}})
	_ = slow
	_, err := c.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected provider error, got %v", err)
	}
}

// blockingProvider ignores the call and waits for ctx cancellation.
type blockingProvider struct {
	name string
	err  error
}

func (b *blockingProvider) Name() string { return "slow" }
func (b *blockingProvider) SupportsRealtime() bool {
	return false
}
func (b *blockingProvider) Healthy(_ context.Context) bool { return true }
func (b *blockingProvider) RenderClip(ctx context.Context, _ Character, _ []byte, _ RenderOpts) (string, error) {
	<-ctx.Done()
	return "", b.err
}
func (b *blockingProvider) OpenStream(_ context.Context, _ Character, _ StreamOpts) (FrameStream, error) {
	return nil, errors.New("no")
}

func TestDefaultAvatarConfig(t *testing.T) {
	cfg := DefaultAvatarConfig()
	if len(cfg.Order) != 3 {
		t.Fatalf("order = %d, want 3", len(cfg.Order))
	}
	if cfg.Order[0].Name != "local" || !cfg.Order[0].Enabled {
		t.Errorf("local must be first and enabled: %+v", cfg.Order[0])
	}
	for _, e := range cfg.Order[1:] {
		if e.Enabled {
			t.Errorf("%s must be disabled by default", e.Name)
		}
	}
}

func TestPaidStubsHonest(t *testing.T) {
	for _, p := range []AvatarVideoProvider{NewHeyGenProvider(nil), NewDIDProvider(nil)} {
		if p.Healthy(context.Background()) {
			t.Errorf("%s: keyless+disabled must not be healthy", p.Name())
		}
		_, err := p.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{})
		if err == nil || !strings.Contains(err.Error(), "API key") {
			t.Errorf("%s: expected no-key error, got %v", p.Name(), err)
		}
		hk := NewHeyGenProvider([]string{"sk-test"})
		hk.SetEnabled(true)
		if !hk.Healthy(context.Background()) {
			t.Error("keyed+enabled heygen must be healthy")
		}
		_, err = hk.RenderClip(context.Background(), validTestChar(t), []byte("wav"), RenderOpts{})
		if err == nil || !strings.Contains(err.Error(), "chưa được kết nối") {
			t.Errorf("keyed heygen: expected not-wired error, got %v", err)
		}
	}
}

func TestChainJSONRoundTrip(t *testing.T) {
	// The dashboard contract expresses timeouts in seconds; ParseAvatarChainJSON
	// must restore them to durations.
	raw := json.RawMessage(`{"order":[{"name":"local","enabled":true,"timeout":1800,"retries":2},{"name":"heygen","enabled":false,"timeout":120,"retries":1},{"name":"did","enabled":false,"timeout":120,"retries":1}]}`)
	back, err := ParseAvatarChainJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Order) != 3 || back.Order[0].Name != "local" {
		t.Errorf("round trip failed: %+v", back.Order)
	}
	if back.Order[0].Timeout != 30*time.Minute {
		t.Errorf("timeout = %v, want 30m", back.Order[0].Timeout)
	}
}

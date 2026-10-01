package engines

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// stubLLM is a scripted LLM provider for chain tests.
type stubLLM struct {
	name    string
	texts   []string // returned in order; exhausted => last repeats
	errs    []error  // per-call errors; nil entry => success
	calls   int
	healthy bool
}

func (s *stubLLM) Name() string                     { return s.name }
func (s *stubLLM) Healthy(ctx context.Context) bool { return s.healthy }
func (s *stubLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	i := s.calls
	s.calls++
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	if err != nil {
		return "", err
	}
	if len(s.texts) == 0 {
		return "ok", nil
	}
	if i < len(s.texts) {
		return s.texts[i], nil
	}
	return s.texts[len(s.texts)-1], nil
}

func testLLMConfig(names ...string) ChainConfig {
	var order []ProviderEntry
	for _, n := range names {
		order = append(order, ProviderEntry{Name: n, Enabled: true, Timeout: 5 * time.Second})
	}
	return ChainConfig{Order: order}
}

func TestLLMChainFailover(t *testing.T) {
	bad := &stubLLM{name: "bad", errs: []error{errors.New("boom")}}
	good := &stubLLM{name: "good", texts: []string{"hello"}}
	var switched [][3]string
	c := NewLLMChain([]LLMProvider{bad, good}, func(from, to, reason string) {
		switched = append(switched, [3]string{from, to, reason})
	})
	text, err := c.Complete(context.Background(), "sys", "hi")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "hello" {
		t.Fatalf("got %q, want %q", text, "hello")
	}
	if len(switched) != 1 || switched[0][0] != "bad" || switched[0][1] != "good" {
		t.Fatalf("onSwitch calls = %v, want [[bad good *]]", switched)
	}
	if bad.calls != 1 || good.calls != 1 {
		t.Fatalf("calls: bad=%d good=%d", bad.calls, good.calls)
	}
}

func TestLLMChainAllFail(t *testing.T) {
	a := &stubLLM{name: "a", errs: []error{errors.New("x")}}
	b := &stubLLM{name: "b", errs: []error{errors.New("y")}}
	c := NewLLMChain([]LLMProvider{a, b}, nil)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLLMChainNoEnabled(t *testing.T) {
	a := &stubLLM{name: "a", texts: []string{"x"}}
	c := NewLLMChain([]LLMProvider{a}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{{Name: "a", Enabled: false}}})
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil || !strings.Contains(err.Error(), "no enabled providers") {
		t.Fatalf("err = %v, want no-enabled-providers", err)
	}
	if a.calls != 0 {
		t.Fatalf("disabled provider was called %d times", a.calls)
	}
}

func TestLLMChainSetConfigRuntime(t *testing.T) {
	a := &stubLLM{name: "a", texts: []string{"A"}}
	b := &stubLLM{name: "b", texts: []string{"B"}}
	c := NewLLMChain([]LLMProvider{a, b}, nil)

	text, err := c.Complete(context.Background(), "", "hi")
	if err != nil || text != "A" {
		t.Fatalf("before SetConfig: text=%q err=%v", text, err)
	}
	// Reorder at runtime: b first. Must take effect immediately, no restart.
	c.SetConfig(testLLMConfig("b", "a"))
	text, err = c.Complete(context.Background(), "", "hi")
	if err != nil || text != "B" {
		t.Fatalf("after SetConfig: text=%q err=%v, want B", text, err)
	}
	if got := c.ActiveProviders(); len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("ActiveProviders = %v", got)
	}
}

func TestLLMChainDisabledSkipped(t *testing.T) {
	a := &stubLLM{name: "a", texts: []string{"A"}}
	b := &stubLLM{name: "b", texts: []string{"B"}}
	c := NewLLMChain([]LLMProvider{a, b}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "a", Enabled: false, Timeout: 5 * time.Second},
		{Name: "b", Enabled: true, Timeout: 5 * time.Second},
	}})
	text, err := c.Complete(context.Background(), "", "hi")
	if err != nil || text != "B" {
		t.Fatalf("text=%q err=%v, want B", text, err)
	}
	if a.calls != 0 {
		t.Fatalf("disabled provider called %d times", a.calls)
	}
}

func TestLLMChainRetries(t *testing.T) {
	// Fails twice, then succeeds. Retries=2 => 3 attempts, no failover.
	flaky := &stubLLM{name: "flaky", errs: []error{errors.New("e1"), errors.New("e2")}, texts: []string{"late"}}
	next := &stubLLM{name: "next", texts: []string{"next"}}
	var switched int
	c := NewLLMChain([]LLMProvider{flaky, next}, func(from, to, reason string) { switched++ })
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "flaky", Enabled: true, Timeout: 5 * time.Second, Retries: 2},
		{Name: "next", Enabled: true, Timeout: 5 * time.Second},
	}})
	text, err := c.Complete(context.Background(), "", "hi")
	if err != nil || text != "late" {
		t.Fatalf("text=%q err=%v, want late", text, err)
	}
	if flaky.calls != 3 {
		t.Fatalf("flaky calls = %d, want 3", flaky.calls)
	}
	if switched != 0 || next.calls != 0 {
		t.Fatalf("unexpected failover: switched=%d next.calls=%d", switched, next.calls)
	}

	// Retries=1 => only 2 attempts, still failing => failover to next.
	flaky2 := &stubLLM{name: "flaky", errs: []error{errors.New("e1"), errors.New("e2")}, texts: []string{"late"}}
	next2 := &stubLLM{name: "next", texts: []string{"next"}}
	c2 := NewLLMChain([]LLMProvider{flaky2, next2}, func(from, to, reason string) { switched++ })
	c2.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "flaky", Enabled: true, Timeout: 5 * time.Second, Retries: 1},
		{Name: "next", Enabled: true, Timeout: 5 * time.Second},
	}})
	text, err = c2.Complete(context.Background(), "", "hi")
	if err != nil || text != "next" {
		t.Fatalf("text=%q err=%v, want next", text, err)
	}
}

func TestLLMChainMissingKeyFailsFast(t *testing.T) {
	nokey := &stubLLM{name: "nokey", errs: []error{errors.New("gemini: missing API key")}}
	next := &stubLLM{name: "next", texts: []string{"next"}}
	c := NewLLMChain([]LLMProvider{nokey, next}, nil)
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "nokey", Enabled: true, Timeout: 5 * time.Second, Retries: 5},
		{Name: "next", Enabled: true, Timeout: 5 * time.Second},
	}})
	text, err := c.Complete(context.Background(), "", "hi")
	if err != nil || text != "next" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if nokey.calls != 1 {
		t.Fatalf("missing-key provider retried %d times, want 1 (fail fast)", nokey.calls)
	}
}

func TestLLMChainAttemptTimeout(t *testing.T) {
	slow := &stubLLM{name: "slow", texts: []string{"slow-ok"}}
	// make it actually slow by blocking on ctx
	slowP := &blockingLLM{name: "slow"}
	fast := &stubLLM{name: "fast", texts: []string{"fast"}}
	var reasons []string
	c := NewLLMChain([]LLMProvider{slowP, fast}, func(from, to, reason string) { reasons = append(reasons, reason) })
	_ = slow
	c.SetConfig(ChainConfig{Order: []ProviderEntry{
		{Name: "slow", Enabled: true, Timeout: 100 * time.Millisecond},
		{Name: "fast", Enabled: true, Timeout: 5 * time.Second},
	}})
	text, err := c.Complete(context.Background(), "", "hi")
	if err != nil || text != "fast" {
		t.Fatalf("text=%q err=%v, want fast", text, err)
	}
	if len(reasons) != 1 || reasons[0] != "timeout" {
		t.Fatalf("reasons = %v, want [timeout]", reasons)
	}
}

type blockingLLM struct{ name string }

func (b *blockingLLM) Name() string                     { return b.name }
func (b *blockingLLM) Healthy(ctx context.Context) bool { return true }
func (b *blockingLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(30 * time.Second):
		return "too late", nil
	}
}

func TestParseGeminiText(t *testing.T) {
	sample := `{"candidates":[{"content":{"parts":[{"text":"Xin chào cả nhà"}]}}]}`
	text, err := parseGeminiText([]byte(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if text != "Xin chào cả nhà" {
		t.Fatalf("got %q", text)
	}
	if _, err := parseGeminiText([]byte(`{"error":{"message":"quota gone"}}`)); err == nil {
		t.Fatal("expected api error")
	}
	if _, err := parseGeminiText([]byte(`{"candidates":[]}`)); err == nil {
		t.Fatal("expected no-candidates error")
	}
}

func TestParseChatCompletion(t *testing.T) {
	sample := `{"choices":[{"message":{"role":"assistant","content":"Chào bạn"}}]}`
	text, err := parseChatCompletion([]byte(sample))
	if err != nil || text != "Chào bạn" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestGeminiMissingKey(t *testing.T) {
	g := NewGeminiProvider("")
	if g.Healthy(context.Background()) {
		t.Fatal("Healthy should be false without key")
	}
	_, err := g.Complete(context.Background(), "", "hi")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Fatalf("err = %v, want missing API key", err)
	}
	g.SetAPIKey("k")
	if !g.Healthy(context.Background()) {
		t.Fatal("Healthy should be true after SetAPIKey")
	}
}

func TestPaidProvider(t *testing.T) {
	p := NewPaidLLMProvider()
	if p.Name() != "paid" {
		t.Fatalf("name = %q", p.Name())
	}
	if p.Healthy(context.Background()) {
		t.Fatal("paid should be unhealthy by default")
	}
	_, err := p.Complete(context.Background(), "", "hi")
	if err == nil || !strings.Contains(err.Error(), "paid tier disabled") {
		t.Fatalf("err = %v, want 'paid tier disabled'", err)
	}
}

func TestDefaultLLMConfig(t *testing.T) {
	cfg := DefaultLLMConfig([]string{"key"})
	if len(cfg.Order) != 3 {
		t.Fatalf("order len = %d", len(cfg.Order))
	}
	if cfg.Order[0].Name != "gemini" || cfg.Order[1].Name != "llama-server" || cfg.Order[2].Name != "paid" {
		t.Fatalf("order = %v", cfg.Order)
	}
	if !cfg.Order[0].Enabled || !cfg.Order[1].Enabled {
		t.Fatal("gemini and llama-server should be enabled")
	}
	if cfg.Order[2].Enabled {
		t.Fatal("paid should be disabled")
	}
	if cfg.Order[0].APIKey != "key" {
		t.Fatalf("gemini key = %q", cfg.Order[0].APIKey)
	}
}

func TestFailReason(t *testing.T) {
	cases := map[string]string{
		"gemini: HTTP 429: quota exceeded": "quota/rate-limit",
		"rate limit hit":                   "quota/rate-limit",
		"context deadline exceeded":        "timeout",
		"gemini: missing API key":          "missing-key",
		"dial tcp: connection refused":     "unreachable",
		"something else broke":             "error",
	}
	for in, want := range cases {
		if got := failReason(errors.New(in)); got != want {
			t.Errorf("failReason(%q) = %q, want %q", in, got, want)
		}
	}
}

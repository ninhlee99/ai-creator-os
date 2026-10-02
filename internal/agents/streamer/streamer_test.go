//go:build parked

package streamer

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"

	_ "modernc.org/sqlite"
)

func testCfg() governance.Config {
	return governance.Config{
		DryRun:                   true, // 1s pacing per segment
		MaxLiveMinutesPerSession: 120,
		AIDisclosureText:         "AI-generated stream",
	}
}

func testLedger(t *testing.T) (*ledger.Ledger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := ledger.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, path
}

type fakeLLM struct{ lastPrompt string }

func (f *fakeLLM) Complete(_ context.Context, _, prompt string) (string, error) {
	f.lastPrompt = prompt
	return "Xin chào cả nhà, cùng chơi game nào!", nil
}
func (f *fakeLLM) Name() string                   { return "fake-llm" }
func (f *fakeLLM) Healthy(_ context.Context) bool { return true }

type fakeTTS struct{}

func (fakeTTS) Synthesize(_ context.Context, _, _ string) ([]byte, error) {
	return []byte("RIFF....fake-wav"), nil
}
func (fakeTTS) Name() string                   { return "fake-tts" }
func (fakeTTS) Healthy(_ context.Context) bool { return true }

type fakeAvatar struct{ speaks int }

func (f *fakeAvatar) RenderSegment(_ context.Context, _ []byte) ([]byte, error) {
	f.speaks++
	return []byte("fake-mp4"), nil
}

type fakeEngine struct{ started, stopped, pushed int }

func (f *fakeEngine) Start(_, _, _ string) error { f.started++; return nil }
func (f *fakeEngine) Stop() error                { f.stopped++; return nil }
func (f *fakeEngine) PushSegment(_ []byte, _ string) error {
	f.pushed++
	return nil
}

func withShortPlan(t *testing.T) {
	t.Helper()
	old := SegmentPlan
	SegmentPlan = []Segment{{Kind: "welcome", Minutes: 1}, {Kind: "closing", Minutes: 1}}
	t.Cleanup(func() { SegmentPlan = old })
}

func countEvents(t *testing.T, dbPath string) int {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM live_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunLiveDryRun(t *testing.T) {
	l, dbPath := testLedger(t)
	withShortPlan(t)
	llm := &fakeLLM{}
	av := &fakeAvatar{}
	se := &fakeEngine{}

	res := RunLive(context.Background(), testCfg(), l, llm, fakeTTS{}, av, se, 0)
	if res["ok"] != true {
		t.Fatalf("ok = %v (%v)", res["ok"], res["error"])
	}
	if res["minutes"] != 2 {
		t.Errorf("minutes = %v, want 2", res["minutes"])
	}
	if av.speaks != 2 || se.pushed != 2 {
		t.Errorf("speaks=%d pushed=%d, want 2/2", av.speaks, se.pushed)
	}
	if se.started != 1 || se.stopped != 1 {
		t.Errorf("started=%d stopped=%d, want 1/1", se.started, se.stopped)
	}
	if !strings.Contains(llm.lastPrompt, "closing") {
		t.Errorf("director never saw closing segment: %q", llm.lastPrompt)
	}
	// LogEvent must have been called: started + 2 segments = 3 rows.
	if n := countEvents(t, dbPath); n < 3 {
		t.Errorf("live_events rows = %d, want >= 3", n)
	}
}

func TestRunLiveGovernanceStopsSession(t *testing.T) {
	l, _ := testLedger(t)
	withShortPlan(t)
	cfg := testCfg()
	cfg.MaxLiveMinutesPerSession = 1 // only the first segment fits
	av := &fakeAvatar{}
	se := &fakeEngine{}

	res := RunLive(context.Background(), cfg, l, &fakeLLM{}, fakeTTS{}, av, se, 0)
	if res["ok"] != true {
		t.Fatalf("ok = %v (%v)", res["ok"], res["error"])
	}
	if res["minutes"] != 1 {
		t.Errorf("minutes = %v, want 1 (governance stop)", res["minutes"])
	}
	if se.pushed != 1 || se.stopped != 1 {
		t.Errorf("pushed=%d stopped=%d", se.pushed, se.stopped)
	}
}

func TestRunLiveMaxMinutesOverride(t *testing.T) {
	l, _ := testLedger(t)
	withShortPlan(t)
	av := &fakeAvatar{}
	se := &fakeEngine{}
	// maxMinutes=1 overrides the generous config limit.
	res := RunLive(context.Background(), testCfg(), l, &fakeLLM{}, fakeTTS{}, av, se, 1)
	if res["ok"] != true || res["minutes"] != 1 {
		t.Errorf("res = %v", res)
	}
}

func TestDirectSegment(t *testing.T) {
	llm := &fakeLLM{}
	script, err := DirectSegment(context.Background(), llm, "game", "featured_product=chưa có")
	if err != nil {
		t.Fatal(err)
	}
	if script == "" {
		t.Error("empty script")
	}
	if !strings.Contains(llm.lastPrompt, "game") {
		t.Errorf("prompt missing kind: %q", llm.lastPrompt)
	}
}

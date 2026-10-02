package web

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
)

// TestKillSwitchStopsDaemon is the cross-layer safety contract: pressing
// the dashboard Kill button (POST /kill) must stop the network daemon on
// its next tick — no restart, no second switch. The daemon is wired the
// way cmd/aicos/main.go wires it in production.
func TestKillSwitchStopsDaemon(t *testing.T) {
	s := newTestServer(t)
	s.Cfg.SetDryRun(false) // operator has enabled live ops

	// Daemon wired exactly like cmd/aicos/main.go: the shared web
	// *Config is its Gate, so dashboard switches are read live.
	cfg := network.DefaultNetConfig()
	cfg.MasterSwitch = true
	cfg.DryRun = false
	cfg.Gate = s.Cfg
	d := network.NewDaemon(s.Ledger, s.Mgr, nil, cfg)

	// One live supposedly on air (slot far from its end so the tick must
	// not stop it for any reason other than the kill switch).
	stops := 0
	d.OnStopLive = func(any) { stops++ }
	d.State().Running[42] = network.RunningSlot{
		Slot:   ledger.Slot{ID: 42, AccountID: 1, StartMin: 1439, DurationMin: 60},
		Handle: "handle-42",
	}

	// Baseline: without kill, the tick leaves the on-air live alone.
	sum, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if sum.StoppedAll || stops != 0 {
		t.Fatalf("pre-kill tick stopped the live: %+v stops=%d", sum, stops)
	}

	// Operator hits the dashboard Kill button.
	if rec := postForm(t, s, "/kill", url.Values{}); rec.Code != 303 {
		t.Fatalf("POST /kill = %d, want 303", rec.Code)
	}
	if !s.Cfg.KillSwitch() {
		t.Fatal("web config did not record the kill")
	}

	sum, err = d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick after kill: %v", err)
	}
	if !sum.StoppedAll || stops != 1 {
		t.Fatalf("daemon ignored the dashboard kill switch: summary=%+v stops=%d", sum, stops)
	}
}

// TestDryRunGateBlocksDaemonStart proves the other half of the shared
// gate: while the dashboard dry-run is on, the daemon must not start a
// due live slot; turning it off through the Settings handler (POST
// /settings/dryrun) must let the very next tick start it.
func TestDryRunGateBlocksDaemonStart(t *testing.T) {
	s := newTestServer(t) // LoadConfig default: dry-run ON

	cfg := network.DefaultNetConfig()
	cfg.MasterSwitch = true
	cfg.Gate = s.Cfg
	d := network.NewDaemon(s.Ledger, s.Mgr, nil, cfg)

	// A live_ready account with a due slot, mirroring daemon_test setup.
	t.Setenv("TEST_GATE_RTMP", "key-abc")
	a, err := s.Mgr.Add("liveacc", "", "TEST_GATE_RTMP")
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"researching", "persona_assigned", "growing", "live_ready"} {
		if _, err := s.Mgr.Transition(a.ID, to, nil); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	if err := s.Ledger.SetFollowers(a.ID, 1500); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	date := now.Format("2006-01-02")
	start := now.Hour()*60 + now.Minute() - 5
	if start < 0 {
		start = 0
	}
	if err := s.Ledger.SaveSlots([]ledger.Slot{{
		AccountID: a.ID, SlotDate: date,
		StartMin: start, DurationMin: 90, Status: "planned",
	}}); err != nil {
		t.Fatal(err)
	}
	d.State().LastSlotDate = date // skip the daily-plan rebuild

	starts := 0
	d.OnStartLive = func(acct *network.Account, slot ledger.Slot, key string) (any, error) {
		starts++
		return "handle", nil
	}

	// Dry-run on (dashboard default): the due slot must NOT start.
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("tick (dry-run): %v", err)
	}
	if starts != 0 {
		t.Fatalf("daemon started a live during dry-run (starts=%d)", starts)
	}

	// Operator turns dry-run off in Settings.
	if rec := postForm(t, s, "/settings/dryrun", url.Values{"value": {"off"}}); rec.Code != 303 {
		t.Fatalf("POST /settings/dryrun = %d, want 303", rec.Code)
	}
	if s.Cfg.DryRun() {
		t.Fatal("web config still in dry-run after toggle")
	}

	sum, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick (live): %v", err)
	}
	if starts != 1 || len(sum.Started) != 1 {
		t.Fatalf("daemon did not start the due slot after dry-run off: starts=%d summary=%+v", starts, sum)
	}
}

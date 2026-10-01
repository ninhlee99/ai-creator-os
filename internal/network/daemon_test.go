package network

import (
	"context"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// liveReadyAccount creates an account walked all the way to live_ready.
func liveReadyAccount(t *testing.T, l *ledger.Ledger, mgr *AccountManager, username, rtmpRef string) *Account {
	t.Helper()
	a, err := mgr.Add(username, "", rtmpRef)
	if err != nil {
		t.Fatal(err)
	}
	a = walkChain(t, mgr, a, "researching", "persona_assigned", "growing", "live_ready")
	if err := l.SetFollowers(a.ID, 1500); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDaemonTickStartsDueSlot(t *testing.T) {
	l, mgr := newTestManager(t)
	t.Setenv("TEST_DAEMON_RTMP", "key-abc")
	a := liveReadyAccount(t, l, mgr, "liveacc", "TEST_DAEMON_RTMP")

	date, _, nowMin := todayICT("Asia/Ho_Chi_Minh")
	start := nowMin - 5
	if start < 0 {
		start = 0
	}
	if err := l.SaveSlots([]ledger.Slot{{
		AccountID: a.ID, SlotDate: date,
		StartMin: start, DurationMin: 90, Status: "planned",
	}}); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultNetConfig()
	cfg.MasterSwitch = true
	cfg.DryRun = false
	d := NewDaemon(l, mgr, nil, cfg)
	// Skip the daily-plan rebuild so only the hand-made slot is due.
	d.State().LastSlotDate = date

	var started []string
	var gotKey string
	d.OnStartLive = func(acct *Account, slot ledger.Slot, key string) (any, error) {
		started = append(started, acct.Username)
		gotKey = key
		return "handle-1", nil
	}
	stops := 0
	d.OnStopLive = func(handle any) { stops++ }

	sum, err := d.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != "liveacc" {
		t.Fatalf("started = %v, want [liveacc]", started)
	}
	if gotKey != "key-abc" {
		t.Fatalf("rtmp key = %q, want key-abc", gotKey)
	}
	got, _ := mgr.Get(a.ID)
	if got.Status != "live" {
		t.Fatalf("account status = %s, want live", got.Status)
	}
	slots, _ := l.GetSlots(date)
	startedOK := false
	for _, s := range slots {
		if s.AccountID == a.ID && s.StartMin == start && s.Status == "started" {
			startedOK = true
		}
	}
	if !startedOK {
		t.Fatalf("due slot was not marked started: %+v", slots)
	}

	// Kill switch stops everything, now.
	d.cfg.KillSwitch = true
	sum2, err := d.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stops != 1 {
		t.Fatalf("stops = %d, want 1", stops)
	}
	if !sum2.StoppedAll {
		t.Error("expected StoppedAll after kill switch")
	}
	if len(d.State().Running) != 0 {
		t.Error("running map not drained after kill switch")
	}
	_ = sum
}

func TestDaemonTickDryRunNeverStarts(t *testing.T) {
	l, mgr := newTestManager(t)
	t.Setenv("TEST_DAEMON_RTMP2", "key-xyz")
	a := liveReadyAccount(t, l, mgr, "dryacc", "TEST_DAEMON_RTMP2")

	date, _, nowMin := todayICT("Asia/Ho_Chi_Minh")
	start := nowMin - 5
	if start < 0 {
		start = 0
	}
	if err := l.SaveSlots([]ledger.Slot{{
		AccountID: a.ID, SlotDate: date,
		StartMin: start, DurationMin: 90, Status: "planned",
	}}); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultNetConfig()
	cfg.MasterSwitch = true // DryRun stays true
	d := NewDaemon(l, mgr, nil, cfg)
	d.OnStartLive = func(acct *Account, slot ledger.Slot, key string) (any, error) {
		t.Fatal("OnStartLive must not be called in dry-run")
		return nil, nil
	}
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.Get(a.ID)
	if got.Status != "live_ready" {
		t.Fatalf("account status = %s, want live_ready (dry-run must not start)", got.Status)
	}
}

func TestDaemonTickMasterOff(t *testing.T) {
	_, mgr := newTestManager(t)
	d := NewDaemon(nil, mgr, nil, DefaultNetConfig()) // master off by default
	sum, err := d.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sum.StoppedAll || len(sum.Started) != 0 || len(sum.Onboarded) != 0 {
		t.Fatalf("master-off tick should be a no-op: %+v", sum)
	}
}

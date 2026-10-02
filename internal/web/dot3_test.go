package web

// Đợt 3 — "Zero-touch là mặc định": automation switches default ON when
// unset, a stored OFF always wins, DRY-RUN/kill stay the safety gates,
// the live schedule builds itself, and the homepage carries the two
// aggregate stats orphaned by the /analytics dissolution.

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

// ------------------------------------------------------- setting semantics

func TestGrowthProductionSettingSemantics(t *testing.T) {
	s := newTestServer(t)
	if !s.GrowthProductionEnabled() {
		t.Error("unset: GrowthProductionEnabled() = false, want true (default ON)")
	}
	if err := s.Ledger.SetSetting(SettingGrowthProduction, "0"); err != nil {
		t.Fatal(err)
	}
	if s.GrowthProductionEnabled() {
		t.Error("stored 0: enabled = true, want false (stored OFF respected)")
	}
	if err := s.Ledger.SetSetting(SettingGrowthProduction, "1"); err != nil {
		t.Fatal(err)
	}
	if !s.GrowthProductionEnabled() {
		t.Error("stored 1: enabled = false, want true")
	}
}

func TestAutopilotSwitchesDefaultOn(t *testing.T) {
	s := newTestServer(t)
	// R2-W4: the autopilot switches live in the single settings facade
	// (ledger settings), not the products store.

	enabled, _, _, _, autoPub := s.scheduleView()
	if !enabled || !autoPub {
		t.Fatalf("unset switches: enabled=%v autoPublish=%v, want both ON", enabled, autoPub)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotAutoPublish, "0"); err != nil {
		t.Fatal(err)
	}
	enabled, _, _, _, autoPub = s.scheduleView()
	if enabled || autoPub {
		t.Fatalf("stored 0: enabled=%v autoPublish=%v, want both OFF", enabled, autoPub)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if enabled, _, _, _, _ = s.scheduleView(); !enabled {
		t.Error("stored 1: enabled = false, want ON")
	}
}

// ------------------------------------------------------------ growth tick

func TestGrowthTickUnsetDefaultProduces(t *testing.T) {
	s, fp, _ := newGrowthServer(t) // dry-run off; production setting untouched
	a := growthAccount(t, s, "default_on")
	seedPlan(t, s, a.ID, draftOn(s.today(), growth.VariantTikTok, "truyện ma — tập 1"))

	s.GrowthAutomationTick(context.Background())

	if len(fp.enqueued) != 1 {
		t.Fatalf("unset default: enqueued = %d, want 1", len(fp.enqueued))
	}
	if it := itemByVariant(t, s, a.ID, growth.VariantTikTok); it.Status != growth.ItemProduced {
		t.Errorf("item = %s, want produced", it.Status)
	}
}

func TestGrowthTickKillSwitchWins(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	s.Cfg.SetKillSwitch(true)
	a := growthAccount(t, s, "kill_wins")
	seedPlan(t, s, a.ID, draftOn(s.today(), growth.VariantTikTok, "truyện ma — tập 1"))

	notes := s.GrowthAutomationTick(context.Background())

	if len(fp.enqueued) != 0 {
		t.Errorf("kill switch on but %d jobs enqueued", len(fp.enqueued))
	}
	if joined := strings.Join(notes, " "); !strings.Contains(joined, "Kill switch") {
		t.Errorf("tick notes = %q, want the kill-switch note", joined)
	}
	if it := itemByVariant(t, s, a.ID, growth.VariantTikTok); it.Status != growth.ItemPlanned {
		t.Errorf("item = %s, want planned (untouched under kill)", it.Status)
	}
}

// ------------------------------------------------------------- homepage

func TestDashboardRestoredStats(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Ledger.RecordCommission("2026-10", 150000, "dot3-test"); err != nil {
		t.Fatalf("record commission: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Ledger.StartSession(nil); err != nil {
			t.Fatalf("start session: %v", err)
		}
	}
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Hoa hồng đã ghi nhận", "150.000 ₫", "Phiên live đã chạy"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing restored stat %q", want)
		}
	}
}

func TestDashboardAutoBuildsSchedule(t *testing.T) {
	s := newTestServer(t)
	a, err := s.Mgr.Add("auto_sched", "", "")
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
	// Today must not be the account's rest day, whatever weekday the
	// test happens to run on.
	wd := (int(time.Now().In(s.location()).Weekday()) + 6) % 7
	if _, err := s.db.Exec("UPDATE accounts SET rest_weekday=? WHERE id=?", (wd+1)%7, a.ID); err != nil {
		t.Fatal(err)
	}

	if slots, _ := s.Ledger.GetSlots(s.today()); len(slots) != 0 {
		t.Fatalf("precondition: %d slots before any page view", len(slots))
	}
	if rec := get(t, s, "/"); rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	slots, err := s.Ledger.GetSlots(s.today())
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 {
		t.Fatal("opening the homepage did not auto-build today's schedule")
	}
	// A second view (another page that ensures) must not duplicate.
	if rec := get(t, s, "/schedule"); rec.Code != http.StatusOK {
		t.Fatalf("GET /schedule = %d, want 200", rec.Code)
	}
	if slots2, _ := s.Ledger.GetSlots(s.today()); len(slots2) != len(slots) {
		t.Errorf("slots changed across views: %d -> %d", len(slots), len(slots2))
	}
}

// ------------------------------------------------------ local model ensure

type fakeVieNeuCtl struct {
	state string
	calls atomic.Int32
}

func (f *fakeVieNeuCtl) Status() (string, string) { return f.state, "fake" }
func (f *fakeVieNeuCtl) Start(context.Context) error {
	return nil
}
func (f *fakeVieNeuCtl) Stop() error { return nil }
func (f *fakeVieNeuCtl) Restart(context.Context) error {
	return nil
}
func (f *fakeVieNeuCtl) SetVoice(string) error { return nil }
func (f *fakeVieNeuCtl) EnsureModel(context.Context, func(int64, int64)) error {
	f.calls.Add(1)
	return nil
}
func (f *fakeVieNeuCtl) Voices() []string { return nil }

type fakeAvatarCtl struct {
	configured, present bool
	calls               atomic.Int32
}

func (f *fakeAvatarCtl) Status() (string, string) { return "stopped", "fake" }
func (f *fakeAvatarCtl) Start(context.Context) error {
	return nil
}
func (f *fakeAvatarCtl) Stop() error { return nil }
func (f *fakeAvatarCtl) Restart(context.Context) error {
	return nil
}
func (f *fakeAvatarCtl) EnsureModel(context.Context, func(int64, int64)) error {
	f.calls.Add(1)
	return nil
}
func (f *fakeAvatarCtl) ModelConfigured() bool { return f.configured }
func (f *fakeAvatarCtl) ModelPresent() bool    { return f.present }

func TestEnsureLocalModels(t *testing.T) {
	s := newTestServer(t)
	vn := &fakeVieNeuCtl{state: "stopped"}
	av := &fakeAvatarCtl{configured: true, present: false}
	s.VieNeu = vn
	s.AvatarSidecar = av

	s.EnsureLocalModels(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (vn.calls.Load() == 0 || av.calls.Load() == 0) {
		time.Sleep(10 * time.Millisecond)
	}
	if vn.calls.Load() == 0 {
		t.Error("vieneu ensure not kicked at startup (sidecar stopped)")
	}
	if av.calls.Load() == 0 {
		t.Error("avatar ensure not kicked at startup (model missing)")
	}

	// Already-ready runtimes must not be kicked.
	s2 := newTestServer(t)
	vn2 := &fakeVieNeuCtl{state: "running"}
	av2 := &fakeAvatarCtl{configured: true, present: true}
	s2.VieNeu = vn2
	s2.AvatarSidecar = av2
	s2.EnsureLocalModels(context.Background())
	time.Sleep(150 * time.Millisecond)
	if vn2.calls.Load() != 0 || av2.calls.Load() != 0 {
		t.Errorf("ready runtimes kicked: vieneu=%d avatar=%d, want 0/0",
			vn2.calls.Load(), av2.calls.Load())
	}
}

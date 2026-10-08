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

	enabled, _, _, _, autoPub, youTube := s.scheduleView()
	if !enabled || !autoPub || !youTube {
		t.Fatalf("unset switches: enabled=%v autoPublish=%v youTube=%v, want all ON", enabled, autoPub, youTube)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotAutoPublish, "0"); err != nil {
		t.Fatal(err)
	}
	enabled, _, _, _, autoPub, _ = s.scheduleView()
	if enabled || autoPub {
		t.Fatalf("stored 0: enabled=%v autoPublish=%v, want both OFF", enabled, autoPub)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if enabled, _, _, _, _, _ = s.scheduleView(); !enabled {
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
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Hoa hồng đã ghi nhận", "150.000 ₫"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing restored stat %q", want)
		}
	}
	for _, gone := range []string{"Phiên live", "Lịch live", "Slot live"} {
		if strings.Contains(body, gone) {
			t.Errorf("dashboard still shows live remnant %q", gone)
		}
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

func TestEnsureLocalModels(t *testing.T) {
	s := newTestServer(t)
	vn := &fakeVieNeuCtl{state: "stopped"}
	s.VieNeu = vn

	s.EnsureLocalModels(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && vn.calls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if vn.calls.Load() == 0 {
		t.Error("vieneu ensure not kicked at startup (sidecar stopped)")
	}

	// Already-ready runtimes must not be kicked.
	s2 := newTestServer(t)
	vn2 := &fakeVieNeuCtl{state: "running"}
	s2.VieNeu = vn2
	s2.EnsureLocalModels(context.Background())
	time.Sleep(150 * time.Millisecond)
	if vn2.calls.Load() != 0 {
		t.Errorf("ready runtime kicked: vieneu=%d, want 0", vn2.calls.Load())
	}
}

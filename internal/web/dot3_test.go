package web

// Đợt 3 — automation switches default ON when unset, a stored OFF always wins;
// DRY-RUN/kill stay the workflow gates,
// the homepage reads current Accesstrade totals only after a successful sync.

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

type fakeJob struct{ status, output string }

type fakeProducer struct {
	jobs     map[string]*fakeJob
	seq      int
	enqueued []automation.ProduceRequest
	failErr  error
}

func newFakeProducer() *fakeProducer { return &fakeProducer{jobs: map[string]*fakeJob{}} }

func (f *fakeProducer) Enqueue(_ context.Context, req automation.ProduceRequest) (string, error) {
	if f.failErr != nil {
		return "", f.failErr
	}
	f.seq++
	id := fmt.Sprintf("job-%d", f.seq)
	f.jobs[id] = &fakeJob{status: "done", output: "/tmp/" + id + ".mp4"}
	f.enqueued = append(f.enqueued, req)
	return id, nil
}

func (f *fakeProducer) JobState(id string) (string, string) {
	j, ok := f.jobs[id]
	if !ok {
		return "", ""
	}
	return j.status, j.output
}

func newGrowthServer(t *testing.T) (*Server, *fakeProducer, any) {
	t.Helper()
	s := newTestServer(t)
	fp := newFakeProducer()
	s.GrowthProducer = fp
	s.Cfg.SetDryRun(false)
	return s, fp, nil
}

func growthAccount(t *testing.T, s *Server, username string) *network.Account {
	t.Helper()
	a, err := s.Mgr.Add(username, "", "")
	if err != nil {
		t.Fatalf("Mgr.Add(%s): %v", username, err)
	}
	if _, err := s.Growth.EnsureProfile(a.ID); err != nil {
		t.Fatalf("EnsureProfile: %v", err)
	}
	return a
}

func seedPlan(t *testing.T, s *Server, accountID int64, drafts ...growth.PlanDraft) {
	t.Helper()
	if _, err := s.Growth.InsertPlan(accountID, "d30", "test", drafts); err != nil {
		t.Fatalf("InsertPlan: %v", err)
	}
}

func draftOn(date, variant, topic string) growth.PlanDraft {
	return growth.PlanDraft{
		Date: date, FormatID: growth.Slugify(strings.Split(topic, "—")[0]),
		Variant: variant, Topic: topic, Hook: "hook",
		Group: date + "|" + growth.Slugify(strings.Split(topic, "—")[0]),
	}
}

func itemByVariant(t *testing.T, s *Server, accountID int64, variant string) growth.PlanItem {
	t.Helper()
	_, items, err := s.Growth.ActivePlan(accountID)
	if err != nil {
		t.Fatalf("ActivePlan: %v", err)
	}
	for _, it := range items {
		if it.Variant == variant {
			return it
		}
	}
	t.Fatalf("item variant %s not found in plan", variant)
	return growth.PlanItem{}
}

func enableProduction(t *testing.T, s *Server) {
	t.Helper()
	if err := s.Ledger.SetSetting(SettingGrowthProduction, "1"); err != nil {
		t.Fatal(err)
	}
}

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

	enabled, _, _ := s.scheduleView()
	if !enabled {
		t.Fatalf("unset switch: enabled=%v, want ON", enabled)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	enabled, _, _ = s.scheduleView()
	if enabled {
		t.Fatalf("stored 0: enabled=%v, want OFF", enabled)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if enabled, _, _ = s.scheduleView(); !enabled {
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

func TestDashboardShelfCount(t *testing.T) {
	s := newTestServer(t)
	pstore, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("open products store: %v", err)
	}
	t.Cleanup(func() { _ = pstore.Close() })
	s.Products = pstore

	rec := get(t, s, "/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Sản phẩm Kệ hàng (1-Link)", "Quản lý kệ hàng ↗"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing shelf stat %q", want)
		}
	}
	for _, gone := range []string{"Phiên live", "Lịch live", "Slot live", "Hoa hồng Accesstrade"} {
		if strings.Contains(body, gone) {
			t.Errorf("dashboard still shows remnant %q", gone)
		}
	}
}

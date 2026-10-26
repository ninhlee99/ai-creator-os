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

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
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

	enabled, _, _, autoPub := s.scheduleView()
	if !enabled || !autoPub {
		t.Fatalf("unset switches: enabled=%v autoPublish=%v, want all ON", enabled, autoPub)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotAutoPublish, "0"); err != nil {
		t.Fatal(err)
	}
	enabled, _, _, autoPub = s.scheduleView()
	if enabled || autoPub {
		t.Fatalf("stored 0: enabled=%v autoPublish=%v, want both OFF", enabled, autoPub)
	}
	if err := s.Ledger.SetSetting(SettingAutopilotEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if enabled, _, _, _ = s.scheduleView(); !enabled {
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

func TestDashboardUsesSyncedAccesstradeStats(t *testing.T) {
	s := newTestServer(t)
	atStore, err := accesstrade.NewStore(filepath.Join(t.TempDir(), "accesstrade.db"))
	if err != nil {
		t.Fatalf("open Accesstrade store: %v", err)
	}
	t.Cleanup(func() { _ = atStore.Close() })
	s.AT = atStore
	if err := s.Ledger.SetSetting(accesstrade.KeySetting, "test-access-key"); err != nil {
		t.Fatalf("set Accesstrade test key: %v", err)
	}
	if _, err := s.Ledger.RecordCommission("2026-10", 999000, "legacy-source"); err != nil {
		t.Fatalf("record legacy commission: %v", err)
	}
	beforeSync := get(t, s, "/")
	if beforeSync.Code != http.StatusOK {
		t.Fatalf("GET / before order sync = %d, want 200", beforeSync.Code)
	}
	if body := beforeSync.Body.String(); !strings.Contains(body, "Chưa có dữ liệu đơn") || strings.Contains(body, "999.000 ₫") {
		t.Fatalf("dashboard treated legacy commission as current Accesstrade stats: %s", body)
	}
	productsBeforeSync := get(t, s, "/products")
	if !strings.Contains(productsBeforeSync.Body.String(), "Chưa đồng bộ") {
		t.Fatal("products page treated unsynced Accesstrade orders as known zero")
	}

	_, _, err = s.AT.UpsertOrders([]accesstrade.Order{
		{OrderID: "approved-1", Status: accesstrade.OrderApproved, PubCommission: 150000},
		{OrderID: "pending-1", Status: accesstrade.OrderPending, PubCommission: 25000},
	})
	if err != nil {
		t.Fatalf("save Accesstrade orders: %v", err)
	}
	if err := s.AT.SetSyncState("orders.until", "2026-10-23T05:00:00Z"); err != nil {
		t.Fatalf("save sync marker: %v", err)
	}
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Hoa hồng Accesstrade đã duyệt", "150.000 ₫", "1 đơn đã duyệt · 1 chờ duyệt", "23/10/2026 05:00 UTC"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing synced Accesstrade stat %q", want)
		}
	}
	if strings.Contains(body, "999.000 ₫") {
		t.Error("dashboard rendered legacy commission as current Accesstrade revenue")
	}
	productsAfterSync := get(t, s, "/products")
	for _, want := range []string{"Đã duyệt: 1 đơn · 150.000 ₫", "Chờ duyệt: 1 đơn · 25.000 ₫", "Cập nhật 23/10/2026 05:00 UTC"} {
		if !strings.Contains(productsAfterSync.Body.String(), want) {
			t.Errorf("products page missing synced order summary %q", want)
		}
	}
	for _, gone := range []string{"Phiên live", "Lịch live", "Slot live"} {
		if strings.Contains(body, gone) {
			t.Errorf("dashboard still shows live remnant %q", gone)
		}
	}
}

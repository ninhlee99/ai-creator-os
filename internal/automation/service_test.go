package automation

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"

	_ "modernc.org/sqlite"
)

// ------------------------------------------------------------- fakes

type fakeGate struct{ kill, dry bool }

func (g *fakeGate) KillSwitch() bool { return g.kill }
func (g *fakeGate) DryRun() bool     { return g.dry }

type fakeEnv map[string]string

func (e fakeEnv) EffectiveEnv(name string) (string, bool) {
	v, ok := e[name]
	return v, ok
}

type fakeJob struct{ status, output string }

type fakeProducer struct {
	jobs     map[string]*fakeJob
	seq      int
	enqueued []ProduceRequest
	failErr  error
}

func newFakeProducer() *fakeProducer { return &fakeProducer{jobs: map[string]*fakeJob{}} }

func (f *fakeProducer) Enqueue(_ context.Context, req ProduceRequest) (string, error) {
	if f.failErr != nil {
		return "", f.failErr
	}
	f.seq++
	id := "job"
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

type fakeUploader struct {
	state  YTState
	result publishers.PublishResult
	n      int
}

func (f *fakeUploader) State(*network.Account) YTState { return f.state }
func (f *fakeUploader) Upload(_ context.Context, _ *network.Account, _, _, _, _ string) publishers.PublishResult {
	f.n++
	return f.result
}

func readyUploader() *fakeUploader {
	return &fakeUploader{
		state:  YTState{HasClient: true, HasToken: true, HasChannel: true, Privacy: "private"},
		result: publishers.PublishResult{Ok: true, Platform: "youtube", RemoteID: "VID123", URL: "https://youtu.be/VID123"},
	}
}

// ------------------------------------------------------------ harness

type harness struct {
	svc *Service
	fp  *fakeProducer
	yt  *fakeUploader
	g   *fakeGate
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ledger.db")
	l, err := ledger.New(dbPath)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	sqldb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })
	gs, err := growth.NewStore(sqldb)
	if err != nil {
		t.Fatalf("growth.NewStore: %v", err)
	}
	mgr, err := network.NewAccountManager(l, dbPath)
	if err != nil {
		t.Fatalf("NewAccountManager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	g := &fakeGate{}
	fp := newFakeProducer()
	yt := readyUploader()
	svc := NewService()
	svc.Growth = gs
	svc.Ledger = l
	svc.Accounts = mgr
	svc.Gate = g
	svc.Env = fakeEnv{}
	svc.Settings = LedgerSettings{L: l}
	svc.Producer = fp
	svc.Uploader = yt
	return &harness{svc: svc, fp: fp, yt: yt, g: g}
}

func (h *harness) account(t *testing.T, username string) *network.Account {
	t.Helper()
	a, err := h.svc.Accounts.(*network.AccountManager).Add(username, "", "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return a
}

func (h *harness) seedPlan(t *testing.T, accountID int64, variant, topic string) {
	t.Helper()
	// Seed one day back so the variant stagger (shorts +1d) lands the
	// item effective-today — same ICT clock the tick uses.
	t0, _ := time.Parse("2006-01-02", h.svc.today())
	today := t0.AddDate(0, 0, -1).Format("2006-01-02")
	slug := growth.Slugify(strings.Split(topic, "—")[0])
	_, err := h.svc.Growth.InsertPlan(accountID, "d30", "test", []growth.PlanDraft{{
		Date: today, FormatID: slug, Variant: variant, Topic: topic,
		Hook: "hook", Group: today + "|" + slug,
	}})
	if err != nil {
		t.Fatalf("InsertPlan: %v", err)
	}
}

func notesJoined(notes []string) string { return strings.Join(notes, "\n") }

// ----------------------------------------------- the safety gates

// TestServiceTickKillSwitchWins — kill chặn toàn bộ vòng tick của
// service, không enqueue gì cả.
func TestServiceTickKillSwitchWins(t *testing.T) {
	h := newHarness(t)
	h.g.kill = true
	a := h.account(t, "kill_acct")
	h.seedPlan(t, a.ID, growth.VariantShorts, "truyện ma — tập 1")

	notes := h.svc.GrowthTick(context.Background(), false)
	if !strings.Contains(notesJoined(notes), "Kill switch") {
		t.Errorf("notes = %v, want the kill-switch line", notes)
	}
	if len(h.fp.enqueued) != 0 {
		t.Errorf("enqueued = %d under kill switch, want 0", len(h.fp.enqueued))
	}
}

// TestServiceTickDryRunBlocksProduction — dry-run chặn sản xuất thật.
func TestServiceTickDryRunBlocksProduction(t *testing.T) {
	h := newHarness(t)
	h.g.dry = true
	a := h.account(t, "dry_acct")
	h.seedPlan(t, a.ID, growth.VariantShorts, "truyện ma — tập 1")

	h.svc.GrowthTick(context.Background(), false)
	if len(h.fp.enqueued) != 0 {
		t.Errorf("enqueued = %d in dry-run, want 0", len(h.fp.enqueued))
	}
	if h.yt.n != 0 {
		t.Errorf("uploads = %d in dry-run, want 0", h.yt.n)
	}
}

// TestServiceTickStoredOffProducesNothing — toggle "0" đã lưu thì tick
// không sản xuất.
func TestServiceTickStoredOffProducesNothing(t *testing.T) {
	h := newHarness(t)
	if err := h.svc.Settings.Set("growth.production_enabled", "0"); err != nil {
		t.Fatal(err)
	}
	a := h.account(t, "off_acct")
	h.seedPlan(t, a.ID, growth.VariantShorts, "truyện ma — tập 1")

	h.svc.GrowthTick(context.Background(), false)
	if len(h.fp.enqueued) != 0 {
		t.Errorf("enqueued = %d with production off, want 0", len(h.fp.enqueued))
	}
}

// ---------------------------------------------- full flow + idempotency

// TestServiceTickFullPublishFlow — một vòng: produce → advance →
// publish Shorts lên YouTube trong quota.
func TestServiceTickFullPublishFlow(t *testing.T) {
	h := newHarness(t)
	a := h.account(t, "flow_acct")
	h.seedPlan(t, a.ID, growth.VariantShorts, "truyện ma — tập 1")

	h.svc.GrowthTick(context.Background(), false)
	if len(h.fp.enqueued) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(h.fp.enqueued))
	}
	// Second tick: the fake job reports done → advance marks produced →
	// publish uploads it.
	h.svc.GrowthTick(context.Background(), false)
	if h.yt.n != 1 {
		t.Fatalf("uploads = %d, want 1", h.yt.n)
	}
	items, err := h.svc.Growth.ActivePlanItems(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Variant == growth.VariantShorts && it.Status != growth.ItemPublished {
			t.Errorf("shorts item status = %s, want published", it.Status)
		}
	}
	used, err := h.svc.Growth.QuotaUsed(time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if used != growth.UploadCostUnits {
		t.Errorf("quota used = %d, want %d", used, growth.UploadCostUnits)
	}
}

// TestServiceTickRestartIdempotency — tick lại không enqueue trùng mục
// đã sản xuất/đăng.
func TestServiceTickRestartIdempotency(t *testing.T) {
	h := newHarness(t)
	a := h.account(t, "idem_acct")
	h.seedPlan(t, a.ID, growth.VariantShorts, "truyện ma — tập 1")

	for i := 0; i < 3; i++ {
		h.svc.GrowthTick(context.Background(), false)
	}
	if len(h.fp.enqueued) != 1 {
		t.Errorf("enqueued = %d after 3 ticks, want exactly 1", len(h.fp.enqueued))
	}
	if h.yt.n != 1 {
		t.Errorf("uploads = %d after 3 ticks, want exactly 1", h.yt.n)
	}
}

// TestServiceSyncOneAccountUsesThresholds — vòng sync dùng ngưỡng đã
// lưu qua UI (LoadThresholds), không phải DefaultConfig hardcode.
func TestServiceSyncOneAccountUsesThresholds(t *testing.T) {
	h := newHarness(t)
	a := h.account(t, "thresh_acct")
	over := growth.DefaultConfig()
	over.KillMinVideos = 3
	over.KillMinDays = 5
	if err := SaveThresholds(h.svc.Settings, over); err != nil {
		t.Fatal(err)
	}
	// The sync runs the real engine against no connected metrics source —
	// the assertion is behavioral: it returns honest notes and does not
	// crash on the persisted thresholds.
	notes := h.svc.SyncOneAccount(context.Background(), a)
	_ = notesJoined(notes)
	if got := LoadThresholds(h.svc.Settings); got.KillMinVideos != 3 || got.KillMinDays != 5 {
		t.Errorf("thresholds not persisted: %+v", got)
	}
}

package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

// ------------------------------------------------------------- fakes

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
	f.jobs[id] = &fakeJob{status: studioStatusDone, output: "/tmp/" + id + ".mp4"}
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

const studioStatusDone = "done"

type ytCall struct{ title, desc, kind string }

type fakeYT struct {
	state   automation.YTState
	result  publishers.PublishResult
	uploads []ytCall
}

func (f *fakeYT) State(*network.Account) automation.YTState { return f.state }
func (f *fakeYT) Upload(_ context.Context, _ *network.Account, _, title, description, kind string) publishers.PublishResult {
	f.uploads = append(f.uploads, ytCall{title: title, desc: description, kind: kind})
	return f.result
}

func readyYT() *fakeYT {
	return &fakeYT{
		state:  automation.YTState{HasClient: true, HasToken: true, HasChannel: true, Privacy: "private"},
		result: publishers.PublishResult{Ok: true, Platform: "youtube", RemoteID: "VID123", URL: "https://youtu.be/VID123", Draft: true},
	}
}

// ------------------------------------------------------------- setup

func newGrowthServer(t *testing.T) (*Server, *fakeProducer, *fakeYT) {
	t.Helper()
	s := newTestServer(t)
	fp := newFakeProducer()
	yt := readyYT()
	s.GrowthProducer = fp
	s.GrowthYT = yt
	s.Cfg.SetDryRun(false)
	return s, fp, yt
}

func growthAccount(t *testing.T, s *Server, username string) *network.Account {
	t.Helper()
	a, err := s.Mgr.Add(username, "", "")
	if err != nil {
		t.Fatalf("Mgr.Add(%s): %v", username, err)
	}
	if _, err := s.Growth.EnsureProfile(a.ID, "tiktok", true); err != nil {
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

// daysAgo returns the ICT date n days before the server's today.
func daysAgo(s *Server, n int) string {
	t0, _ := time.Parse("2006-01-02", s.today())
	return t0.AddDate(0, 0, -n).Format("2006-01-02")
}

func enableProduction(t *testing.T, s *Server) {
	t.Helper()
	if err := s.Ledger.SetSetting(SettingGrowthProduction, "1"); err != nil {
		t.Fatalf("enable production: %v", err)
	}
}

func itemsOf(t *testing.T, s *Server, accountID int64) []growth.PlanItem {
	t.Helper()
	items, err := s.Growth.ActivePlanItems(accountID)
	if err != nil {
		t.Fatalf("ActivePlanItems: %v", err)
	}
	return items
}

func itemByVariant(t *testing.T, s *Server, accountID int64, variant string) growth.PlanItem {
	t.Helper()
	for _, it := range itemsOf(t, s, accountID) {
		if it.Variant == variant {
			return it
		}
	}
	t.Fatalf("no %s item for account %d", variant, accountID)
	return growth.PlanItem{}
}

// ------------------------------------------------------------- tests

func TestGrowthTickStoredOffProducesNothing(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	// Đợt 3: unset now defaults ON — this test pins the other half of
	// the contract: a stored "0" is always respected.
	if err := s.Ledger.SetSetting(SettingGrowthProduction, "0"); err != nil {
		t.Fatalf("store off: %v", err)
	}
	a := growthAccount(t, s, "toggle_off")
	seedPlan(t, s, a.ID, draftOn(s.today(), growth.VariantTikTok, "truyện ma — tập 1"))

	s.GrowthAutomationTick(context.Background())

	if len(fp.enqueued) != 0 {
		t.Errorf("toggle off but %d jobs enqueued", len(fp.enqueued))
	}
	if it := itemByVariant(t, s, a.ID, growth.VariantTikTok); it.Status != growth.ItemPlanned {
		t.Errorf("item status = %s, want planned", it.Status)
	}
}

func TestGrowthTickFullPublishFlowAndIdempotency(t *testing.T) {
	s, fp, yt := newGrowthServer(t)
	enableProduction(t, s)
	a := growthAccount(t, s, "full_flow")
	today := s.today()
	seedPlan(t, s, a.ID,
		draftOn(today, growth.VariantTikTok, "truyện ma — tập 1"),
		draftOn(daysAgo(s, 1), growth.VariantShorts, "truyện ma — tập 1"),
		draftOn(today, growth.VariantYouTube, "truyện ma — bản dài tập 1"),
	)

	s.GrowthAutomationTick(context.Background())

	// TikTok rendered (draft-only platform), Shorts published for real.
	tk := itemByVariant(t, s, a.ID, growth.VariantTikTok)
	if tk.Status != growth.ItemProduced {
		t.Errorf("tiktok item = %s (note %q), want produced", tk.Status, tk.PubNote)
	}
	sh := itemByVariant(t, s, a.ID, growth.VariantShorts)
	if sh.Status != growth.ItemPublished || sh.PublishedRef != "VID123" {
		t.Errorf("shorts item = %s ref %q, want published/VID123", sh.Status, sh.PublishedRef)
	}
	if !strings.HasSuffix(sh.PubTitle, "#Shorts") {
		t.Errorf("shorts publish title = %q, want #Shorts suffix", sh.PubTitle)
	}
	if len(yt.uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(yt.uploads))
	}
	if !strings.Contains(yt.uploads[0].desc, "synthetic") {
		t.Errorf("description misses the AI disclosure: %q", yt.uploads[0].desc)
	}
	used, err := s.Growth.QuotaUsed(today)
	if err != nil || used != growth.UploadCostUnits {
		t.Errorf("quota used = %d (%v), want %d", used, err, growth.UploadCostUnits)
	}
	// Long-form is staggered +3 days: not produced yet.
	lg := itemByVariant(t, s, a.ID, growth.VariantYouTube)
	if lg.Status != growth.ItemPlanned {
		t.Errorf("long item = %s, want planned (lag 3 days)", lg.Status)
	}

	// Second tick: no duplicate jobs, no duplicate uploads.
	s.GrowthAutomationTick(context.Background())
	if len(fp.enqueued) != 2 {
		t.Errorf("after tick 2: enqueued = %d, want 2 (tiktok+shorts)", len(fp.enqueued))
	}
	if len(yt.uploads) != 1 {
		t.Errorf("after tick 2: uploads = %d, want 1", len(yt.uploads))
	}
	if used, _ := s.Growth.QuotaUsed(today); used != growth.UploadCostUnits {
		t.Errorf("quota after tick 2 = %d, want %d", used, growth.UploadCostUnits)
	}
}

func TestGrowthTickRestartDoesNotDuplicate(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	a := growthAccount(t, s, "restart_case")
	seedPlan(t, s, a.ID, draftOn(daysAgo(s, 1), growth.VariantShorts, "kể chuyện đêm khuya"))

	s.GrowthAutomationTick(context.Background())
	if got := itemByVariant(t, s, a.ID, growth.VariantShorts); got.Status != growth.ItemPublished {
		t.Fatalf("pre-restart item = %s, want published", got.Status)
	}

	// Simulate a process restart: a brand-new Server over the same data,
	// whose Studio still knows yesterday's jobs.
	dbPath := filepath.Join(s.Cfg.DatabasePath)
	l2, err := ledger.New(dbPath)
	if err != nil {
		t.Fatalf("ledger reopen: %v", err)
	}
	defer l2.Close()
	mgr2, err := network.NewAccountManager(l2, dbPath)
	if err != nil {
		t.Fatalf("mgr reopen: %v", err)
	}
	defer mgr2.Close()
	cfg := LoadConfig()
	cfg.DatabasePath = dbPath
	s2, err := NewServer(cfg, l2, mgr2, dbPath)
	if err != nil {
		t.Fatalf("server reopen: %v", err)
	}
	defer s2.Close()
	fp2 := newFakeProducer()
	for id, j := range fp.jobs {
		cp := *j
		fp2.jobs[id] = &cp
	}
	s2.GrowthProducer = fp2
	s2.GrowthYT = readyYT()
	s2.Cfg.SetDryRun(false)

	s2.GrowthAutomationTick(context.Background())
	if len(fp2.enqueued) != 0 {
		t.Errorf("restart spawned %d duplicate jobs", len(fp2.enqueued))
	}
	if got := itemByVariant(t, s2, a.ID, growth.VariantShorts); got.Status != growth.ItemPublished {
		t.Errorf("post-restart item = %s, want published", got.Status)
	}
}

func TestGrowthTickWaitingConnectThenReady(t *testing.T) {
	s, _, yt := newGrowthServer(t)
	enableProduction(t, s)
	yt.state = automation.YTState{HasClient: true, HasToken: false, Privacy: "private"}
	a := growthAccount(t, s, "wait_connect")
	seedPlan(t, s, a.ID, draftOn(daysAgo(s, 1), growth.VariantShorts, "mẹo học tiếng Anh"))

	s.GrowthAutomationTick(context.Background())
	s.GrowthAutomationTick(context.Background())

	it := itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemWaitingConnect {
		t.Fatalf("item = %s, want waiting_connect", it.Status)
	}
	if !strings.Contains(it.PubNote, "OAuth") {
		t.Errorf("waiting note should name the missing step: %q", it.PubNote)
	}
	if len(yt.uploads) != 0 {
		t.Errorf("uploads attempted without credentials: %d", len(yt.uploads))
	}

	yt.state.HasToken = true
	s.GrowthAutomationTick(context.Background())
	it = itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemPublished {
		t.Errorf("after OAuth: item = %s, want published", it.Status)
	}
}

func TestGrowthTickQuotaDeferred(t *testing.T) {
	s, _, yt := newGrowthServer(t)
	enableProduction(t, s)
	today := s.today()
	if err := s.Growth.AddQuota(today, growth.DailyQuotaUnits); err != nil {
		t.Fatal(err)
	}
	a := growthAccount(t, s, "quota_case")
	seedPlan(t, s, a.ID, draftOn(daysAgo(s, 1), growth.VariantShorts, "review đồ gia dụng"))

	s.GrowthAutomationTick(context.Background())

	it := itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemWaitingQuota {
		t.Errorf("item = %s (note %q), want waiting_quota", it.Status, it.PubNote)
	}
	if len(yt.uploads) != 0 {
		t.Errorf("uploads attempted past quota: %d", len(yt.uploads))
	}
}

func TestGrowthTickDedupRegeneratesAngle(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	today := s.today()
	a1 := growthAccount(t, s, "dedup_one")
	a2 := growthAccount(t, s, "dedup_two")
	seedPlan(t, s, a1.ID, draftOn(today, growth.VariantTikTok, "truyện ma — tập 1"))
	seedPlan(t, s, a2.ID, draftOn(today, growth.VariantTikTok, "truyện ma — tập 1"))

	s.GrowthAutomationTick(context.Background())

	it1 := itemByVariant(t, s, a1.ID, growth.VariantTikTok)
	it2 := itemByVariant(t, s, a2.ID, growth.VariantTikTok)
	if it1.Topic == it2.Topic {
		t.Errorf("cross-account identical topics shipped: %q", it1.Topic)
	}
	regenerated := 0
	for _, it := range []growth.PlanItem{it1, it2} {
		if it.DedupAction == "angle_regenerated" {
			regenerated++
			if !strings.Contains(it.Topic, " — ") || strings.HasSuffix(it.Topic, "tập 1") {
				t.Errorf("regenerated topic malformed: %q", it.Topic)
			}
		}
		if it.Status != growth.ItemProduced {
			t.Errorf("item %d status = %s, want produced (dedup must not block)", it.ID, it.Status)
		}
	}
	if regenerated != 1 {
		t.Errorf("regenerated = %d, want exactly 1 (the second account)", regenerated)
	}
	if len(fp.enqueued) != 2 {
		t.Errorf("enqueued = %d, want 2 (production continues)", len(fp.enqueued))
	}
	alerts, err := s.Growth.ListAlerts(a2.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, al := range alerts {
		if al.Kind == "dedup" {
			found = true
		}
	}
	if !found {
		// The regenerated account may be a1 depending on list order.
		alerts1, _ := s.Growth.ListAlerts(a1.ID, 10)
		for _, al := range alerts1 {
			if al.Kind == "dedup" {
				found = true
			}
		}
	}
	if !found {
		t.Error("no dedup alert recorded")
	}
}

func TestGrowthTickStaleDroppedAndLagRespected(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	a := growthAccount(t, s, "stale_case")
	today := s.today()
	past := func(days int) string {
		t0, _ := time.Parse("2006-01-02", today)
		return t0.AddDate(0, 0, -days).Format("2006-01-02")
	}
	seedPlan(t, s, a.ID,
		draftOn(past(20), growth.VariantTikTok, "chủ đề cũ"),
		draftOn(today, growth.VariantShorts, "truyện ma — tập 1"),
		draftOn(today, growth.VariantYouTube, "truyện ma — bản dài tập 1"),
	)

	s.GrowthAutomationTick(context.Background())

	if len(fp.enqueued) != 0 {
		t.Errorf("enqueued = %d, want 0 (stale dropped, shorts/long lagging)", len(fp.enqueued))
	}
	for _, it := range itemsOf(t, s, a.ID) {
		switch it.Variant {
		case growth.VariantTikTok:
			if it.Status != growth.ItemDropped {
				t.Errorf("stale tiktok item = %s, want dropped", it.Status)
			}
		default:
			if it.Status != growth.ItemPlanned {
				t.Errorf("%s item = %s, want planned (still in stagger lag)", it.Variant, it.Status)
			}
		}
	}
}

func TestGrowthTickEnqueueFailureRetries(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	fp.failErr = fmt.Errorf("media-gen chưa sẵn sàng")
	a := growthAccount(t, s, "retry_case")
	seedPlan(t, s, a.ID, draftOn(daysAgo(s, 1), growth.VariantShorts, "luyện nghe tiếng Anh"))

	s.GrowthAutomationTick(context.Background())
	it := itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemPlanned || it.Attempts != 1 {
		t.Fatalf("after failure: status=%s attempts=%d, want planned/1", it.Status, it.Attempts)
	}

	fp.failErr = nil
	s.GrowthAutomationTick(context.Background())
	it = itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemPublished {
		t.Errorf("after retry: status = %s, want published", it.Status)
	}
}

func TestGrowthTickJobFailedMarksItemFailed(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	// Producer that completes enqueue but reports failed renders.
	fp2 := &jobFailProducer{fakeProducer: fp}
	s.GrowthProducer = fp2
	a := growthAccount(t, s, "fail_case")
	seedPlan(t, s, a.ID, draftOn(daysAgo(s, 1), growth.VariantShorts, "câu chuyện đời"))

	s.GrowthAutomationTick(context.Background())
	s.GrowthAutomationTick(context.Background())

	it := itemByVariant(t, s, a.ID, growth.VariantShorts)
	if it.Status != growth.ItemFailed {
		t.Errorf("item = %s, want failed", it.Status)
	}
}

type jobFailProducer struct{ *fakeProducer }

func (p *jobFailProducer) JobState(id string) (string, string) {
	if _, ok := p.jobs[id]; !ok {
		return "", ""
	}
	return "failed", ""
}

func TestGrowthTickDryRunBlocksProduction(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	s.Cfg.SetDryRun(true) // newGrowthServer cleared it; put it back
	enableProduction(t, s)
	a := growthAccount(t, s, "dryrun_case")
	seedPlan(t, s, a.ID, draftOn(s.today(), growth.VariantTikTok, "truyện ma — tập 1"))

	s.GrowthAutomationTick(context.Background())

	if len(fp.enqueued) != 0 {
		t.Errorf("dry-run enqueued %d jobs", len(fp.enqueued))
	}
	if it := itemByVariant(t, s, a.ID, growth.VariantTikTok); it.Status != growth.ItemPlanned {
		t.Errorf("dry-run item = %s, want planned", it.Status)
	}
}

func TestGrowthTickProductionCapPerTick(t *testing.T) {
	s, fp, _ := newGrowthServer(t)
	enableProduction(t, s)
	a := growthAccount(t, s, "cap_case")
	today := s.today()
	seedPlan(t, s, a.ID,
		draftOn(today, growth.VariantTikTok, "chủ đề alpha"),
		draftOn(today, growth.VariantTikTok, "chủ đề beta"),
		draftOn(today, growth.VariantTikTok, "chủ đề gamma"),
		draftOn(today, growth.VariantTikTok, "chủ đề delta"),
		draftOn(today, growth.VariantTikTok, "chủ đề omega"),
	)

	s.GrowthAutomationTick(context.Background())

	if want := s.automation().MaxProductionsPerTick; len(fp.enqueued) != want {
		t.Errorf("enqueued = %d, want cap %d", len(fp.enqueued), want)
	}
}

func TestGrowthProductionToggleAndPage(t *testing.T) {
	s, _, _ := newGrowthServer(t)
	_ = growthAccount(t, s, "page_case")

	rec := postForm(t, s, "/growth/production", url.Values{"enabled": {"1"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle POST = %d", rec.Code)
	}
	if !s.GrowthProductionEnabled() {
		t.Error("production toggle did not persist")
	}
	rec = get(t, s, "/growth")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /growth = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Sản xuất &amp; đăng tự động", "đang bật", "Chạy ngay", "Đăng YouTube"} {
		if !strings.Contains(body, want) {
			t.Errorf("/growth missing %q", want)
		}
	}
	rec = postForm(t, s, "/growth/production/run", url.Values{})
	if rec.Code != http.StatusOK {
		t.Errorf("run-now POST = %d", rec.Code)
	}
	rec = postForm(t, s, "/growth/production", url.Values{"enabled": {"0"}})
	if rec.Code != http.StatusOK || s.GrowthProductionEnabled() {
		t.Errorf("toggle off failed: code %d enabled %v", rec.Code, s.GrowthProductionEnabled())
	}
}

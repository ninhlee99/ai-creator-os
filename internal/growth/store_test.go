package growth

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "growth.db"))
	if err != nil {
		t.Fatal(err)
	}
	// The decisions audit table belongs to the ledger schema; create it
	// here so Decide() works in package-local tests too.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS decisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT, agent TEXT, action TEXT,
		target TEXT, reason TEXT, inputs_json TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	st, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return st
}

func TestEnsureProfileCreatesLadder(t *testing.T) {
	st := testStore(t)
	p, err := st.EnsureProfile(7, "tiktok", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stage != StageColdStart {
		t.Errorf("new profile stage = %s, want cold_start", p.Stage)
	}
	targets, err := st.ListTargets(7)
	if err != nil {
		t.Fatal(err)
	}
	// 3 TikTok follower milestones + 5 YouTube ladder rows.
	if len(targets) != 8 {
		t.Errorf("targets = %d, want 8 (3 tiktok + 5 youtube)", len(targets))
	}
	// Idempotent: ensuring again must not duplicate.
	if _, err := st.EnsureProfile(7, "tiktok", true); err != nil {
		t.Fatal(err)
	}
	targets2, _ := st.ListTargets(7)
	if len(targets2) != len(targets) {
		t.Errorf("EnsureProfile not idempotent: %d -> %d targets", len(targets), len(targets2))
	}
}

func TestPlanInsertSupersede(t *testing.T) {
	st := testStore(t)
	drafts := GeneratePlan(PlanRequest{
		Stage: StageColdStart, Pillars: []string{"truyện ma"},
		Start: time.Now(), Days: 10,
	})
	id1, err := st.InsertPlan(3, "d30", "khởi tạo", drafts)
	if err != nil {
		t.Fatal(err)
	}
	plan, items, err := st.ActivePlan(3)
	if err != nil || plan == nil {
		t.Fatalf("ActivePlan: %v %v", plan, err)
	}
	if plan.ID != id1 || len(items) != len(drafts) {
		t.Errorf("plan %d items %d, want plan %d items %d", plan.ID, len(items), id1, len(drafts))
	}
	// Replan supersedes, never rewrites history.
	id2, err := st.InsertPlan(3, "d30", "replan tuần", drafts)
	if err != nil {
		t.Fatal(err)
	}
	plan2, _, err := st.ActivePlan(3)
	if err != nil || plan2 == nil || plan2.ID != id2 {
		t.Errorf("after replan active = %v, want %d", plan2, id2)
	}
	up, err := st.UpcomingItems(3, "2000-01-01", 3)
	if err != nil || len(up) != 3 {
		t.Errorf("UpcomingItems = %d (%v), want 3", len(up), err)
	}
}

func TestSnapshotRoundTripAndTargets(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(5, "tiktok", false); err != nil {
		t.Fatal(err)
	}
	f := int64(1200)
	v := int64(40)
	if err := st.InsertSnapshot(Snapshot{AccountID: 5, Followers: &f, Videos: &v, Source: "tiktok_api"}); err != nil {
		t.Fatal(err)
	}
	latest, err := st.LatestSnapshot(5)
	if err != nil || latest == nil || latest.Followers == nil || *latest.Followers != 1200 {
		t.Fatalf("LatestSnapshot = %+v, err %v", latest, err)
	}
	reached, err := st.MarkTargetsReached(5, map[string]float64{"followers": 1200})
	if err != nil {
		t.Fatal(err)
	}
	if len(reached) != 2 { // 100 + 1.000 milestones
		t.Errorf("reached = %v, want 2 milestones", reached)
	}
	// Append-only: a second reading keeps the first intact.
	f2 := int64(1300)
	_ = st.InsertSnapshot(Snapshot{AccountID: 5, Followers: &f2, Source: "tiktok_api"})
	series, err := st.SnapshotSeries(5, 10)
	if err != nil || len(series) != 2 {
		t.Fatalf("series = %d, err %v", len(series), err)
	}
	if *series[0].Followers != 1200 || *series[1].Followers != 1300 {
		t.Error("series must be oldest-first with both readings")
	}
}

func TestEvaluateKillsWeakFormat(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(9, "tiktok", false); err != nil {
		t.Fatal(err)
	}
	// Seed 9 published videos of one weak format + 1 of a strong one by
	// writing plan items with published refs and video snapshots.
	drafts := GeneratePlan(PlanRequest{Stage: StageFormatTesting, Pillars: []string{"truyện ma", "kể chuyện đời"}, Start: time.Now(), Days: 20})
	planID, err := st.InsertPlan(9, "d30", "test", drafts)
	if err != nil {
		t.Fatal(err)
	}
	db := st.db
	publish := func(format, vid string, views int, completion float64) {
		if _, err := db.Exec(`UPDATE content_plan_items SET status='published', published_ref=?
			WHERE id=(SELECT id FROM content_plan_items WHERE plan_id=? AND format_id=? AND status='planned' LIMIT 1)`,
			vid, planID, format); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO video_metric_snapshots
			(account_id, platform_video_id, taken_at, views, shares, saves, completion)
			VALUES (?, ?, datetime('now','-20 days'), ?, 1, 0, ?)`,
			9, vid, views, completion); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 9; i++ {
		publish("truyen_ma", fmt.Sprintf("weak%d", i), 100, 0.10)
	}
	publish("ke_chuyen_doi", "strong0", 9000, 0.80)

	res, err := st.Evaluate(9, "acct9", DefaultConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stats, err := st.ListFormatStats(9)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]string{}
	for _, s := range stats {
		verdicts[s.FormatID] = s.Verdict
	}
	if verdicts["truyen_ma"] != VerdictKilled {
		t.Errorf("weak format verdict = %q, want killed (all: %v, actions %v)", verdicts["truyen_ma"], verdicts, res.Actions)
	}
	if verdicts["ke_chuyen_doi"] == VerdictKilled {
		t.Errorf("strong format must not be killed: %v", verdicts)
	}
	alerts, _ := st.ListAlerts(9, 10)
	found := false
	for _, a := range alerts {
		if a.Kind == "format" {
			found = true
		}
	}
	if !found {
		t.Error("kill must produce a notify-only format alert")
	}
}

func TestEvaluateNoDataDecidesNothing(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(11, "youtube", true); err != nil {
		t.Fatal(err)
	}
	res, err := st.Evaluate(11, "acct11", DefaultConfig(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.PauseAccount || len(res.Actions) != 0 {
		t.Errorf("no-data evaluate must decide nothing, got %+v", res)
	}
}

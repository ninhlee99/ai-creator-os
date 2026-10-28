package growth

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestDiagnoseAffiliateVideoFailsClosed(t *testing.T) {
	zero := int64(0)
	positive := int64(5000)
	cases := []AffiliateMetricsInput{
		{},
		{Views: &positive, Clicks: &zero, Orders: &zero},
		{Views: &positive, Clicks: &positive, Orders: &positive},
	}
	for _, in := range cases {
		d := DiagnoseAffiliateVideo(1, "job", in)
		if d.FunnelStage != "unknown" || d.AutoTuned {
			t.Fatalf("diagnosis must be unknown and not auto-tuned: %+v", d)
		}
		if d.Diagnosis == "" || d.SuggestedAction == "" {
			t.Fatalf("unknown diagnosis must explain evidence gap and next step: %+v", d)
		}
	}
}

func TestDiagnoseViralVideoFailsClosed(t *testing.T) {
	zero := 0.0
	low := 0.1
	cases := []ViralMetricsInput{
		{},
		{ThreeSecRetentionRate: &low, CompletionRate: &low},
		{ThreeSecRetentionRate: &zero, CompletionRate: &zero},
	}
	for _, in := range cases {
		d := DiagnoseViralVideo(2, "video", in)
		if d.FunnelStage != "unknown" || d.AutoTuned {
			t.Fatalf("diagnosis must be unknown and not auto-tuned: %+v", d)
		}
		if d.Diagnosis == "" || d.SuggestedAction == "" {
			t.Fatalf("unknown diagnosis must explain evidence gap and next step: %+v", d)
		}
	}
}

func TestListDiagnosticsDowngradesLegacyClaims(t *testing.T) {
	st := testStore(t)
	_, err := st.db.Exec(`INSERT INTO channel_diagnostics
		(account_id, flow_type, target_id, funnel_stage, diagnosis, metrics_json, suggested_action, auto_tuned, created_at)
		VALUES (1, 'viral', 'v1', 'healthy', 'unsupported old claim', '{"views":0}', 'auto action', 1, '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ListDiagnostics(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	if got[0].FunnelStage != "unknown" || got[0].AutoTuned {
		t.Fatalf("legacy unsupported claim must be downgraded: %+v", got[0])
	}
	if got[0].MetricsJSON != `{"views":0}` {
		t.Fatalf("raw evidence must be preserved, got %s", got[0].MetricsJSON)
	}
}

func TestRecordDiagnosticForcesUnknownUntilEvidenceContractExists(t *testing.T) {
	st := testStore(t)
	_, err := st.RecordDiagnostic(ChannelDiagnostic{
		AccountID: 1, FlowType: "viral", TargetID: "v2", FunnelStage: "healthy",
		Diagnosis: "looks good", SuggestedAction: "scale it", AutoTuned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ListDiagnostics(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FunnelStage != "unknown" || got[0].AutoTuned {
		t.Fatalf("unverified caller claim must be stored as unknown and not auto-tuned: %+v", got)
	}
	if got[0].Diagnosis == "looks good" || got[0].SuggestedAction == "scale it" {
		t.Fatalf("unverified caller text must not be rendered as diagnosis or action: %+v", got[0])
	}
}

func tuningEvidence() TuningEvidence {
	return TuningEvidence{Source: "tiktok_analytics", EvidenceIDs: []string{"v1", "v2", "v3"},
		WindowStart: "2026-09-01T00:00:00Z", WindowEnd: "2026-10-01T00:00:00Z",
		Baseline: 100, Observed: 130, SampleSize: 3, Confidence: 0.9, Explanation: "same-account comparison"}
}

func TestAdjustmentAppliesAccountScopedBoundedTuningAndRollback(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(10); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureProfile(11); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyBoundedAdjustment(10, "growth.kill_min_videos", 8, 100, "weak formats need more samples", TuningEvidence{}); err == nil {
		t.Fatal("adjustment without evidence must fail")
	}
	h, err := st.ApplyBoundedAdjustment(10, "growth.kill_min_videos", 8, 100, "weak formats need more samples", tuningEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if h.NewValue != "9" || !h.Applied || h.ChangePct != 12.5 {
		t.Fatalf("bounded setting change = %+v; want account-scoped 8 -> 9", h)
	}
	var evidence TuningEvidence
	if err := json.Unmarshal([]byte(h.EvidenceJSON), &evidence); err != nil || len(evidence.EvidenceIDs) != 3 {
		t.Fatalf("persisted evidence = %+v, err=%v", evidence, err)
	}
	cfg, err := st.configWithAccountTuning(10, DefaultConfig())
	if err != nil || cfg.KillMinVideos != 9 {
		t.Fatalf("account tuning config = %+v, err=%v", cfg, err)
	}
	other, err := st.configWithAccountTuning(11, DefaultConfig())
	if err != nil || other.KillMinVideos != 8 {
		t.Fatalf("tuning leaked to other account: %+v, err=%v", other, err)
	}
	rolledBack, err := st.RollbackSetting(10, h.ID)
	if err != nil || !rolledBack.RolledBack || rolledBack.RolledBackAt == "" {
		t.Fatalf("rollback = %+v, err=%v", rolledBack, err)
	}
	cfg, err = st.configWithAccountTuning(10, DefaultConfig())
	if err != nil || cfg.KillMinVideos != 8 {
		t.Fatalf("rollback did not restore default: %+v, err=%v", cfg, err)
	}
	if _, err := st.RollbackSetting(10, h.ID); err == nil {
		t.Fatal("second rollback must fail")
	}
	hist, err := st.ListTuningHistory(10, 10)
	if err != nil || len(hist) != 1 || !hist[0].Applied || !hist[0].RolledBack {
		t.Fatalf("tuning audit = %+v, err=%v", hist, err)
	}
}

func TestRollbackRefusesSupersededSetting(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(10); err != nil {
		t.Fatal(err)
	}
	first, err := st.ApplyBoundedAdjustment(10, "growth.kill_min_videos", 8, 12, "first", tuningEvidence())
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.ApplyBoundedAdjustment(10, "growth.kill_min_videos", 9, 12, "second", tuningEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if second.NewValue != "10" {
		t.Fatalf("second bounded value = %s, want 10", second.NewValue)
	}
	if _, err := st.RollbackSetting(10, first.ID); err == nil {
		t.Fatal("stale rollback must fail")
	}
	if _, err := st.RollbackSetting(10, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RollbackSetting(10, first.ID); err != nil {
		t.Fatalf("rollback after newer rollback: %v", err)
	}
}

func TestEvaluateUsesAccountTuning(t *testing.T) {
	st := testStore(t)
	if _, err := st.EnsureProfile(9); err != nil {
		t.Fatal(err)
	}
	drafts := GeneratePlan(PlanRequest{Stage: StageFormatTesting, Pillars: []string{"truyện ma"}, Start: time.Now(), Days: 20})
	planID, err := st.InsertPlan(9, "d30", "tuning test", drafts)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		vid := fmt.Sprintf("tuned%d", i)
		if _, err := st.db.Exec(`UPDATE content_plan_items SET status='published', published_ref=? WHERE id=(SELECT id FROM content_plan_items WHERE plan_id=? AND status='planned' LIMIT 1)`, vid, planID); err != nil {
			t.Fatal(err)
		}
		if _, err := st.db.Exec(`INSERT INTO video_metric_snapshots (account_id,platform_video_id,taken_at,views,shares,saves,completion) VALUES (9,?,datetime('now','-20 days'),100,0,0,0.1)`, vid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.Exec(`UPDATE growth_profiles SET tuning_json=? WHERE account_id=?`, `{"growth.kill_min_videos":3,"growth.kill_min_days":1}`, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Evaluate(9, "acct9", DefaultConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	stats, err := st.ListFormatStats(9)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Verdict != VerdictKilled {
		t.Fatalf("account tuning was not applied in Evaluate: %+v", stats)
	}
}

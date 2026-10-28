package studio

import (
	"path/filepath"
	"strings"
	"testing"
)

// Đợt Y: chân đăng (publish legs) — idempotency cho auto-publish.
func TestPublishLegs(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if got := st.PublishLegStatus("job1", "tiktok"); got != "" {
		t.Fatalf("leg chưa đăng phải là rỗng, được %q", got)
	}
	if err := st.SetPublishLeg("job1", "tiktok", LegDone, "remote-1"); err != nil {
		t.Fatal(err)
	}
	if got := st.PublishLegStatus("job1", "tiktok"); got != LegDone {
		t.Fatalf("leg status = %q, muốn done", got)
	}
	// Upsert: done → failed (lỗi tạm thời sau khi đã đăng không xảy ra
	// thật, nhưng upsert phải hoạt động).
	if err := st.SetPublishLeg("job1", "tiktok", LegFailed, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPublishLeg("job1", "youtube", LegFailed, ""); err != nil {
		t.Fatal(err)
	}
	failed := st.FailedPublishLegs()
	if len(failed) != 2 {
		t.Fatalf("failed legs = %d, muốn 2", len(failed))
	}
	// Platform khác nhau không va chạm.
	if err := st.SetPublishLeg("job1", "youtube", LegDone, "yt-1"); err != nil {
		t.Fatal(err)
	}
	if got := st.PublishLegStatus("job1", "tiktok"); got != LegFailed {
		t.Fatalf("leg tiktok = %q, muốn failed", got)
	}
	if got := st.PublishLegStatus("job1", "youtube"); got != LegDone {
		t.Fatalf("leg youtube = %q, muốn done", got)
	}
}

func TestBeginPublishLegWithConsentCommitsClaimAndEvidenceTogether(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	jobID, err := st.insertJob(KindViral, "viral", ViralParams{})
	if err != nil {
		t.Fatal(err)
	}
	const consent = "operator confirmed preview and TikTok draft"

	claimed, err := st.BeginPublishLegWithConsent(jobID, "tiktok", consent)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, err = %v", claimed, err)
	}
	job, ok := st.GetJob(jobID)
	if !ok || !strings.Contains(job.Log, consent) {
		t.Fatalf("consent evidence missing from job log: %+v", job)
	}
	if got := st.PublishLegStatus(jobID, "tiktok"); got != LegPublishing {
		t.Fatalf("leg status = %q, want publishing", got)
	}
	if claimed, err := st.BeginPublishLegWithConsent(jobID, "tiktok", consent); err != nil || claimed {
		t.Fatalf("duplicate claim = %v, err = %v; want no claim", claimed, err)
	}
	job, _ = st.GetJob(jobID)
	if strings.Count(job.Log, consent) != 1 {
		t.Fatalf("duplicate claim changed consent evidence: %q", job.Log)
	}

	if _, err := st.db.Exec(`CREATE TRIGGER reject_consent_log BEFORE UPDATE OF log ON studio_jobs
		BEGIN SELECT RAISE(ABORT, 'log write failed'); END`); err != nil {
		t.Fatal(err)
	}
	failedJobID, err := st.insertJob(KindViral, "viral without log", ViralParams{})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.BeginPublishLegWithConsent(failedJobID, "tiktok", consent); err == nil || claimed {
		t.Fatalf("claim with failed consent persistence = %v, err = %v; want rollback", claimed, err)
	}
	if got := st.PublishLegStatus(failedJobID, "tiktok"); got != "" {
		t.Fatalf("leg status after consent rollback = %q, want empty", got)
	}
	if claimed, err := st.BeginPublishLegWithConsent(failedJobID, "tiktok", " "); err == nil || claimed {
		t.Fatalf("empty consent claim = %v, err = %v; want rejection", claimed, err)
	}
}

// Đợt Y: job "queued" kẹt sau restart cũng bị fail (goroutine đã chết cùng
// process cũ, không ai re-dispatch) — trước đây chỉ fail job "running".
func TestMarkInterruptedJobsSweepsQueued(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idQ, _ := st.insertJob(KindAffiliate, "Queued", AffiliateParams{ProductName: "x"})
	j, _ := st.GetJob(idQ)
	if j.Status != StatusQueued {
		t.Fatalf("job mới phải ở queued, được %q", j.Status)
	}
	if n := st.MarkInterruptedJobs(); n != 1 {
		t.Fatalf("swept %d, muốn 1", n)
	}
	j, _ = st.GetJob(idQ)
	if j.Status != StatusFailed {
		t.Errorf("queued job status = %q, muốn failed", j.Status)
	}
}

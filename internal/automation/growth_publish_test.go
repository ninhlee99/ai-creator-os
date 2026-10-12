package automation

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// fakeGrowthUploader đếm số lần upload được gọi.
type fakeGrowthUploader struct {
	state   YTState
	uploads int
}

func (f *fakeGrowthUploader) State(a *network.Account) YTState { return f.state }
func (f *fakeGrowthUploader) Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult {
	f.uploads++
	return publishers.PublishResult{Ok: true, RemoteID: "VID999"}
}

// fakeGrowthProducer trả về job done với output.
type fakeGrowthProducer struct{}

func (f *fakeGrowthProducer) Enqueue(ctx context.Context, req ProduceRequest) (string, error) {
	return "job1", nil
}
func (f *fakeGrowthProducer) JobState(jobID string) (string, string) {
	return studio.StatusDone, "/tmp/fake.mp4"
}

// fakeLegJobStore giả lập chân đăng YouTube đã done.
type fakeLegJobStore struct {
	*fakeJobStore
}

func newGrowthPublishService(t *testing.T, legStatus, legRID string) (*Service, *fakeGrowthUploader, *growth.Store, *network.Account) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	gs, err := growth.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	svc.Growth = gs
	svc.Gate = &fakeGate{}
	yt := &fakeGrowthUploader{state: YTState{HasClient: true, HasToken: true, HasChannel: true, Privacy: "private"}}
	svc.Uploader = yt
	svc.Producer = &fakeGrowthProducer{}
	js := &fakeJobStore{jobs: map[string]studio.Job{}, legs: map[string]string{}}
	if legStatus != "" {
		_ = js.SetPublishLeg("job1", "youtube", legStatus, legRID)
	}
	svc.JobStore = js
	acct := &network.Account{ID: 1, Username: "yt1", YoutubeChannel: "UCx"}
	return svc, yt, gs, acct
}

func seedGrowthItem(t *testing.T, gs *growth.Store, accountID int64) growth.PlanItem {
	t.Helper()
	drafts := []growth.PlanDraft{{
		Date: "2026-10-12", FormatID: "ke-chuyen", Variant: growth.VariantShorts,
		Topic: "truyện ma", Hook: "h", SeriesEp: 1, Group: "2026-10-12|ke-chuyen",
	}}
	planID, err := gs.InsertPlan(accountID, "d30", "test", drafts)
	if err != nil {
		t.Fatal(err)
	}
	_ = planID
	items, err := gs.ItemsByStatus(accountID, []string{growth.ItemPlanned}, 10)
	if err != nil || len(items) == 0 {
		t.Fatalf("seed plan: %v (items %d)", err, len(items))
	}
	it := items[0]
	it.StudioJobID = "job1"
	if err := gs.SetItemStatus(it.ID, growth.ItemProduced, "render xong"); err != nil {
		t.Fatal(err)
	}
	// Đọc lại để có StudioJobID — SetItemStatus không đổi job id, gán trực tiếp qua DB.
	// Đơn giản: dùng item đã có, set StudioJobID qua update.
	items2, _ := gs.ItemsByStatus(accountID, []string{growth.ItemProduced}, 10)
	if len(items2) == 0 {
		t.Fatal("không tìm thấy item produced")
	}
	// Gán StudioJobID qua PlanItem (không có setter riêng — dùng trực tiếp).
	it2 := items2[0]
	it2.StudioJobID = "job1"
	return it2
}

// TestPublishItemSkipsWhenYouTubeLegDone: chân YouTube đã done (do autopilot
// đăng trước) thì publishItem không upload lại — đánh dấu item đã đăng.
func TestPublishItemSkipsWhenYouTubeLegDone(t *testing.T) {
	svc, yt, gs, acct := newGrowthPublishService(t, studio.LegDone, "VID123")
	it := seedGrowthItem(t, gs, acct.ID)

	notes := svc.publishItem(context.Background(), acct, it)
	if yt.uploads != 0 {
		t.Errorf("uploads = %d, want 0 (đã đăng rồi, không đăng lại)", yt.uploads)
	}
	got, err := gs.GetItem(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != growth.ItemPublished {
		t.Errorf("item status = %s, want %s", got.Status, growth.ItemPublished)
	}
	if len(notes) == 0 {
		t.Error("want a skip note, got none")
	}
}

// TestPublishItemUploadsWhenLegNotDone: chưa có leg thì đăng bình thường.
func TestPublishItemUploadsWhenLegNotDone(t *testing.T) {
	svc, yt, gs, acct := newGrowthPublishService(t, "", "")
	it := seedGrowthItem(t, gs, acct.ID)

	svc.publishItem(context.Background(), acct, it)
	if yt.uploads != 1 {
		t.Errorf("uploads = %d, want 1", yt.uploads)
	}
}

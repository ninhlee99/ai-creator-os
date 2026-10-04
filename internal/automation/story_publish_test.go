package automation

// Đợt M1: PublishStoryNow dùng SEO meta + thumbnail qua MetaUploader,
// fallback Upload thường khi uploader không hỗ trợ.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type fakeMetaRunner struct {
	fakeStoryRunner
	assets []studio.Asset
}

func (f *fakeMetaRunner) ListAssets(jobID string) []studio.Asset { return f.assets }

// fakeAccounts (dùng chung từ reup_transform_test.go) trả về kênh test.

type fakeMetaUploader struct {
	gotMeta  publishers.VideoMeta
	gotKind  string
	metaUsed bool
}

func (f *fakeMetaUploader) State(a *network.Account) YTState { return YTState{} }
func (f *fakeMetaUploader) Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult {
	return publishers.PublishResult{Ok: true, Platform: "youtube", RemoteID: "vid1"}
}
func (f *fakeMetaUploader) UploadMeta(ctx context.Context, a *network.Account, videoPath string, meta publishers.VideoMeta, kind string) publishers.PublishResult {
	f.metaUsed = true
	f.gotMeta = meta
	f.gotKind = kind
	return publishers.PublishResult{Ok: true, Platform: "youtube", RemoteID: "vid1", ThumbnailSet: meta.ThumbnailPath != ""}
}

func TestPublishStoryNowUsesSEOMetaAndThumbnail(t *testing.T) {
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	thumb := filepath.Join(t.TempDir(), "scene1.jpg")
	if err := os.WriteFile(thumb, []byte{0xFF, 0xD8}, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &fakeMetaRunner{
		assets: []studio.Asset{{ID: 1, JobID: "st-d", Idx: 0, Kind: "photo", Path: thumb, Status: studio.StatusDone}},
	}
	r.jobs = map[string]studio.Job{
		"st-d": {ID: "st-d", Kind: studio.KindStory, Title: "Người mẹ", Status: studio.StatusDone,
			Output: "/tmp/x.mp4", Params: `{"topic":"Người mẹ","genre":"tình cảm"}`},
	}
	svc.Story = r
	svc.Accounts = &fakeAccounts{accts: []*network.Account{{Username: "kenh"}}}
	u := &fakeMetaUploader{}
	svc.Uploader = u

	note, ok := svc.PublishStoryNow(context.Background(), "st-d", "kenh")
	if !ok {
		t.Fatalf("publish thất bại: %s", note)
	}
	if !u.metaUsed {
		t.Fatal("phải dùng UploadMeta khi uploader hỗ trợ")
	}
	// không LLM → template trung thực: title từ tên truyện, có disclosure
	if !strings.Contains(u.gotMeta.Title, "Người mẹ") {
		t.Fatalf("meta.Title=%q, want từ tên truyện", u.gotMeta.Title)
	}
	if !strings.Contains(u.gotMeta.Description, "AI") {
		t.Fatalf("meta.Description thiếu disclosure: %q", u.gotMeta.Description)
	}
	if len(u.gotMeta.Tags) == 0 {
		t.Fatal("phải có tags")
	}
	if u.gotMeta.ThumbnailPath != thumb {
		t.Fatalf("thumbnail=%q, want %q", u.gotMeta.ThumbnailPath, thumb)
	}
	if u.gotKind != "short_film" {
		t.Fatalf("kind=%q", u.gotKind)
	}
	// log job ghi nhận
	found := false
	for _, a := range r.appended {
		if strings.Contains(a, "Đã đăng YouTube") {
			found = true
		}
	}
	if !found {
		t.Fatalf("job log thiếu dòng đăng: %v", r.appended)
	}
}

func TestPublishStoryNowFallsBackToPlainUpload(t *testing.T) {
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	r := &fakeStoryRunner{}
	r.jobs = map[string]studio.Job{
		"st-d": {ID: "st-d", Kind: studio.KindStory, Title: "X", Status: studio.StatusDone, Output: "/tmp/x.mp4"},
	}
	svc.Story = r
	svc.Accounts = &fakeAccounts{accts: []*network.Account{{Username: "kenh"}}}
	// uploader cũ không có UploadMeta
	plain := &fakePlainUploader{}
	svc.Uploader = plain
	if _, ok := svc.PublishStoryNow(context.Background(), "st-d", "kenh"); !ok {
		t.Fatal("uploader cũ phải vẫn đăng được qua Upload thường")
	}
	if !plain.used {
		t.Fatal("phải fallback Upload thường")
	}
}

type fakePlainUploader struct{ used bool }

func (f *fakePlainUploader) State(a *network.Account) YTState { return YTState{} }
func (f *fakePlainUploader) Upload(ctx context.Context, a *network.Account, videoPath, title, description, kind string) publishers.PublishResult {
	f.used = true
	return publishers.PublishResult{Ok: true, Platform: "youtube", RemoteID: "vid1"}
}

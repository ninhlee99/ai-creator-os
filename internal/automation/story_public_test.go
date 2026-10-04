package automation

// Đợt N: tự chuyển public sau thời gian chờ.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
)

var errTestBoom = errors.New("boom")

type fakePrivacyUploader struct {
	fakePlainUploader
	flipped map[string]string // videoID -> privacy
	failErr error
}

func (f *fakePrivacyUploader) UpdatePrivacy(ctx context.Context, a *network.Account, videoID, privacy string) error {
	if f.failErr != nil {
		return f.failErr
	}
	if f.flipped == nil {
		f.flipped = map[string]string{}
	}
	f.flipped[videoID] = privacy
	return nil
}

func publicTestService(t *testing.T) *Service {
	t.Helper()
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	svc.Accounts = &fakeAccounts{accts: []*network.Account{{Username: "kenh"}}}
	_ = svc.Settings.Set(KeyStoryAccount, "kenh")
	svc.Story = &fakeStoryRunner{}
	return svc
}

func setPending(t *testing.T, svc *Service, m map[string]string) {
	t.Helper()
	svc.savePendingPublic(m)
}

func TestStoryPublicTickFlipsDueVideos(t *testing.T) {
	svc := publicTestService(t)
	_ = svc.Settings.Set(KeyStoryPublicAfterHours, "24")
	u := &fakePrivacyUploader{}
	svc.Uploader = u
	old := time.Now().Add(-25 * time.Hour).Format("2006-01-02T15:04:05")
	fresh := time.Now().Format("2006-01-02T15:04:05")
	setPending(t, svc, map[string]string{
		"job-old":   old + "|vidOld",
		"job-fresh": fresh + "|vidFresh",
	})
	notes := svc.StoryPublicTick(context.Background())
	if len(notes) != 1 || !strings.Contains(notes[0], "tự chuyển công khai") {
		t.Fatalf("notes=%q, want 1 note chuyển public", notes)
	}
	if u.flipped["vidOld"] != "public" {
		t.Fatalf("vidOld chưa chuyển: %v", u.flipped)
	}
	if _, ok := u.flipped["vidFresh"]; ok {
		t.Fatal("vidFresh chưa tới giờ — không được chuyển")
	}
	// job-old đã ra khỏi hàng chờ
	if _, ok := svc.pendingPublic()["job-old"]; ok {
		t.Fatal("job-old phải ra khỏi hàng chờ sau khi chuyển")
	}
}

func TestStoryPublicTickOffByDefault(t *testing.T) {
	svc := publicTestService(t)
	u := &fakePrivacyUploader{}
	svc.Uploader = u
	setPending(t, svc, map[string]string{"job1": "2020-01-01T00:00:00|vid1"})
	if notes := svc.StoryPublicTick(context.Background()); len(notes) != 0 {
		t.Fatalf("mặc định 0 giờ = tắt, got %q", notes)
	}
	if len(u.flipped) != 0 {
		t.Fatal("không được chuyển khi tắt")
	}
}

func TestStoryPublicTickRespectsKillAndDryRun(t *testing.T) {
	svc := publicTestService(t)
	_ = svc.Settings.Set(KeyStoryPublicAfterHours, "1")
	u := &fakePrivacyUploader{}
	svc.Uploader = u
	setPending(t, svc, map[string]string{"job1": "2020-01-01T00:00:00|vid1"})
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.StoryPublicTick(context.Background()); len(notes) != 0 {
		t.Fatalf("kill phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.StoryPublicTick(context.Background()); len(notes) != 0 {
		t.Fatalf("dry-run phải chặn, got %q", notes)
	}
}

func TestStoryPublicTickKeepsFailedForRetry(t *testing.T) {
	svc := publicTestService(t)
	_ = svc.Settings.Set(KeyStoryPublicAfterHours, "1")
	u := &fakePrivacyUploader{failErr: errTestBoom}
	svc.Uploader = u
	setPending(t, svc, map[string]string{"job1": "2020-01-01T00:00:00|vid1"})
	notes := svc.StoryPublicTick(context.Background())
	if len(notes) != 1 || !strings.Contains(notes[0], "thất bại") {
		t.Fatalf("notes=%q, want ghi nhận lỗi", notes)
	}
	if _, ok := svc.pendingPublic()["job1"]; !ok {
		t.Fatal("lỗi API → giữ lại hàng chờ để thử lại")
	}
}

func TestEnqueuePublicOnlyWhenEnabled(t *testing.T) {
	svc := publicTestService(t)
	svc.enqueuePublic("j1", "vid1")
	if len(svc.pendingPublic()) != 0 {
		t.Fatal("mặc định tắt → không hẹn giờ")
	}
	_ = svc.Settings.Set(KeyStoryPublicAfterHours, "24")
	svc.enqueuePublic("j2", "vid2")
	if _, ok := svc.pendingPublic()["j2"]; !ok {
		t.Fatal("bật 24h → phải hẹn giờ")
	}
	svc.enqueuePublic("j3", "")
	if _, ok := svc.pendingPublic()["j3"]; ok {
		t.Fatal("thiếu videoID → không hẹn")
	}
}

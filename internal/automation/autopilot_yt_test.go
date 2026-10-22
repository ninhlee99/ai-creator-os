package automation

// Đợt Q: autoPublishYouTubeAffiliate — video affiliate tự lên YouTube
// Shorts (private-first), quota-guarded.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// fakeJobStore: GetJob + AppendLog ghi lại để assert.
type fakeJobStore struct {
	jobs map[string]studio.Job
	logs []string
	legs map[string]string
}

func (f *fakeJobStore) GetJob(id string) (studio.Job, bool) {
	j, ok := f.jobs[id]
	return j, ok
}

func (f *fakeJobStore) AppendLog(id, line string) {
	f.logs = append(f.logs, id+":"+line)
}

// Publish legs (Đợt Y): fake lưu trạng thái trong map để test idempotency.
func (f *fakeJobStore) PublishLegStatus(jobID, platform string) string {
	return f.legs[jobID+"/"+platform]
}

// Đợt AA: remote id lưu cùng key với hậu tố "/rid".
func (f *fakeJobStore) PublishLegRemoteID(jobID, platform string) string {
	return f.legs[jobID+"/"+platform+"/rid"]
}

func (f *fakeJobStore) SetPublishLeg(jobID, platform, status, remoteID string) error {
	if f.legs == nil {
		f.legs = map[string]string{}
	}
	f.legs[jobID+"/"+platform] = status
	f.legs[jobID+"/"+platform+"/rid"] = remoteID
	return nil
}

func (f *fakeJobStore) FailedPublishLegs() []studio.PublishLeg { return nil }

func (f *fakeJobStore) has(substr string) bool {
	for _, l := range f.logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

// ytTestService dựng service cho autopilot YouTube: ledger + growth +
// fake stores + OAuth/HTTP giả.
func ytTestService(t *testing.T) (*Service, *fakeJobStore, *growth.Store, *int, *[]byte) {
	t.Helper()
	l, _ := openTestLedger(t)
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "growth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	gs, err := growth.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	js := &fakeJobStore{jobs: map[string]studio.Job{}}
	svc.JobStore = js
	svc.Accounts = &fakeAccounts{accts: []*network.Account{
		{ID: 7, Username: "aff", YoutubeChannel: "UCx", YoutubeContentTypes: []string{"short_video"}},
	}}
	svc.Growth = gs

	t.Setenv("YOUTUBE_CLIENT_ID", "cid")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "csec")
	tokDir := t.TempDir()
	publishers.SetTokenDir(tokDir)
	t.Cleanup(func() { publishers.SetTokenDir(".") })
	if err := os.WriteFile(filepath.Join(tokDir, "youtube_token_aff.json"),
		[]byte(`{"refresh_token":"r"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := 0
	var initBody []byte
	old := youtubeAffiliatePublisher
	youtubeAffiliatePublisher = func(acct *network.Account) *publishers.YouTubePublisher {
		pub := publishers.NewYouTubePublisher(acct.Username, acct.YoutubeContentTypes, acct.YoutubeChannel)
		pub.Synthetic = true
		pub.HTTP = func(method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
			calls++
			switch {
			case strings.Contains(url, "oauth2.googleapis.com/token"):
				return 200, nil, []byte(`{"access_token":"at"}`), nil
			case method == "POST":
				initBody = body // metadata đăng (title/desc/tags)
				return 200, map[string]string{"Location": "https://upload/session"}, []byte(`{}`), nil
			default:
				return 200, nil, []byte(`{"id":"vid123"}`), nil
			}
		}
		return pub
	}
	t.Cleanup(func() { youtubeAffiliatePublisher = old })
	return svc, js, gs, &calls, &initBody
}

func ytTestJob(t *testing.T) studio.Job {
	t.Helper()
	out := filepath.Join(t.TempDir(), "aff.mp4")
	if err := os.WriteFile(out, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(studio.AffiliateParams{
		AccountID: 7, ProductName: "Kem chống nắng",
		AffLink: "https://accesstrade.vn/xyz",
	})
	return studio.Job{ID: "job-1", Kind: studio.KindAffiliate, Title: "Kem chống nắng",
		Status: studio.StatusDone, Output: out, Params: string(raw)}
}

func TestAutoPublishYouTubeAffiliateOK(t *testing.T) {
	svc, js, gs, calls, initBody := ytTestService(t)
	j := ytTestJob(t)
	js.jobs[j.ID] = j
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	if *calls == 0 {
		t.Fatal("không gọi YouTube API")
	}
	if !js.has("Đã đăng YouTube Shorts (private)") {
		t.Fatalf("thiếu log đăng thành công: %q", js.logs)
	}
	if !strings.Contains(string(*initBody), "https://accesstrade.vn/xyz") {
		t.Fatalf("mô tả thiếu link affiliate: %s", initBody)
	}
	used, _ := gs.QuotaUsed(svc.quotaDay())
	if used < growth.UploadCostUnits {
		t.Fatalf("quota chưa được ghi nhận, used=%d", used)
	}
}

func TestAutoPublishYouTubeAffiliateQuotaGuard(t *testing.T) {
	svc, js, gs, calls, _ := ytTestService(t)
	_ = gs.AddQuota(svc.quotaDay(), growth.DailyQuotaUnits) // cạn quota
	j := ytTestJob(t)
	js.jobs[j.ID] = j
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	if *calls != 0 {
		t.Fatalf("hết quota vẫn gọi API (%d calls)", *calls)
	}
	if !js.has("chờ quota") {
		t.Fatalf("thiếu log chờ quota trung thực: %q", js.logs)
	}
}

func TestAutoPublishYouTubeAffiliateToggleOff(t *testing.T) {
	svc, js, _, calls, _ := ytTestService(t)
	_ = svc.Settings.Set("autopilot_youtube_enabled", "0")
	_ = svc.Settings.Set("autopilot_auto_publish", "0") // tắt luôn TikTok để cô lập
	j := ytTestJob(t)
	js.jobs[j.ID] = j
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	if *calls != 0 {
		t.Fatal("đã tắt vẫn đăng")
	}
}

// Đợt Y: hook fire 2 lần cho cùng job không được đăng trùng — chân done
// thì lần 2 bỏ qua.
func TestAutoPublishYouTubeIdempotent(t *testing.T) {
	svc, js, _, calls, _ := ytTestService(t)
	_ = svc.Settings.Set("autopilot_auto_publish", "0") // tắt TikTok để cô lập
	j := ytTestJob(t)
	js.jobs[j.ID] = j
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	afterFirst := *calls
	if afterFirst == 0 {
		t.Fatal("lần 1 không gọi YouTube API")
	}
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	if *calls != afterFirst {
		t.Fatalf("lần 2 gọi thêm API (%d → %d) — mất idempotency", afterFirst, *calls)
	}
	if !js.has("đã đăng rồi — bỏ qua") {
		t.Fatalf("thiếu log bỏ qua lần 2: %q", js.logs)
	}
	if got := js.PublishLegStatus(j.ID, "youtube"); got != studio.LegDone {
		t.Fatalf("leg youtube = %q, muốn done", got)
	}
}

// Đợt Y: chân lỗi tạm thời (hết quota) được đánh failed để tick thử lại.
func TestAutoPublishYouTubeQuotaMarksLegFailed(t *testing.T) {
	svc, js, gs, _, _ := ytTestService(t)
	_ = svc.Settings.Set("autopilot_auto_publish", "0")
	_ = gs.AddQuota(svc.quotaDay(), growth.DailyQuotaUnits) // cạn quota
	j := ytTestJob(t)
	js.jobs[j.ID] = j
	svc.AutoPublishAffiliate(context.Background(), j.ID)
	if got := js.PublishLegStatus(j.ID, "youtube"); got != studio.LegFailed {
		t.Fatalf("leg youtube = %q, muốn failed để retry", got)
	}
}

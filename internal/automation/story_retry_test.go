package automation

// Đợt P: StoryRetryTick tự thử lại job story lỗi (backoff + giới hạn).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// fakeRetryStudio: failed jobs cho sẵn + ghi params retry.
type fakeRetryStudio struct {
	fakeStoryRunner
	failedJobs []studio.Job
}

func (f *fakeRetryStudio) StoryJobs(limit int) []studio.Job {
	if limit > 0 && len(f.failedJobs) > limit {
		return f.failedJobs[:limit]
	}
	return f.failedJobs
}

func failedStoryJob(id, title string, attempt int, failedAt time.Time) studio.Job {
	sp := studio.StoryParams{Topic: title, Genre: "tâm lý", Words: 600, Scenes: 6, Attempt: attempt}
	raw, _ := json.Marshal(sp)
	return studio.Job{
		ID: id, Kind: studio.KindStory, Title: title,
		Status: studio.StatusFailed, Params: string(raw),
		CreatedAt:  failedAt.Add(-time.Hour).Format("2006-01-02T15:04:05"),
		FinishedAt: failedAt.Format("2006-01-02T15:04:05"),
	}
}

func retryTestService(t *testing.T, jobs []studio.Job) *Service {
	t.Helper()
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{}
	svc.Story = &fakeRetryStudio{fakeStoryRunner: fakeStoryRunner{nextID: "st-retry"}, failedJobs: jobs}
	return svc
}

func TestStoryRetryTickRetriesOldFailedJob(t *testing.T) {
	j := failedStoryJob("st-old", "truyện cũ", 0, time.Now().Add(-8*time.Hour))
	svc := retryTestService(t, []studio.Job{j})
	notes := svc.StoryRetryTick(context.Background())
	if len(notes) != 1 {
		t.Fatalf("notes=%q, want 1 (thử lại)", notes)
	}
	r := svc.Story.(*fakeRetryStudio)
	if len(r.params) != 1 {
		t.Fatalf("params=%d, want 1", len(r.params))
	}
	if r.params[0].Attempt != 1 {
		t.Fatalf("attempt=%d, want 1", r.params[0].Attempt)
	}
	if v, ok := svc.Settings.Get(KeyStoryRetriedPrefix + "st-old"); !ok || v != "1" {
		t.Fatal("thiếu dấu đã-thử-lại cho job cũ")
	}
	if len(r.appended) != 1 || !strings.Contains(r.appended[0], "st-old") {
		t.Fatalf("log job cũ không được ghi: %q", r.appended)
	}
}

func TestStoryRetryTickCooldownAndMax(t *testing.T) {
	// Mới lỗi 1h trước (cooldown 6h) → chưa thử.
	fresh := failedStoryJob("st-fresh", "mới lỗi", 0, time.Now().Add(-time.Hour))
	// Đã thử 3 lần (max) → báo hết lượt 1 lần duy nhất.
	maxed := failedStoryJob("st-maxed", "hết lượt", 3, time.Now().Add(-8*time.Hour))
	// Đã có dấu retried → không thử lại lần 2.
	done := failedStoryJob("st-done", "đã thử", 1, time.Now().Add(-8*time.Hour))
	svc := retryTestService(t, []studio.Job{fresh, maxed, done})
	_ = svc.Settings.Set(KeyStoryRetriedPrefix+"st-done", "1")
	notes := svc.StoryRetryTick(context.Background())
	if len(notes) != 1 || !strings.Contains(notes[0], "hết 3 lần") {
		t.Fatalf("notes=%q, want 1 note báo hết lượt", notes)
	}
	// Tick sau: đã báo rồi → im lặng, không spam.
	if notes := svc.StoryRetryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("đã báo thì không spam, got %q", notes)
	}
}

func TestStoryRetryTickRespectsKillAndOff(t *testing.T) {
	j := failedStoryJob("st-old", "truyện cũ", 0, time.Now().Add(-8*time.Hour))
	svc := retryTestService(t, []studio.Job{j})
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.StoryRetryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("kill phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.StoryRetryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("dry-run phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{}
	_ = svc.Settings.Set(KeyStoryRetryEnabled, "0")
	if notes := svc.StoryRetryTick(context.Background()); len(notes) != 0 {
		t.Fatalf("tắt retry phải im lặng, got %q", notes)
	}
}

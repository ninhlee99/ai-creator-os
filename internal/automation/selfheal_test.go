package automation

// Đợt L: self-heal tick — quét job treo + canh đĩa.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeHealer mở rộng fakeStoryRunner với khả năng tự chữa của Studio.
type fakeHealer struct {
	fakeStoryRunner
	swept        int
	prunedFailed int
	prunedOld    int
	freed        int64
}

func (f *fakeHealer) SweepHungJobs(maxAge time.Duration) int { return f.swept }
func (f *fakeHealer) PruneFailedWork() (int, int64)          { return f.prunedFailed, f.freed }
func (f *fakeHealer) PruneOldPublishedOutputs(maxAge time.Duration, isPublished func(id string) bool) (int, int64) {
	return f.prunedOld, 0
}

func selfHealTestService(t *testing.T) (*Service, *fakeHealer) {
	t.Helper()
	l, _ := openTestLedger(t)
	svc := NewService()
	svc.Settings = LedgerSettings{L: l}
	svc.Gate = &fakeGate{} // không kill, không dry-run
	h := &fakeHealer{}
	svc.Story = h
	svc.DataDir = t.TempDir() // đĩa test gần như trống
	return svc, h
}

func TestSelfHealTickSweepsHungJobs(t *testing.T) {
	svc, h := selfHealTestService(t)
	h.swept = 2
	notes := svc.SelfHealTick(context.Background())
	found := false
	for _, n := range notes {
		if strings.Contains(n, "treo quá 12h") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notes=%q, want ghi nhận 2 job treo", notes)
	}
	// chạy lại ngay → bị chặn bởi interval 1 giờ
	if notes := svc.SelfHealTick(context.Background()); len(notes) != 0 {
		t.Fatalf("lần 2 phải im lặng (interval), got %q", notes)
	}
}

func TestSelfHealTickRespectsKillAndDryRun(t *testing.T) {
	svc, h := selfHealTestService(t)
	h.swept = 3
	svc.Gate = &fakeGate{kill: true}
	if notes := svc.SelfHealTick(context.Background()); len(notes) != 0 {
		t.Fatalf("kill switch phải chặn, got %q", notes)
	}
	svc.Gate = &fakeGate{dry: true}
	if notes := svc.SelfHealTick(context.Background()); len(notes) != 0 {
		t.Fatalf("dry-run phải chặn, got %q", notes)
	}
}

func TestSelfHealDiskGuardPrunesWhenFull(t *testing.T) {
	svc, h := selfHealTestService(t)
	// ép ngưỡng xuống 0% → đĩa test (gần trống nhưng >0%) vẫn "đầy"
	_ = svc.Settings.Set(KeyDiskMaxPct, "0")
	h.prunedFailed = 3
	h.freed = 1024 * 1024
	notes := svc.SelfHealTick(context.Background())
	found := false
	for _, n := range notes {
		if strings.Contains(n, "đã dọn 3 job lỗi") && strings.Contains(n, "1.0 MB") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notes=%q, want ghi nhận dọn đĩa", notes)
	}
}

func TestSelfHealDiskGuardQuietWhenHealthy(t *testing.T) {
	svc, _ := selfHealTestService(t)
	// ngưỡng 100% → đĩa test không bao giờ đầy
	_ = svc.Settings.Set(KeyDiskMaxPct, "100")
	if notes := svc.SelfHealTick(context.Background()); len(notes) != 0 {
		t.Fatalf("đĩa khỏe phải im lặng, got %q", notes)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 1024: "1.0 KB", 1536 * 1024: "1.5 MB"}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Fatalf("formatBytes(%d)=%q, want %q", in, got, want)
		}
	}
}

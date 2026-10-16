package harness

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ninhlee99/ai-creator-os/internal/network"
)

type fakeLLM struct{}

func (f *fakeLLM) Complete(ctx context.Context, system, prompt string) (string, error) {
	return "Chẩn đoán: Danh mục kênh đang phát triển tốt; ưu tiên video viral 9:16 và sản phẩm affiliate chủ lực.", nil
}

func (f *fakeLLM) Name() string                     { return "fake-llm" }
func (f *fakeLLM) Healthy(ctx context.Context) bool { return true }

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "harness_test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestMemoryStore(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ms, err := NewMemoryStore(db)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	ctx := context.Background()

	// 1. Ghi nhớ 1 insight tích cực
	err = ms.RecordInsight(ctx, AgentMemory{
		MemoryType:      MemHookPerformance,
		ChannelID:       "chan-tech",
		Niche:           "công nghệ",
		FormatID:        "hook_top3_errors",
		Title:           "Hook Top 3 Sai Lầm Đạt View Khủng",
		Insight:         "Câu mở đầu dạng 'Top 3 sai lầm khiến bạn mất tiền' giữ chân 82% người xem ở 3s đầu.",
		ConfidenceScore: 0.9,
		ImpactScore:     0.85,
		SampleSize:      5,
	})
	if err != nil {
		t.Fatalf("RecordInsight pos: %v", err)
	}

	// 2. Ghi nhớ 1 insight tiêu cực
	err = ms.RecordInsight(ctx, AgentMemory{
		MemoryType:      MemHookPerformance,
		ChannelID:       "chan-tech",
		Niche:           "công nghệ",
		FormatID:        "hook_boring_intro",
		Title:           "Lời Chào Dài Khiến Người Xem Lướt Qua",
		Insight:         "Chào hỏi rườm rà ở 2s đầu làm rớt 70% khán giả.",
		ConfidenceScore: 0.95,
		ImpactScore:     -0.8,
		SampleSize:      3,
	})
	if err != nil {
		t.Fatalf("RecordInsight neg: %v", err)
	}

	// 3. Truy vấn
	mems, err := ms.QueryRelevantMemories(ctx, "công nghệ", "chan-tech", 10)
	if err != nil {
		t.Fatalf("QueryRelevantMemories: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(mems))
	}

	// 4. Sinh prompt context
	promptCtx := ms.GeneratePromptContext(ctx, "công nghệ", "chan-tech")
	if !strings.Contains(promptCtx, "Hook Top 3 Sai Lầm") {
		t.Errorf("prompt context missing positive insight: %s", promptCtx)
	}
	if !strings.Contains(promptCtx, "Lời Chào Dài") {
		t.Errorf("prompt context missing negative warning: %s", promptCtx)
	}
}

func TestOODACycleAndHarness(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	h, err := New(db, nil, nil, nil, &fakeLLM{}, Config{
		AutonomyLevel: AutonomyFullyAutonomous,
		Interval:      1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("New harness: %v", err)
	}

	ctx := context.Background()

	// Chạy chu kỳ nhận thức OODA với 2 tài khoản mô phỏng
	fakeAccounts := []*network.Account{
		{ID: 1, Username: "tech_reviewer", Niche: "công nghệ", Followers: 2500, Status: "format_testing"},
		{ID: 2, Username: "bep_me_bi", Niche: "gia dụng", Followers: 12000, Status: "scaling"},
	}

	obs := h.ooda.Observe(ctx, fakeAccounts, 1, 10)
	if obs.TotalChannels != 2 {
		t.Errorf("expected 2 channels, got %d", obs.TotalChannels)
	}

	diag, tasks := h.ooda.OrientAndDecide(ctx, obs, AutonomyFullyAutonomous)
	if diag == "" {
		t.Errorf("expected non-empty diagnosis")
	}
	if len(tasks) == 0 {
		t.Errorf("expected decision tasks generated")
	}

	res, err := h.ooda.ExecuteCycle(ctx, fakeAccounts, nil, AutonomyFullyAutonomous)
	if err != nil {
		t.Fatalf("ExecuteCycle: %v", err)
	}
	if res.Status != "completed" {
		t.Errorf("expected completed status, got %s", res.Status)
	}

	// Kiểm tra cycle trực tiếp qua Harness
	cycleRes, err := h.RunCycle(ctx)
	if err != nil {
		t.Fatalf("h.RunCycle: %v", err)
	}
	if cycleRes == nil {
		t.Fatalf("expected non-nil cycleRes")
	}

	latest := h.LatestCycle()
	if latest == nil || latest.CycleID != cycleRes.CycleID {
		t.Errorf("expected matching latest cycle ID")
	}

	cycles := h.ListRecentCycles(5)
	if len(cycles) == 0 {
		t.Errorf("expected at least 1 recent cycle")
	}

	// Kiểm tra GetCycle
	fetched, err := h.GetCycle(cycleRes.CycleID)
	if err != nil {
		t.Fatalf("h.GetCycle: %v", err)
	}
	if fetched.CycleID != cycleRes.CycleID {
		t.Errorf("GetCycle ID mismatch: %s != %s", fetched.CycleID, cycleRes.CycleID)
	}

	// Kiểm tra MemoryStore.Delete và GetMemory
	delMemID := "mem-del-unit"
	err = h.Memory().RecordInsight(ctx, AgentMemory{
		ID:         delMemID,
		MemoryType: MemConversionInsight,
		Title:      "Temp insight",
		Insight:    "Will be deleted",
	})
	if err != nil {
		t.Fatalf("RecordInsight: %v", err)
	}
	gotMem, err := h.Memory().GetMemory(ctx, delMemID)
	if err != nil || gotMem == nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if err := h.Memory().Delete(ctx, delMemID); err != nil {
		t.Fatalf("ms.Delete: %v", err)
	}
	if _, err := h.Memory().GetMemory(ctx, delMemID); err == nil {
		t.Errorf("expected error after deleting memory, got nil")
	}
}

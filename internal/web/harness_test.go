package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/harness"
)

func newHarnessServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)

	h, err := harness.New(
		s.DB(),
		s.Studio,
		s.Mgr,
		s.Growth,
		nil,
		harness.Config{
			AutonomyLevel: harness.AutonomyFullyAutonomous,
			Interval:      1 * time.Hour,
		},
	)
	if err != nil {
		t.Fatalf("harness.New: %v", err)
	}
	s.Harness = h
	return s
}

func TestHarnessPageNoHarness(t *testing.T) {
	s := newTestServer(t) // Harness == nil
	rec := get(t, s, "/harness")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /harness = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Agent Harness") {
		t.Errorf("trang /harness thiếu tiêu đề Agent Harness: %s", body)
	}
	if !strings.Contains(body, "Disconnected") {
		t.Errorf("trang /harness thiếu nhãn Disconnected: %s", body)
	}
}

func TestHarnessPageWithHarness(t *testing.T) {
	s := newHarnessServer(t)
	rec := get(t, s, "/harness")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /harness = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Agent Harness",
		"Level 3: Fully Autonomous",
		"Run OODA Cycle Now",
		"Episodic Memory Bank",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /harness thiếu nội dung %q", want)
		}
	}
}

func TestHarnessRunCycle(t *testing.T) {
	s := newHarnessServer(t)

	rec := postForm(t, s, "/harness/cycle", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /harness/cycle = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=") {
		t.Errorf("POST /harness/cycle redirect thiếu notice: %s", loc)
	}

	// Xác minh chu kỳ đã được lưu vào Harness
	latest := s.Harness.LatestCycle()
	if latest == nil {
		t.Fatalf("LatestCycle nil sau khi chạy chu kỳ")
	}
	if latest.Diagnosis == "" {
		t.Errorf("LatestCycle chẩn đoán rỗng")
	}
}

func TestHarnessAutonomyUpdate(t *testing.T) {
	s := newHarnessServer(t)

	// Đổi sang cấp độ 1
	rec := postForm(t, s, "/harness/autonomy", url.Values{
		"level": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /harness/autonomy = %d, want 303", rec.Code)
	}
	if s.Harness.AutonomyLevel() != harness.AutonomyAssisted {
		t.Errorf("AutonomyLevel = %v, want Assisted (1)", s.Harness.AutonomyLevel())
	}

	// Đổi sang cấp độ 2
	rec = postForm(t, s, "/harness/autonomy", url.Values{
		"level": {"2"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /harness/autonomy = %d, want 303", rec.Code)
	}
	if s.Harness.AutonomyLevel() != harness.AutonomySemiAutonomous {
		t.Errorf("AutonomyLevel = %v, want SemiAutonomous (2)", s.Harness.AutonomyLevel())
	}
}

func TestHarnessMemoryAdd(t *testing.T) {
	s := newHarnessServer(t)

	rec := postForm(t, s, "/harness/memory", url.Values{
		"domain":  {"hook_performance"},
		"channel": {"ai_tech_trends"},
		"subject": {"3s Pattern Interrupt"},
		"insight": {"Cắt khung hình bất ngờ kèm âm thanh sấm sét giúp tăng tỷ lệ xem tiếp thêm 42%"},
		"score":   {"0.95"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /harness/memory = %d, want 303", rec.Code)
	}

	// Lấy lại từ store
	mems, err := s.Harness.Memory().ListAllMemories(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAllMemories: %v", err)
	}
	found := false
	for _, m := range mems {
		if strings.Contains(m.Insight, "Cắt khung hình bất ngờ") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Không tìm thấy ký ức vừa nạp trong store")
	}
}

func TestHarnessCycleDetailAPI(t *testing.T) {
	s := newHarnessServer(t)

	// Run 1 cycle first so we have a cycle ID
	cycle, err := s.Harness.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}

	rec := get(t, s, "/api/harness/cycles/"+cycle.CycleID)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/harness/cycles/%s = %d, want 200", cycle.CycleID, rec.Code)
	}

	var res harness.OODACycleResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Unmarshal cycle JSON: %v", err)
	}
	if res.CycleID != cycle.CycleID {
		t.Errorf("CycleID = %s, want %s", res.CycleID, cycle.CycleID)
	}
	if res.Status != "completed" {
		t.Errorf("Status = %s, want completed", res.Status)
	}
}

func TestHarnessPromptPreviewAPI(t *testing.T) {
	s := newHarnessServer(t)

	// Seed an insight
	_ = s.Harness.Memory().RecordInsight(context.Background(), harness.AgentMemory{
		MemoryType:      harness.MemHookPerformance,
		Niche:           "tech",
		ChannelID:       "chan-1",
		Title:           "Fast Cut",
		Insight:         "Fast cut in first 3s increases retention",
		ConfidenceScore: 0.9,
		ImpactScore:     0.8,
	})

	rec := get(t, s, "/api/harness/prompt-context?niche=tech&channel=chan-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/harness/prompt-context = %d, want 200", rec.Code)
	}

	var data map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("Unmarshal prompt-context JSON: %v", err)
	}
	promptCtx, _ := data["prompt_context"].(string)
	if !strings.Contains(promptCtx, "Fast cut in first 3s") {
		t.Errorf("prompt_context missing seeded insight: %s", promptCtx)
	}
}

func TestHarnessMemoryDelete(t *testing.T) {
	s := newHarnessServer(t)

	memID := "mem-test-del"
	_ = s.Harness.Memory().RecordInsight(context.Background(), harness.AgentMemory{
		ID:              memID,
		MemoryType:      harness.MemHookPerformance,
		Title:           "To be deleted",
		Insight:         "Delete me",
		ConfidenceScore: 0.8,
	})

	rec := postForm(t, s, "/harness/memory/delete", url.Values{
		"id": {memID},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /harness/memory/delete = %d, want 303", rec.Code)
	}

	mems, err := s.Harness.Memory().ListAllMemories(context.Background(), 100)
	if err != nil {
		t.Fatalf("ListAllMemories: %v", err)
	}
	for _, m := range mems {
		if m.ID == memID {
			t.Errorf("Ký ức %s vẫn còn sau khi xóa", memID)
		}
	}
}


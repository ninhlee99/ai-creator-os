package web

import (
	"context"
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
	if !strings.Contains(body, "Bộ Não Tự Hành") {
		t.Errorf("trang /harness thiếu tiêu đề: %s", body)
	}
	if !strings.Contains(body, "Chưa kết nối") {
		t.Errorf("trang /harness thiếu nhãn chưa kết nối: %s", body)
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
		"Bộ Não Tự Hành Cấp Cao (Agent Harness)",
		"Cấp độ 3: Tự hành toàn diện (Fully Autonomous)",
		"Kích hoạt Chu kỳ Nhận thức OODA ngay",
		"Kho Ký Ức Thực Chiến",
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

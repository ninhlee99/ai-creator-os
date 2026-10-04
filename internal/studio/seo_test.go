package studio

import (
	"context"
	"strings"
	"testing"
)

// Đợt M1: SEO metadata cho video truyện.

func TestTemplateSEOMeta(t *testing.T) {
	m := TemplateSEOMeta("Người mẹ", "tình cảm", "AI disclosure.")
	if !strings.Contains(m.Title, "Người mẹ") {
		t.Fatalf("title=%q, want chứa tên truyện", m.Title)
	}
	if len([]rune(m.Title)) > 100 {
		t.Fatalf("title dài %d rune, want <= 100", len([]rune(m.Title)))
	}
	if !strings.Contains(m.Description, "AI disclosure.") {
		t.Fatal("description phải giữ disclosure")
	}
	if len(m.Tags) < 3 {
		t.Fatalf("tags=%v, want >= 3", m.Tags)
	}
	// tiêu đề rất dài → cắt theo rune, không vỡ ký tự
	long := strings.Repeat("ă", 200)
	m2 := TemplateSEOMeta(long, "", "")
	if len([]rune(m2.Title)) > 100 {
		t.Fatalf("title dài %d rune sau khi cắt", len([]rune(m2.Title)))
	}
}

func TestGenerateSEOMetaNoLLM(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir+"/studio.db", nil, nil, nil, dir+"/out")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GenerateSEOMeta(context.Background(), "topic", "tình cảm", "fallback"); err == nil {
		t.Fatal("thiếu LLM phải trả lỗi để caller dùng template")
	}
}

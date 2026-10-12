package automation

// Đợt Y: test marker "đang đăng" và phân loại lỗi cấu hình.

import (
	"testing"
	"time"
)

func TestParsePublishingMarker(t *testing.T) {
	title, ts := parsePublishingMarker("Tiêu đề hay|" + time.Now().Format(time.RFC3339))
	if title != "Tiêu đề hay" || ts == "" {
		t.Fatalf("parse sai: %q %q", title, ts)
	}
	// Legacy đợt S/V lưu "1" → coi như đăng dở cũ (không title).
	if title, ts := parsePublishingMarker("1"); title != "" || ts != "" {
		t.Fatalf("legacy '1' phải trả rỗng, được %q %q", title, ts)
	}
	if title, ts := parsePublishingMarker(""); title != "" || ts != "" {
		t.Fatalf("rỗng phải trả rỗng, được %q %q", title, ts)
	}
}

func TestFreshPublishingMarker(t *testing.T) {
	fresh := time.Now().Format(time.RFC3339)
	if !freshPublishingMarker(fresh) {
		t.Fatal("marker vừa đặt phải là fresh")
	}
	old := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	if freshPublishingMarker(old) {
		t.Fatal("marker 2h trước phải là stale (đăng dở)")
	}
	if freshPublishingMarker("") {
		t.Fatal("timestamp rỗng không được coi là fresh")
	}
}

func TestIsStoryConfigError(t *testing.T) {
	for _, s := range []string{
		"LỖI: media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)",
		"LỖI: cần LLM để viết truyện (chưa cấu hình)",
		"LỖI: TTS chưa sẵn sàng (không có chuỗi giọng đọc)",
		"something invalid key happened",
	} {
		if !isStoryConfigError(s) {
			t.Fatalf("phải nhận diện lỗi cấu hình: %q", s)
		}
	}
	for _, s := range []string{
		"LỖI: cảnh 3: vẽ ảnh lỗi (429 quota hết) — dừng",
		"LỖI: ghép audio: ffmpeg exit status 1",
		"story: đăng thất bại — network timeout",
	} {
		if isStoryConfigError(s) {
			t.Fatalf("không được nhầm lỗi thoáng qua thành lỗi cấu hình: %q", s)
		}
	}
}

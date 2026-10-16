package web

import (
	"net/url"
	"strings"
	"testing"
)

// Lỗi thật (12/10/2026): nhập key Gemini ở mục giọng đọc → viết chữ và tạo
// ảnh vẫn 0 key. Một key phải dùng chung cho cả 3, thêm/xóa đều đồng bộ.
func TestGeminiKeySharedAcrossChainsAndImages(t *testing.T) {
	t.Setenv("GEMINI_API_KEYS", "")
	t.Setenv("GEMINI_API_KEY", "")
	s := newTestServer(t)
	var media []string
	s.OnGeminiKeys = func(k []string) { media = k }

	rec := postForm(t, s, "/settings/keys/add?chain=tts&provider=gemini", url.Values{"key": {"AIzaSyTEST_key_000000000000000000000000"}})
	if rec.Code != 200 {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	if got := s.storedKeys("llm", "gemini"); len(got) != 1 || got[0] != "AIzaSyTEST_key_000000000000000000000000" {
		t.Fatalf("chuỗi viết chữ phải có key: %v", got)
	}
	if len(media) != 1 {
		t.Fatalf("tạo ảnh phải nhận key: %v", media)
	}
	rec = postForm(t, s, "/settings/keys/delete?chain=llm&provider=gemini", url.Values{"idx": {"0"}})
	if rec.Code != 200 {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if len(s.storedKeys("tts", "gemini")) != 0 || len(media) != 0 {
		t.Fatalf("xóa phải đồng bộ: tts=%v media=%v", s.storedKeys("tts", "gemini"), media)
	}
}

func TestGeminiKeyFormatRejected(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/settings/keys/add?chain=tts&provider=gemini", url.Values{"key": {"gen-abcdefghijklmnopqrstuv"}})
	if rec.Code != 400 || len(s.storedKeys("tts", "gemini")) != 0 {
		t.Fatalf("key sai định dạng phải bị từ chối, got %d", rec.Code)
	}
	if !looksLikeGeminiKey("AIza" + strings.Repeat("x", 35)) {
		t.Fatal("key AIza đúng dạng phải được nhận")
	}
	if !looksLikeGeminiKey("AQ.Ab8RN6JyFbfvKom9YnHcmTestKey12345678901234567890") {
		t.Fatal("key AQ. đúng dạng phải được nhận")
	}
	if looksLikeGeminiKey("AQ.short") {
		t.Fatal("key AQ. quá ngắn phải bị từ chối")
	}
}

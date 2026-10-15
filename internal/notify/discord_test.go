package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendDiscord(t *testing.T) {
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := SendDiscord(srv.URL, "Tiêu đề", "Nội dung thông báo"); err != nil {
		t.Fatalf("SendDiscord: %v", err)
	}
	embeds, ok := gotPayload["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatalf("want embeds in payload, got %v", gotPayload)
	}
	first := embeds[0].(map[string]any)
	if first["title"] != "Tiêu đề" || first["description"] != "Nội dung thông báo" {
		t.Fatalf("unexpected embed content: %v", first)
	}
}

func TestSendDiscordFailClosed(t *testing.T) {
	if err := SendDiscord("", "Tiêu đề", "Nội dung"); err == nil {
		t.Fatal("thiếu webhook URL phải lỗi")
	}
	if err := SendDiscord("https://discord.com/webhook", "", "  "); err == nil {
		t.Fatal("nội dung trống phải lỗi")
	}
}

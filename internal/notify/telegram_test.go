package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendTelegram(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	if err := SendTelegram("tok123", "987", "chào máy"); err != nil {
		t.Fatalf("SendTelegram: %v", err)
	}
	if gotPath != "/bottok123/sendMessage" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody["chat_id"] != "987" || gotBody["text"] != "chào máy" {
		t.Fatalf("body=%v", gotBody)
	}
}

func TestSendTelegramRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"description":"bad request"}`))
	}))
	defer srv.Close()
	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	if err := SendTelegram("tok", "1", "x"); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("want lỗi từ chối rõ ràng, got %v", err)
	}
}

func TestSendTelegramFailClosed(t *testing.T) {
	if err := SendTelegram("", "1", "x"); err == nil {
		t.Fatal("thiếu token phải lỗi")
	}
	if err := SendTelegram("t", "", "x"); err == nil {
		t.Fatal("thiếu chat ID phải lỗi")
	}
	if err := SendTelegram("t", "1", "  "); err == nil {
		t.Fatal("tin trống phải lỗi")
	}
}

package gmodels

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPickPrefersStableNewest(t *testing.T) {
	names := []string{"gemini-2.0-flash", "gemini-3.5-flash-preview", "gemini-3.5-flash",
		"gemini-3.5-flash-lite", "gemini-3.5-flash-tts", "gemini-3.5-flash-image", "text-embedding-004"}
	if got := Pick(names, Text); got != "gemini-3.5-flash" {
		t.Fatalf("text = %q", got)
	}
	if got := Pick(names, Image); got != "gemini-3.5-flash-image" {
		t.Fatalf("image = %q", got)
	}
	if got := Pick(names, TTS); got != "gemini-3.5-flash-tts" {
		t.Fatalf("tts = %q", got)
	}
	if got := Pick([]string{"gemini-9-flash", Default(Text)}, Text); got != Default(Text) {
		t.Fatalf("default còn sống phải được giữ, got %q", got)
	}
	if got := Pick([]string{"imagen-4.0-generate"}, Image); got != "" {
		t.Fatalf("imagen dùng predict, không chọn: %q", got)
	}
}

func TestResolveOverrideAndRediscover(t *testing.T) {
	Reset()
	t.Setenv("GEMINI_TEXT_MODEL", "")
	if Resolve(Text) != Default(Text) {
		t.Fatal("chưa dò thì dùng mặc định")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-4.0-flash","supportedGenerationMethods":["generateContent"]},
		{"name":"models/gemini-4.0-flash-tts","supportedGenerationMethods":["generateContent"]}]}`))
	}))
	defer srv.Close()
	got, err := Rediscover(context.Background(), srv.Client(), srv.URL, "k", Text)
	if err != nil || got != "gemini-4.0-flash" || Resolve(Text) != "gemini-4.0-flash" {
		t.Fatalf("rediscover = %q, %v (resolve %q)", got, err, Resolve(Text))
	}
	if _, err := Rediscover(context.Background(), srv.Client(), srv.URL, "k", Text); err == nil {
		t.Fatal("model hiện tại còn sống → phải báo lỗi để không thử lại vô ích")
	}
	t.Setenv("GEMINI_TEXT_MODEL", "models/gemini-custom")
	if Resolve(Text) != "gemini-custom" {
		t.Fatalf("ghi đè thủ công phải thắng: %q", Resolve(Text))
	}
	Reset()
}

func TestIsModelGone(t *testing.T) {
	for _, s := range []string{
		"HTTP 404: {\"error\":{\"status\":\"NOT_FOUND\"}}",
		"gemini: api error: models/gemini-2.0-flash is not found for API version v1beta",
	} {
		if !IsModelGone(errors.New(s)) {
			t.Fatalf("phải nhận ra model chết: %s", s)
		}
	}
	for _, s := range []string{"HTTP 429: RESOURCE_EXHAUSTED", "HTTP 400: API key not valid"} {
		if IsModelGone(errors.New(s)) {
			t.Fatalf("không phải lỗi model: %s", s)
		}
	}
}

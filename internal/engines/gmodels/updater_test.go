package gmodels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPickLatest(t *testing.T) {
	names := []string{
		"gemini-2.0-flash",
		"gemini-3.5-flash-preview",
		"gemini-3.5-flash",
		"gemini-3.5-flash-lite",
		"gemini-3.8-flash",
		"gemini-3.9-flash",
		"gemini-3.8-flash-tts",
		"gemini-3.9-flash-tts",
		"gemini-2.0-flash-preview-image-generation",
		"gemini-3.0-flash-preview-image-generation",
		"gemini-nano-banana-2.1",
		"veo-2.0-generate-001",
		"veo-3.1-generate-preview",
		"text-embedding-004",
	}

	// 1. PickLatest Text: gemini-3.9-flash should beat gemini-3.8-flash and 3.5
	if got := PickLatest(names, Text); got != "gemini-3.9-flash" {
		t.Fatalf("Text latest = %q, want gemini-3.9-flash", got)
	}

	// 2. PickLatest TTS: gemini-3.9-flash-tts should beat gemini-3.8-flash-tts
	if got := PickLatest(names, TTS); got != "gemini-3.9-flash-tts" {
		t.Fatalf("TTS latest = %q, want gemini-3.9-flash-tts", got)
	}

	// 3. PickLatest Image: gemini-nano-banana-2.1 should beat legacy models
	if got := PickLatest(names, Image); got != "gemini-nano-banana-2.1" {
		t.Fatalf("Image latest = %q, want gemini-nano-banana-2.1", got)
	}

	// 4. PickLatest Video: veo-3.1-generate-preview should beat veo-2.0
	if got := PickLatest(names, Video); got != "veo-3.1-generate-preview" {
		t.Fatalf("Video latest = %q, want veo-3.1-generate-preview", got)
	}

	// 5. Stable beats preview of the same version
	namesTie := []string{"gemini-4.0-flash-preview", "gemini-4.0-flash"}
	if got := PickLatest(namesTie, Text); got != "gemini-4.0-flash" {
		t.Fatalf("Stable should beat preview: got %q, want gemini-4.0-flash", got)
	}

	// 6. Standard beats Lite of the same version
	namesLite := []string{"gemini-4.0-flash-lite", "gemini-4.0-flash"}
	if got := PickLatest(namesLite, Text); got != "gemini-4.0-flash" {
		t.Fatalf("Standard should beat lite: got %q, want gemini-4.0-flash", got)
	}

	// 7. Major version jump (e.g. 4.0-preview beats 3.8-stable)
	namesMajor := []string{"gemini-3.8-flash", "gemini-4.0-flash-preview"}
	if got := PickLatest(namesMajor, Text); got != "gemini-4.0-flash-preview" {
		t.Fatalf("Major version jump should be picked: got %q, want gemini-4.0-flash-preview", got)
	}
}

func TestCheckAndUpdate(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-3.9-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-3.9-flash-tts", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-nano-banana-2.1", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/veo-3.1-generate-preview", "supportedGenerationMethods": ["generateContent"]}
			]
		}`))
	}))
	defer srv.Close()

	tmpDir, err := os.MkdirTemp("", "gmodels_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rep, err := CheckAndUpdate(context.Background(), srv.Client(), srv.URL, []string{"fake-key"}, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndUpdate failed: %v", err)
	}
	if !rep.Success || !rep.Updated {
		t.Fatalf("expected update to succeed, rep: %+v", rep)
	}

	if got := Resolve(Text); got != "gemini-3.9-flash" {
		t.Fatalf("Resolve(Text) = %q, want gemini-3.9-flash", got)
	}
	if got := Resolve(TTS); got != "gemini-3.9-flash-tts" {
		t.Fatalf("Resolve(TTS) = %q, want gemini-3.9-flash-tts", got)
	}
	if got := Resolve(Image); got != "gemini-nano-banana-2.1" {
		t.Fatalf("Resolve(Image) = %q, want gemini-nano-banana-2.1", got)
	}
	if got := Resolve(Video); got != "veo-3.1-generate-preview" {
		t.Fatalf("Resolve(Video) = %q, want veo-3.1-generate-preview", got)
	}

	// Verify persistence file was written
	persistedPath := filepath.Join(tmpDir, persistedFile)
	if _, err := os.Stat(persistedPath); err != nil {
		t.Fatalf("persisted file not created: %v", err)
	}

	// Reset in-memory cache and test LoadPersisted
	Reset()
	if Resolve(Text) != Default(Text) {
		t.Fatal("after reset, should be default")
	}
	if err := LoadPersisted(tmpDir); err != nil {
		t.Fatalf("LoadPersisted failed: %v", err)
	}
	if got := Resolve(Text); got != "gemini-3.9-flash" {
		t.Fatalf("after LoadPersisted, Resolve(Text) = %q, want gemini-3.9-flash", got)
	}

	// Verify LastReport
	last := LastReport()
	if last == nil || !last.Success || last.TotalModels != 4 {
		t.Fatalf("LastReport() invalid: %+v", last)
	}
}

func TestStartDailyUpdater(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	called := make(chan struct{}, 5)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case called <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-4.0-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-4.0-flash-tts", "supportedGenerationMethods": ["generateContent"]}
			]
		}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	SetInitialDelay(10 * time.Millisecond)
	SetUpdateInterval(30 * time.Millisecond)
	SetAPIBaseURL(srv.URL)
	SetHTTPClient(srv.Client())
	t.Cleanup(func() {
		SetInitialDelay(15 * time.Second)
		SetUpdateInterval(24 * time.Hour)
		SetAPIBaseURL("https://generativelanguage.googleapis.com")
		SetHTTPClient(nil)
	})

	go StartDailyUpdater(ctx, func() []string { return []string{"test-key"} }, "")

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("daily updater did not trigger in time")
	}

	// Verify model updated
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if Resolve(Text) == "gemini-4.0-flash" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := Resolve(Text); got != "gemini-4.0-flash" {
		t.Fatalf("Resolve(Text) = %q, want gemini-4.0-flash", got)
	}
	if got := Resolve(TTS); got != "gemini-4.0-flash-tts" {
		t.Fatalf("Resolve(TTS) = %q, want gemini-4.0-flash-tts", got)
	}
}

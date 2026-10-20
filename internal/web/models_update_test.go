package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
)

func TestSettingsModelsUpdateEndpoint(t *testing.T) {
	s := newTestServer(t)
	gmodels.Reset()
	t.Cleanup(gmodels.Reset)

	// Mock server for Gemini ListModels
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-4.0-flash", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-4.0-flash-tts", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/gemini-nano-banana-2.1", "supportedGenerationMethods": ["generateContent"]},
				{"name": "models/veo-3.1-generate-preview", "supportedGenerationMethods": ["generateContent"]}
			]
		}`))
	}))
	defer srv.Close()

	gmodels.SetAPIBaseURL(srv.URL)
	gmodels.SetHTTPClient(srv.Client())
	t.Cleanup(func() {
		gmodels.SetAPIBaseURL("https://generativelanguage.googleapis.com")
		gmodels.SetHTTPClient(nil)
	})

	// Add a test Gemini key
	s.Cfg.GeminiAPIKeys = []string{"AIzaSyTEST_key_1234567890"}

	// Trigger POST /settings/models/update
	rec := postForm(t, s, "/settings/models/update", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/models/update status = %d, want 303", rec.Code)
	}

	// Verify model updated
	if got := gmodels.Resolve(gmodels.Text); got != "gemini-4.0-flash" {
		t.Fatalf("Resolve(Text) = %q, want gemini-4.0-flash", got)
	}
	if got := gmodels.Resolve(gmodels.TTS); got != "gemini-4.0-flash-tts" {
		t.Fatalf("Resolve(TTS) = %q, want gemini-4.0-flash-tts", got)
	}
	if got := gmodels.Resolve(gmodels.Image); got != "gemini-nano-banana-2.1" {
		t.Fatalf("Resolve(Image) = %q, want gemini-nano-banana-2.1", got)
	}
	if got := gmodels.Resolve(gmodels.Video); got != "veo-3.1-generate-preview" {
		t.Fatalf("Resolve(Video) = %q, want veo-3.1-generate-preview", got)
	}

	// GET /settings/he-thong should render the updated models and card
	body := get(t, s, "/settings/he-thong").Body.String()
	if !strings.Contains(body, "gemini-4.0-flash") {
		t.Fatalf("GET /settings/he-thong missing updated model: %s", body)
	}
	if !strings.Contains(body, "Google Gemini AI Models — Cập Nhật Model Mới Nhất Cho Toàn Bộ Adapter") {
		t.Fatalf("GET /settings/he-thong missing update header: %s", body)
	}
}

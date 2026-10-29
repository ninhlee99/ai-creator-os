package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func newTestWebAIServer(t *testing.T) (*Server, *webai.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := webai.NewStore(filepath.Join(dir, "webai.db"))
	if err != nil {
		t.Fatalf("webai.NewStore: %v", err)
	}

	cfg := &Config{}
	cfg.dryRun.Store(true)

	srv := &Server{
		DataDir: dir,
		Cfg:     cfg,
		WebAI:   store,
	}
	if err := srv.parseTemplates(); err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	return srv, store
}

func TestStudioAIGenerateDoesNotRequireCreditsConsent(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()
	dir := t.TempDir()
	st, err := studio.New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv.Studio = st
	srv.WebAI = store
	form := url.Values{
		"provider": {webai.PlatformGoogleFlow},
		"kind":     {"video"},
		"prompt":   {"Tạo video dọc về một chiếc xe đạp"},
	}
	req := httptest.NewRequest(http.MethodPost, "/studio/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleStudioAIGenerate(rec, req)
	// Không yêu cầu consent credits nữa; không chuyển hướng đến lỗi credits
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "credits") {
		t.Fatalf("Flow request must not block on credits consent: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSettingsSystemRendersWebAILoginButtons(t *testing.T) {
	srv := newTestServer(t)
	store, err := webai.NewStore(filepath.Join(t.TempDir(), "webai.db"))
	if err != nil {
		t.Fatalf("webai.NewStore: %v", err)
	}
	defer store.Close()
	srv.WebAI = store
	req := httptest.NewRequest(http.MethodGet, "/settings/system", nil)
	rec := httptest.NewRecorder()
	srv.handleSettingsHeThong(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings status=%d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, marker := range []string{"Đăng nhập", "startBrowserLogin('museai'", "startBrowserLogin('googleflow'"} {
		if !strings.Contains(body, marker) {
			t.Errorf("settings missing Web AI login action %q", marker)
		}
	}
}

func TestAccountProbeDoesNotMarkMissingCredentialsActiveOrExpired(t *testing.T) {
	for _, platform := range []string{"unsupported"} {
		t.Run(platform, func(t *testing.T) {
			outcome, _ := probeAccountSessionOutcome(httptest.NewRequest(http.MethodGet, "/", nil).Context(), &webai.Account{Platform: platform}, nil)
			if outcome != accountProbeUnknown {
				t.Fatalf("probe outcome = %q, want unknown without credentials", outcome)
			}
		})
	}
}

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		Cfg:        cfg,
		WebAI:      store,
		WebAIRelay: webai.NewProxyRelay(store),
	}
	if err := srv.parseTemplates(); err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	return srv, store
}

func TestWebAIHandlers(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	// 1. GET /web-ai
	req := httptest.NewRequest("GET", "/web-ai", nil)
	w := httptest.NewRecorder()
	srv.handleWebAI(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /web-ai status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "AI Web Hub") || !strings.Contains(body, "ChatGPT Web") || !strings.Contains(body, "Claude Web") {
		t.Errorf("GET /web-ai missing expected platform texts: %s", body[:min(len(body), 500)])
	}

	// 2. POST /web-ai/connect
	form := url.Values{
		"platform":    {"chatgpt"},
		"account_name": {"Test User"},
		"email":       {"test@example.com"},
		"session_key": {"tok-123456789"},
		"tier":        {"plus"},
	}
	req = httptest.NewRequest("POST", "/web-ai/connect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleWebAIConnect(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /web-ai/connect status = %d, want 303", w.Code)
	}

	acc, err := store.GetAccount("chatgpt")
	if err != nil || acc.Status != webai.StatusConnected || acc.SessionKey != "tok-123456789" {
		t.Errorf("account not saved properly: %+v, err: %v", acc, err)
	}

	// 3. POST /web-ai/prompt-generate
	pForm := url.Values{
		"product_name": {"Áo Polo Cao Cấp"},
		"niche":        {"thời trang"},
		"framework":    {"problem_solution"},
	}
	req = httptest.NewRequest("POST", "/web-ai/prompt-generate", strings.NewReader(pForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleWebAIPromptGenerate(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("prompt-generate status = %d, want 200", w.Code)
	}
	var pkg webai.AdScriptPackage
	if err := json.Unmarshal(w.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if pkg.ProductName != "Áo Polo Cao Cấp" || len(pkg.Scenes) != 4 {
		t.Errorf("unexpected script package: %+v", pkg)
	}

	// 4. POST /web-ai/disconnect
	dForm := url.Values{"platform": {"chatgpt"}}
	req = httptest.NewRequest("POST", "/web-ai/disconnect", strings.NewReader(dForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleWebAIDisconnect(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /web-ai/disconnect status = %d, want 303", w.Code)
	}
	acc, _ = store.GetAccount("chatgpt")
	if acc.Status != webai.StatusDisconnected {
		t.Errorf("account status want disconnected, got %s", acc.Status)
	}
}

func TestStudioAdCreativeHandler(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	dir := t.TempDir()
	st, err := studio.New(filepath.Join(dir, "studio.db"), nil, nil, nil, dir)
	if err != nil {
		t.Fatalf("studio.New failed: %v", err)
	}
	defer func() {
		st.Shutdown(300 * time.Millisecond)
		_ = st.Close()
	}()
	srv.Studio = st

	form := url.Values{
		"product_name": {"Túi da công sở"},
		"niche":        {"thời trang"},
		"aspect_ratio": {"9:16"},
		"seconds":      {"30"},
		"bpm":          {"120"},
	}
	req := httptest.NewRequest("POST", "/studio/ad-creative", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleStudioAdCreative(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /studio/ad-creative status = %d, want 303", w.Code)
	}

	jobs := st.ListJobs(10)
	if len(jobs) == 0 {
		t.Errorf("expected job to be queued in studio")
	} else if jobs[0].Title != "Túi da công sở" {
		t.Errorf("expected job title 'Túi da công sở', got %s", jobs[0].Title)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

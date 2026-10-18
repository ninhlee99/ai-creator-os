package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
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
	if !strings.Contains(body, "AI Hub") || !strings.Contains(body, "ChatGPT") || !strings.Contains(body, "Claude") {
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
	if pkg.QC.OverallScore <= 0 || pkg.QC.HookScore <= 0 {
		t.Errorf("expected positive QC scorecard scores, got: %+v", pkg.QC)
	}
	if len(pkg.QC.Strengths) == 0 {
		t.Errorf("expected QC strengths, got none")
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

	// 5. GET /web-ai/pool-status
	req = httptest.NewRequest("GET", "/web-ai/pool-status", nil)
	w = httptest.NewRecorder()
	srv.handleWebAIPoolStatus(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /web-ai/pool-status status = %d, want 200", w.Code)
	}
	var poolStatus map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &poolStatus); err != nil {
		t.Fatalf("failed to decode pool-status JSON: %v", err)
	}
	if poolStatus["ok"] != true {
		t.Errorf("expected poolStatus ok = true, got: %v", poolStatus)
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

func TestBioPinAndRedirectClick(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	prodStore, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	defer prodStore.Close()
	srv.Products = prodStore

	// 1. Test POST /bio/pin
	form := url.Values{
		"title":       {"Serum Rau Má Trị Mụn"},
		"category":    {"Mỹ phẩm"},
		"price":       {"249000"},
		"product_url": {"https://shopee.vn/serum-rauma"},
	}
	req := httptest.NewRequest("POST", "/bio/pin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleBioPin(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /bio/pin status = %d, want 200", w.Code)
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if res["ok"] != true {
		t.Fatalf("expected ok = true, got %v", res)
	}
	pid := int64(res["id"].(float64))

	// Verify product was pinned to shelf
	p, err := prodStore.GetByID(pid)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if p.ShelfStatus != "shelf" || p.Title != "Serum Rau Má Trị Mụn" {
		t.Fatalf("unexpected pinned product: %+v", p)
	}
	if p.ClickCount != 0 {
		t.Errorf("initial ClickCount = %d, want 0", p.ClickCount)
	}

	// 2. Test GET /bio-go/{id} records click and redirects
	req = httptest.NewRequest("GET", fmt.Sprintf("/bio-go/%d", pid), nil)
	req.SetPathValue("id", fmt.Sprintf("%d", pid))
	w = httptest.NewRecorder()
	srv.handleBioRedirect(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("GET /bio-go/{id} status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://shopee.vn/serum-rauma" {
		t.Errorf("redirect Location = %s, want https://shopee.vn/serum-rauma", loc)
	}

	// Verify click count incremented
	pAfter, err := prodStore.GetByID(pid)
	if err != nil {
		t.Fatalf("GetByID after click: %v", err)
	}
	if pAfter.ClickCount != 1 {
		t.Errorf("expected ClickCount = 1, got %d", pAfter.ClickCount)
	}
}


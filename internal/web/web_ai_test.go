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

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
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
		DataDir: dir,
		Cfg:     cfg,
		WebAI:   store,
	}
	if err := srv.parseTemplates(); err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	return srv, store
}

func TestStudioAIGenerateRequiresFlowCreditsConsent(t *testing.T) {
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
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "credits") {
		t.Fatalf("Flow request without consent status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if jobs := st.ListJobs(10); len(jobs) != 0 {
		t.Fatalf("Flow request without consent queued %d jobs", len(jobs))
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

func TestBioPinAndRedirectClick(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	prodStore, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	defer prodStore.Close()
	srv.Products = prodStore
	l, err := ledger.New(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	srv.Ledger = l
	accountID, err := l.AddAccount("bio_creator", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Test POST /bio/pin
	form := url.Values{
		"title":       {"Serum Rau Má Trị Mụn"},
		"category":    {"Mỹ phẩm"},
		"price":       {"249000"},
		"product_url": {"https://shopee.vn/serum-rauma"},
		"image_url":   {"https://img.shopee.vn/file/serum-rauma.jpg"},
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
	if err := prodStore.SetBioProfile(accountID, true, "", true); err != nil {
		t.Fatal(err)
	}
	if err := prodStore.SetBioListing(accountID, pid, true, 0, "", 0, "", ""); err != nil {
		t.Fatal(err)
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

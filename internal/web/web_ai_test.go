package web

import (
	"encoding/json"
	"fmt"
	"io"
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
		"platform":     {"chatgpt"},
		"account_name": {"Test User"},
		"email":        {"test@example.com"},
		"session_key":  {"__Secure-1PSID=fake; SID=not-a-session"},
		"tier":         {"plus"},
	}
	req = httptest.NewRequest("POST", "/web-ai/connect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleWebAIConnect(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /web-ai/connect status = %d, want 303", w.Code)
	}
	if accounts, err := store.GetAccounts(webai.PlatformChatGPT); err != nil || len(accounts) != 0 {
		t.Fatalf("unverified manual cookie must not be stored as an active session: accounts=%+v err=%v", accounts, err)
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
	_, err := store.AddAccount(webai.Account{
		Platform: webai.PlatformChatGPT, AccountName: "Test User", Email: "test@example.com",
		SessionKey: "verified-session-fixture", Cookie: "session=fixture", Status: webai.StatusConnected,
	})
	if err != nil {
		t.Fatalf("add disconnect fixture: %v", err)
	}
	dForm := url.Values{"platform": {"chatgpt"}}
	req = httptest.NewRequest("POST", "/web-ai/disconnect", strings.NewReader(dForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.handleWebAIDisconnect(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /web-ai/disconnect status = %d, want 303", w.Code)
	}
	acc, err := store.GetAccount("chatgpt")
	if err != nil {
		t.Fatalf("read disconnected account: %v", err)
	}
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

func TestWebAICaptureSessionRejectsUnverifiedCookie(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	request := httptest.NewRequest(http.MethodPost, "/api/web-ai/capture-session", strings.NewReader(`{"platform":"googleflow","cookie":"__Secure-1PSID=fake","email":"creator@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.handleWebAICaptureSession(response, request)
	if response.Code != http.StatusGone {
		t.Fatalf("unverified session status=%d, want %d", response.Code, http.StatusGone)
	}
	if accounts, err := store.GetAccounts(webai.PlatformGoogleFlow); err != nil || len(accounts) != 0 {
		t.Fatalf("unverified cookie must not be stored: accounts=%+v err=%v", accounts, err)
	}
}

func TestSyncGoogleFlowRequiresSelectedEmailForMultipleAccounts(t *testing.T) {
	srv, store := newTestWebAIServer(t)
	defer store.Close()

	firstID, err := store.AddAccount(webai.Account{
		Platform: webai.PlatformGemini, AccountName: "Google one", Email: "one@gmail.com",
		SessionKey: "session-one", Cookie: "__Secure-1PSID=one", Status: webai.StatusConnected,
	})
	if err != nil {
		t.Fatalf("add first Gemini account: %v", err)
	}
	secondID, err := store.AddAccount(webai.Account{
		Platform: webai.PlatformGemini, AccountName: "Google two", Email: "two@gmail.com",
		SessionKey: "session-two", Cookie: "__Secure-1PSID=two", Status: webai.StatusConnected,
	})
	if err != nil {
		t.Fatalf("add second Gemini account: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/web-ai/accounts/sync-google", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.handleWebAISyncGoogleAccount(response, request)
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["ok"] != false || !strings.Contains(result["error"].(string), "Chọn email") {
		t.Fatalf("multiple accounts must require explicit selection, got %v", result)
	}
	if accounts, _ := store.GetAccounts(webai.PlatformGoogleFlow); len(accounts) != 0 {
		t.Fatalf("ambiguous sync created Flow accounts: %+v", accounts)
	}

	actualEmail := "other@gmail.com"
	originalTransport := http.DefaultTransport
	http.DefaultTransport = webAITestRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{"userWorkflows":[]}`
		if request.URL.Hostname() == "myaccount.google.com" {
			body = `<html><script>window.email="` + actualEmail + `"</script></html>`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	request = httptest.NewRequest(http.MethodPost, "/web-ai/accounts/sync-google", strings.NewReader(`{"account_id":"`+secondID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	srv.handleWebAISyncGoogleAccount(response, request)
	result = nil
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode selected response: %v", err)
	}
	if result["ok"] != false || !strings.Contains(strings.ToLower(result["error"].(string)), "không khớp email") {
		t.Fatalf("Flow session with mismatched identity must fail: %v", result)
	}
	if accounts, _ := store.GetAccounts(webai.PlatformGoogleFlow); len(accounts) != 0 {
		t.Fatalf("identity mismatch created Flow account: %+v", accounts)
	}

	actualEmail = "two@gmail.com"
	request = httptest.NewRequest(http.MethodPost, "/web-ai/accounts/sync-google", strings.NewReader(`{"account_id":"`+secondID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	srv.handleWebAISyncGoogleAccount(response, request)
	result = nil
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode matching Flow response: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("matching active Flow session should sync: %v", result)
	}
	flow, err := store.GetAccount(webai.PlatformGoogleFlow)
	if err != nil || flow.Email != "two@gmail.com" || flow.Cookie != "__Secure-1PSID=two" {
		t.Fatalf("Flow saved wrong identity or cookie: flow=%+v err=%v", flow, err)
	}
	if firstID == secondID {
		t.Fatal("test fixture accounts should have distinct IDs")
	}
}

type webAITestRoundTripper func(*http.Request) (*http.Response, error)

func (f webAITestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
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
	if len(jobs) != 0 {
		t.Errorf("legacy endpoint queued %d jobs without confirmed Flow credits and prepared shot plan", len(jobs))
	}
	if !strings.Contains(w.Header().Get("Location"), "err=") {
		t.Errorf("legacy endpoint should redirect with its missing-input reason, location=%q", w.Header().Get("Location"))
	}
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

func TestAccountProbeDoesNotMarkMissingCredentialsActiveOrExpired(t *testing.T) {
	for _, platform := range []string{"chatgpt", "gemini", "claude"} {
		t.Run(platform, func(t *testing.T) {
			outcome, _ := probeAccountSessionOutcome(httptest.NewRequest(http.MethodGet, "/", nil).Context(), &webai.Account{Platform: platform})
			if outcome != accountProbeUnknown {
				t.Fatalf("probe outcome = %q, want unknown without credentials", outcome)
			}
		})
	}
}

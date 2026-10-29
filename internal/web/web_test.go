package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir) // token files, data/output, jobs all stay in the temp dir
	dbPath := filepath.Join(dir, "ledger.db")

	l, err := ledger.New(dbPath)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	mgr, err := network.NewAccountManager(l, dbPath)
	if err != nil {
		t.Fatalf("NewAccountManager: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	cfg := LoadConfig()
	cfg.DatabasePath = dbPath

	s, err := NewServer(cfg, l, mgr, dbPath)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func postForm(t *testing.T, s *Server, path string, v url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)
	return rec
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func serveLocalTestRoute(s *Server, w http.ResponseWriter, r *http.Request) {
	r.RemoteAddr = "127.0.0.1:12345"
	s.Routes().ServeHTTP(w, r)
}

// (1) GET / renders the dashboard.
func TestDashboard(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET / = %d, want 303 See Other", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/workflow/create" {
		t.Fatalf("GET / redirect location = %q, want /workflow/create", loc)
	}
}

// (2) POST /accounts creates an account and redirects with 303.
func TestAccountCreate(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/accounts", url.Values{
		"username":   {"@test_creator"},
		"niche_hint": {"review đồ gia dụng"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /accounts = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/accounts/") {
		t.Fatalf("redirect location = %q, want /accounts/<id>", loc)
	}
	accts, err := s.Mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(accts) != 1 || accts[0].Username != "test_creator" {
		t.Fatalf("accounts = %+v, want one test_creator", accts)
	}
	// detail page renders for the new account
	if rec := get(t, s, loc); rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", loc, rec.Code)
	}
}

// Đợt G: trang Kênh không còn copy pre-pivot về live (live đã park).
func TestAccountsPageNoLiveCopy(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/accounts")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accounts = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, bad := range []string{"đủ điều kiện live", "đủ live", "1000 followers"} {
		if strings.Contains(body, bad) {
			t.Fatalf("trang Kênh còn copy live pre-pivot: %q", bad)
		}
	}
	if !strings.Contains(body, "Kênh") || !strings.Contains(body, "+ Thêm kênh") {
		t.Fatalf("trang Kênh thiếu tiêu đề hoặc lối tạo kênh")
	}
}

func TestDryRunToggle(t *testing.T) {
	s := newTestServer(t)

	rec := postForm(t, s, "/settings/dryrun", url.Values{"value": {"off"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("dryrun off = %d, want 303", rec.Code)
	}
	if s.Cfg.DryRun() {
		t.Fatal("dry_run should be false after value=off")
	}

	rec = postForm(t, s, "/settings/dryrun", url.Values{"value": {"on"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("dryrun on = %d, want 303", rec.Code)
	}
	if !s.Cfg.DryRun() {
		t.Fatal("dry_run should be true after value=on (bugfix: dry_run = (value == \"on\"))")
	}
}

// (4) POST /settings/chain saves JSON; GET /settings/chain reads it back
// with API keys MASKED (never raw).
func TestChainSaveAndGet(t *testing.T) {
	s := newTestServer(t)
	form := url.Values{
		"pname":   {"gemini", "edge"},
		"enabled": {"0"}, // only gemini checked
		"apikey":  {"k1", ""},
		"timeout": {"60", "30"},
		"retries": {"1", "0"},
	}
	rec := postForm(t, s, "/settings/chain?name=tts", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/chain = %d, want 303", rec.Code)
	}

	rec = get(t, s, "/settings/chain?name=tts")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/chain = %d, want 200", rec.Code)
	}
	var got ChainConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode chain json: %v", err)
	}
	// "k1" is a short test key -> fully masked as "••••"; the raw key must
	// never appear in the JSON API.
	want := ChainConfig{Order: []ProviderEntry{
		{Name: "gemini", Enabled: true, APIKeys: []string{"••••"}, TimeoutSec: 60, Retries: 1},
		{Name: "edge", Enabled: false, TimeoutSec: 30, Retries: 0},
	}}
	if len(got.Order) != len(want.Order) {
		t.Fatalf("order = %+v, want %+v", got.Order, want.Order)
	}
	for i := range want.Order {
		if got.Order[i].Name != want.Order[i].Name ||
			got.Order[i].Enabled != want.Order[i].Enabled ||
			got.Order[i].TimeoutSec != want.Order[i].TimeoutSec ||
			got.Order[i].Retries != want.Order[i].Retries ||
			got.Order[i].APIKey != "" ||
			len(got.Order[i].APIKeys) != len(want.Order[i].APIKeys) {
			t.Fatalf("order[%d] = %+v, want %+v", i, got.Order[i], want.Order[i])
		}
		for j := range want.Order[i].APIKeys {
			if got.Order[i].APIKeys[j] != want.Order[i].APIKeys[j] {
				t.Fatalf("order[%d].APIKeys[%d] = %q, want %q", i, j, got.Order[i].APIKeys[j], want.Order[i].APIKeys[j])
			}
		}
	}
	if strings.Contains(rec.Body.String(), "k1") {
		t.Fatalf("GET /settings/chain leaked the raw key: %s", rec.Body.String())
	}

	// move gemini down: order becomes edge, gemini
	rec = postForm(t, s, "/settings/chain/move?name=tts&dir=down&i=0", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/chain/move = %d, want 303", rec.Code)
	}
	rec = get(t, s, "/settings/chain?name=tts")
	var moved ChainConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &moved); err != nil {
		t.Fatal(err)
	}
	if len(moved.Order) != 2 || moved.Order[0].Name != "edge" || moved.Order[1].Name != "gemini" {
		t.Fatalf("after move: %+v", moved.Order)
	}

	// unknown chain -> 400
	if rec := get(t, s, "/settings/chain?name=nope"); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown chain = %d, want 400", rec.Code)
	}
}

// (5) Path traversal / bad names on /media are rejected. Note: the stdlib
// mux canonicalizes a literal ".." with a 307 redirect to the cleaned path
// (which then 404s) before the handler ever runs — the file is never served.
func TestMediaValidation(t *testing.T) {
	s := newTestServer(t)
	for _, p := range []string{"/media/evil.exe", "/media/nope.mp4", "/media/..%2Fx.mp4"} {
		if rec := get(t, s, p); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	rec := get(t, s, "/media/../x.mp4")
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("GET /media/../x.mp4 = %d, want 404 or 307", rec.Code)
	}
}

// (6) Every GET page renders without error against sample data.
func TestAllPagesRender(t *testing.T) {
	s := newTestServer(t)
	_, err := s.Mgr.Add("sample_acct", "hint", "")
	if err != nil {
		t.Fatal(err)
	}
	// seed a little data so branches render: product + slot + decision
	if _, err := s.Ledger.UpsertProduct("TT-1", map[string]any{
		"title": "Tai nghe X10", "price": 25.0, "commission_rate": 0.12,
		"commission_value": 3.0, "status": "shelf", "score": 0.9,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Ledger.Decide("test", "seed", nil, "seed decision", map[string]any{}); err != nil {
		t.Fatal(err)
	}

	pages := map[string]string{
		"/dashboard":                  "MiuNova",
		"/accounts":                   "Kênh",
		"/accounts/new":               "kênh",
		"/accounts/1":                 "sample_acct",
		"/accounts/1?tab=overview":    "sample_acct",
		"/accounts/1?tab=autopilot":   "Autopilot",
		"/accounts/1?tab=connections": "sample_acct",
		"/studio":                     "Tạo video affiliate",
		"/studio/jobs":                "Video",
		"/studio/trends":              "Radar",
		"/workflow":                   "Workflow",
		"/products":                   "Sản phẩm",
		"/products?tab=shelf":         "Sản phẩm",
		"/publishers":                 "TikTok",
		"/settings/system":            "Hệ thống",
		"/settings/providers":         "giọng đọc",
		"/settings/safety":            "Kill switch",
		"/settings/chain?name=tts":    `"name":"gemini"`,
	}
	for path, marker := range pages {
		rec := get(t, s, path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
			continue
		}
		if body := rec.Body.String(); !strings.Contains(body, marker) {
			t.Errorf("GET %s: body missing %q", path, marker)
		}
	}

	// / redirects 303 to /workflow.
	if rec := get(t, s, "/"); rec.Code != http.StatusSeeOther {
		t.Errorf("GET / = %d, want 303", rec.Code)
	}

	// /settings redirects 303 to the first sub-page (system).
	if rec := get(t, s, "/settings"); rec.Code != http.StatusSeeOther {
		t.Errorf("GET /settings = %d, want 303", rec.Code)
	}

	// each settings sub-page renders inside base with the shared sub-nav
	// and its own concern section (R2-W3)
	body := get(t, s, "/settings/providers").Body.String()
	for _, marker := range []string{"Thứ tự nhà cung cấp viết nội dung (LLM)", "Kiểm tra kết nối", "Hệ thống</a>"} {
		if !strings.Contains(body, marker) {
			t.Errorf("providers page missing %q", marker)
		}
	}
	body = get(t, s, "/settings/system").Body.String()
	for _, marker := range []string{"Chi phí API theo bộ máy/nhà cung cấp", "Mã &amp; biến cấu hình"} {
		if !strings.Contains(body, marker) {
			t.Errorf("system page missing %q", marker)
		}
	}

	// studio create page links out to the split pages instead of
	// rendering the job list inline
	body = get(t, s, "/studio").Body.String()
	for _, marker := range []string{`href="/studio/jobs"`, `href="/studio/trends"`, "JOB_LABELS"} {
		present := strings.Contains(body, marker)
		if marker == "JOB_LABELS" && present {
			t.Errorf("/studio still inlines JOB_LABELS (should live only in app.js)")
		}
		if marker != "JOB_LABELS" && !present {
			t.Errorf("/studio missing %q", marker)
		}
	}

	// JSON APIs — gated behind the api.enabled switch (R2-W7, default OFF):
	// enable like an operator would, then they must render.
	if err := automation.SetAPIEnabled(s.settings(), true); err != nil {
		t.Fatalf("enable api: %v", err)
	}
	for _, path := range []string{"/api/stats", "/api/products", "/api/decisions"} {
		if rec := get(t, s, path); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}

	// unknown account -> 404, unknown path -> 404
	if rec := get(t, s, "/accounts/999999"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /accounts/999999 = %d, want 404", rec.Code)
	}
	if rec := get(t, s, "/no-such-page"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /no-such-page = %d, want 404", rec.Code)
	}

	// static css + js served
	if rec := get(t, s, "/static/style.css"); rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/css") {
		t.Errorf("GET /static/style.css = %d (%s)", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := get(t, s, "/static/app.js"); rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("GET /static/app.js = %d (%s)", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// R2-W3/R2-13: account POST errors redirect back to the ket-noi tab with
// ?err= (surfaced as a toast) instead of a plain silent redirect.
func TestAccountPostErrorsCarryTabAndErr(t *testing.T) {
	s := newTestServer(t)
	acct, err := s.Mgr.Add("err_acct", "hint", "")
	if err != nil {
		t.Fatal(err)
	}
	// invalid transition target -> backend rejects -> ?err=
	rec := postForm(t, s, "/accounts/"+strconv.FormatInt(acct.ID, 10)+"/transition",
		url.Values{"to": {"bogus_state"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST transition = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "tab=") || !strings.Contains(loc, "err=") {
		t.Errorf("transition Location = %q, want tab= and err=", loc)
	}
	// valid transition stays silent (no ?err=) but keeps the tab
	rec = postForm(t, s, "/accounts/"+strconv.FormatInt(acct.ID, 10)+"/transition",
		url.Values{"to": {"researching"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST transition = %d, want 303", rec.Code)
	}
	loc = rec.Header().Get("Location")
	if !strings.Contains(loc, "/accounts/"+strconv.FormatInt(acct.ID, 10)+"?tab=") {
		t.Errorf("transition Location = %q, want tab=", loc)
	}
	if strings.Contains(loc, "err=") {
		t.Errorf("transition Location = %q, unexpected err= on success", loc)
	}
}

// Đợt 2 (gộp trang): /content và /shop redirect 303 sang tab thay thế,
// /analytics tan hẳn (404), và dữ liệu cũ (job kinetic, kệ hàng) vẫn xem
// và tạo mới được ở nhà mới.
func TestMergedPagesRedirects(t *testing.T) {
	s := newTestServer(t)
	// R2-W1: the shelf now lives in the shared products store, so the
	// test server gets one (like the real cmd wiring does).
	store, err := products.NewStore("products-webtest.db")
	if err != nil {
		t.Fatalf("products store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	// legacy URLs -> 303 to the tab that replaced them
	for _, tc := range []struct{ method, path, wantLoc string }{
		{http.MethodGet, "/content", "/studio"},
		{http.MethodPost, "/content", "/studio"},
		{http.MethodGet, "/shop", "/products?tab=shelf"},
		{http.MethodPost, "/shop/add", "/products?tab=shelf"},
	} {
		var rec *httptest.ResponseRecorder
		if tc.method == http.MethodPost {
			rec = postForm(t, s, tc.path, url.Values{})
		} else {
			rec = get(t, s, tc.path)
		}
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s %s = %d, want 303", tc.method, tc.path, rec.Code)
			continue
		}
		if loc := rec.Header().Get("Location"); loc != tc.wantLoc {
			t.Errorf("%s %s Location = %q, want %q", tc.method, tc.path, loc, tc.wantLoc)
		}
	}

	// /analytics dissolved -> 404
	if rec := get(t, s, "/analytics"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /analytics = %d, want 404", rec.Code)
	}

	// sidebar no longer links to the three dissolved pages
	home := get(t, s, "/workflow").Body.String()
	for _, dead := range []string{`href="/content"`, `href="/shop"`, `href="/analytics"`} {
		if strings.Contains(home, dead) {
			t.Errorf("sidebar still links %s", dead)
		}
	}

	// shelf add from the products tab lands on the shelf and renders there
	rec := postForm(t, s, "/products/shelf/add", url.Values{
		"platform_pid": {"TT-KE-1"}, "title": {"Kệ test"}, "price": {"10"}, "commission_pct": {"20"},
		"product_url": {"https://shopee.vn/product/123"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/products?tab=shelf") {
		t.Fatalf("POST /products/shelf/add = %d (%q), want 303 /products?tab=shelf", rec.Code, rec.Header().Get("Location"))
	}
	if body := get(t, s, "/products?tab=shelf").Body.String(); !strings.Contains(body, "Kệ test") && !strings.Contains(body, "Sản phẩm") {
		t.Errorf("products shelf tab does not show the added product")
	}
}

// Kill switch toggle round-trips through the config.
func TestKillSwitch(t *testing.T) {
	s := newTestServer(t)
	if rec := postForm(t, s, "/kill", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /kill = %d, want 303", rec.Code)
	}
	if !s.Cfg.KillSwitch() {
		t.Fatal("kill switch should be engaged")
	}
	if rec := postForm(t, s, "/unkill", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /unkill = %d, want 303", rec.Code)
	}
	if s.Cfg.KillSwitch() {
		t.Fatal("kill switch should be released")
	}
}

// (8) Keyring endpoints: add / duplicate / status / delete, all masked.
func TestKeyAddDeleteStatus(t *testing.T) {
	s := newTestServer(t)
	const key1 = "AIzaxxxxxxxxxxxxxxxxtest-key-ABCDEF1234"
	const key2 = "AIzaxxxxxxxxxxxxxxxxtest-key-ZZZZZZ5678"

	add := func(key string) map[string]any {
		t.Helper()
		rec := postForm(t, s, "/settings/keys/add?chain=llm&provider=gemini",
			url.Values{"key": {key}})
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /settings/keys/add = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		var j map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
			t.Fatal(err)
		}
		return j
	}

	j := add(key1)
	if j["ok"] != true || j["duplicate"] != false {
		t.Fatalf("add key1 = %v, want ok+not duplicate", j)
	}
	if j["masked"] != "••••••••1234" {
		t.Fatalf("add key1 masked = %v, want ••••••••1234", j["masked"])
	}
	if j["total"] != float64(1) {
		t.Fatalf("add key1 total = %v, want 1", j["total"])
	}

	// duplicate add: no second copy
	j = add(key1)
	if j["duplicate"] != true || j["total"] != float64(1) {
		t.Fatalf("duplicate add = %v, want duplicate+total 1", j)
	}

	j = add(key2)
	if j["total"] != float64(2) {
		t.Fatalf("add key2 total = %v, want 2", j["total"])
	}

	// status: masked, render-ready, no raw keys anywhere
	rec := get(t, s, "/settings/keys/status?chain=llm&provider=gemini")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/keys/status = %d, want 200", rec.Code)
	}
	var st struct {
		Ok   bool        `json:"ok"`
		Keys []KeyStatus `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Ok || len(st.Keys) != 2 {
		t.Fatalf("status = %+v, want 2 keys", st)
	}
	if st.Keys[0].Masked != "••••••••1234" || st.Keys[1].Masked != "••••••••5678" {
		t.Fatalf("status masked = %q %q", st.Keys[0].Masked, st.Keys[1].Masked)
	}
	if st.Keys[0].BadgeClass != "badge-ok" || st.Keys[0].StateLabel != "Hoạt động" {
		t.Fatalf("status[0] badge = %q label = %q", st.Keys[0].BadgeClass, st.Keys[0].StateLabel)
	}
	if body := rec.Body.String(); strings.Contains(body, key1) || strings.Contains(body, key2) {
		t.Fatalf("status JSON leaked a raw key: %s", body)
	}

	// delete first key
	rec = postForm(t, s, "/settings/keys/delete?chain=llm&provider=gemini",
		url.Values{"idx": {"0"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /settings/keys/delete = %d, want 200", rec.Code)
	}
	var del map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &del); err != nil {
		t.Fatal(err)
	}
	if del["ok"] != true || del["total"] != float64(1) || del["masked"] != "••••••••1234" {
		t.Fatalf("delete = %v, want ok+total 1+masked", del)
	}
	if body := rec.Body.String(); strings.Contains(body, key1) {
		t.Fatalf("delete response leaked a raw key: %s", body)
	}

	// delete out of range -> 400
	if rec := postForm(t, s, "/settings/keys/delete?chain=llm&provider=gemini",
		url.Values{"idx": {"7"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete idx=7 = %d, want 400", rec.Code)
	}

	// empty key -> 400
	if rec := postForm(t, s, "/settings/keys/add?chain=llm&provider=gemini",
		url.Values{"key": {"  "}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("add empty key = %d, want 400", rec.Code)
	}

	// unknown chain -> 400
	if rec := postForm(t, s, "/settings/keys/add?chain=nope&provider=gemini",
		url.Values{"key": {"x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("add unknown chain = %d, want 400", rec.Code)
	}

	// test endpoint without a wired adapter -> 501, never leaks the key
	rec = postForm(t, s, "/settings/keys/test?chain=llm&provider=gemini",
		url.Values{"idx": {"0"}})
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("POST /settings/keys/test = %d, want 501", rec.Code)
	}
}

// (9) The main chain form preserves keyring-managed keys: the new template
// posts an empty apikey placeholder for gemini rows, which must not wipe
// the stored keys.
func TestChainSavePreservesGeminiKeys(t *testing.T) {
	s := newTestServer(t)
	const key = "AIzaxxxxxxxxxxxxxxxxpreserve-me-KEY9999"
	if rec := postForm(t, s, "/settings/keys/add?chain=tts&provider=gemini",
		url.Values{"key": {key}}); rec.Code != http.StatusOK {
		t.Fatalf("add = %d, want 200", rec.Code)
	}
	// what the new settings form posts for a gemini row: empty apikey
	form := url.Values{
		"pname":   {"gemini", "edge"},
		"enabled": {"0", "1"},
		"apikey":  {"", ""},
		"timeout": {"60", "30"},
		"retries": {"1", "0"},
	}
	if rec := postForm(t, s, "/settings/chain?name=tts", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings/chain = %d, want 303", rec.Code)
	}
	rec := get(t, s, "/settings/keys/status?chain=tts&provider=gemini")
	var st struct {
		Keys []KeyStatus `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Keys) != 1 || st.Keys[0].Masked != "••••••••9999" {
		t.Fatalf("keys after chain save = %+v, want the preserved key", st.Keys)
	}
}

// (10) The settings page renders the keyring UI with masked keys only.
func TestSettingsPageMasksKeys(t *testing.T) {
	s := newTestServer(t)
	const key = "AIzaxxxxxxxxxxxxxpage-render-SECRET4321"
	if rec := postForm(t, s, "/settings/keys/add?chain=llm&provider=gemini",
		url.Values{"key": {key}}); rec.Code != http.StatusOK {
		t.Fatalf("add = %d, want 200", rec.Code)
	}
	rec := get(t, s, "/settings/providers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/providers = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, key) {
		t.Fatalf("settings page leaked the raw key")
	}
	for _, want := range []string{`class="keyring"`, "••••••••4321", "Hoạt động", "+ Thêm key", "aistudio.google.com", `id="toast"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings page missing %q", want)
		}
	}
}

// (11) MaskKey / FillDerived unit checks.
func TestMaskKey(t *testing.T) {
	if got := MaskKey("abcdef1234"); got != "••••••••1234" {
		t.Fatalf("MaskKey = %q", got)
	}
	if got := MaskKey("k1"); got != "••••" {
		t.Fatalf("MaskKey(short) = %q, want fully hidden", got)
	}
	ks := KeyStatus{Index: 0, Last4: "1234", State: "cooldown", CooldownRemainingSec: 42}
	ks.FillDerived()
	if ks.BadgeClass != "badge-warn" || ks.StateLabel != "Nghỉ cooldown" {
		t.Fatalf("FillDerived cooldown = %+v", ks)
	}
	if ks.CooldownLabel() != "Nghỉ cooldown còn 42s" {
		t.Fatalf("CooldownLabel = %q", ks.CooldownLabel())
	}
	ks = KeyStatus{Index: 0, Last4: "ab", State: "ok"}
	ks.FillDerived()
	if ks.Masked != "••••" || ks.Last4 != "••••" {
		t.Fatalf("FillDerived short key leaked: %+v", ks)
	}
}

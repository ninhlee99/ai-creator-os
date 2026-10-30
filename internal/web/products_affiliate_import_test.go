package web

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func TestTikTokCookieHeaderForURLScopesSavedSession(t *testing.T) {
	store, err := webai.NewStore(filepath.Join(t.TempDir(), "webai.db"))
	if err != nil {
		t.Fatalf("webai.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.UpsertAccountSessionWithCookieJar(webai.PlatformTikTok, "", "", []webai.BrowserCookie{{
		Name: "session", Value: "logged-in", Domain: ".tiktok.com", Path: "/", Secure: true,
	}}, "TikTok", "", "")
	if err != nil {
		t.Fatalf("save TikTok session: %v", err)
	}
	if got := tikTokCookieHeaderForURL(store, "https://shop.tiktok.com/product/1"); got != "session=logged-in" {
		accounts, _ := store.GetAccounts(webai.PlatformTikTok)
		t.Fatalf("TikTok cookie = %q, want scoped session cookie (accounts=%+v)", got, func() []string {
			var state []string
			for _, a := range accounts {
				state = append(state, fmt.Sprintf("status=%s active=%t cookies=%d", a.Status, a.IsActive, len(a.CookieJar)))
			}
			return state
		}())
	}
	if got := tikTokCookieHeaderForURL(store, "https://tiktok.com.evil.example/image.jpg"); got != "" {
		t.Fatalf("unrelated host received TikTok cookie %q", got)
	}
}

func TestProductSampleVideoUploadIsSavedForWorkflow(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("sample_video", "boutique-reference.mp4")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.WriteString(part, "reference-video-bytes"); err != nil {
		t.Fatalf("write sample video: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/products/create-video", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}

	path, err := saveUpload(req, "sample_video", t.TempDir())
	if err != nil {
		t.Fatalf("save sample video: %v", err)
	}
	if path == "" || filepath.Ext(path) != ".mp4" {
		t.Fatalf("saved sample video path = %q, want .mp4 file", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved sample video: %v", err)
	}
	if string(got) != "reference-video-bytes" {
		t.Fatalf("saved sample video bytes = %q", got)
	}
}

func TestHandleProductsImportLink(t *testing.T) {
	mockAffiliateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta property="og:title" content="Bộ Dụng Cụ Nấu Ăn Silicon Chịu Nhiệt Cao Cấp | Shopee Việt Nam">
	<meta property="og:image" content="https://example.com/bep-silicon.jpg">
	<meta property="product:price:amount" content="289000">
	<meta property="og:site_name" content="Shopee">
</head>
<body>
	<div>Giá: 289.000₫</div>
</body>
</html>`))
	}))
	defer mockAffiliateServer.Close()

	s := newTestServer(t)
	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	form := url.Values{
		"url": {mockAffiliateServer.URL + "/product/i.123.456789"},
	}
	rec := postForm(t, s, "/products/import-link", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /products/import-link code = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("expected success redirect with ok=..., got %q", loc)
	}

	shelf, err := s.Products.Shelf(10)
	if err != nil {
		t.Fatalf("Products.Shelf failed: %v", err)
	}
	if len(shelf) == 0 {
		t.Fatal("expected at least 1 shelf product")
	}

	var found *products.Product
	for _, p := range shelf {
		if strings.Contains(p.Title, "Bộ Dụng Cụ Nấu Ăn Silicon") {
			found = &p
			break
		}
	}
	if found == nil {
		t.Fatalf("imported product not found in shelf, got shelf: %+v", shelf)
	}
	if found.Price != 289000 {
		t.Errorf("Price mismatch: got %f, want 289000", found.Price)
	}
	if found.Theme != "home" {
		t.Errorf("Theme mismatch: got %q, want home", found.Theme)
	}
}

func TestHandleProductsCreateVideoRequiresChannelCharacter(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st

	store, err := products.NewStore(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatalf("products.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	s.Products = store

	prodID, err := s.Products.SaveImportedProduct(products.ExtractedProduct{
		PlatformPID:    "TEST-PID-99",
		Title:          "Áo Thun Unisex Cotton 100%",
		Price:          150000,
		Currency:       "VND",
		CommissionRate: 0.12,
		Theme:          "fashion",
		Category:       "Thời trang",
		ProductURL:     "https://example.com/ao-thun",
	})
	if err != nil {
		t.Fatalf("SaveImportedProduct failed: %v", err)
	}

	account, err := s.Mgr.Add("profile_without_model", "fashion", "")
	if err != nil {
		t.Fatalf("Mgr.Add: %v", err)
	}
	form := url.Values{
		"product_id": {strconv.FormatInt(prodID, 10)},
		"account":    {account.Username},
	}
	rec := postForm(t, s, "/products/create-video", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /products/create-video code = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	parsed, err := url.Parse(loc)
	if err != nil || !strings.Contains(parsed.Query().Get("err"), "kênh chưa có ảnh nhân vật") {
		t.Fatalf("redirect=%q, want missing profile-character error", loc)
	}
	if jobs := s.Studio.ListJobs(10); len(jobs) != 0 {
		t.Fatalf("missing profile character must stop before queueing; jobs=%+v", jobs)
	}
}

func TestResolveProductJobCharacterPhotosStaysInSelectedAccount(t *testing.T) {
	s := newViralServer(t)
	selected, err := s.Mgr.Add("selected_creator", "fashion", "")
	if err != nil {
		t.Fatalf("Mgr.Add selected: %v", err)
	}
	other, err := s.Mgr.Add("other_creator", "fashion", "")
	if err != nil {
		t.Fatalf("Mgr.Add other: %v", err)
	}
	emptyProfile, err := s.Mgr.Add("empty_profile", "fashion", "")
	if err != nil {
		t.Fatalf("Mgr.Add empty profile: %v", err)
	}
	selectedSrc := filepath.Join(t.TempDir(), "selected.jpg")
	otherSrc := filepath.Join(t.TempDir(), "other.jpg")
	if err := os.WriteFile(selectedSrc, []byte("selected-character"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherSrc, []byte("other-character"), 0o600); err != nil {
		t.Fatal(err)
	}
	models, err := s.Studio.ModelLibrary()
	if err != nil {
		t.Fatalf("ModelLibrary: %v", err)
	}
	selectedPhoto, err := models.Add(selected.ID, selectedSrc)
	if err != nil {
		t.Fatalf("add selected photo: %v", err)
	}
	otherPhoto, err := models.Add(other.ID, otherSrc)
	if err != nil {
		t.Fatalf("add other photo: %v", err)
	}

	photos, err := resolveProductJobCharacterPhotos(s, selected.ID, 0)
	if err != nil {
		t.Fatalf("resolve selected profile: %v", err)
	}
	if len(photos) != 1 || photos[0] != selectedPhoto.Path {
		t.Fatalf("profile photos=%v, want selected account photo %q", photos, selectedPhoto.Path)
	}
	if _, err := resolveProductJobCharacterPhotos(s, selected.ID, otherPhoto.ID); err == nil {
		t.Fatal("photo from another account must be rejected")
	}
	if _, err := resolveProductJobCharacterPhotos(s, emptyProfile.ID, 0); err == nil {
		t.Fatal("account without a profile character must be rejected")
	}
}

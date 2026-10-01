package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestParseOAuthRedirect(t *testing.T) {
	code, state, oerr := parseOAuthRedirect("  https://www.tiktok.com/?code=abc%2A1&scopes=video.upload&state=s1#x ")
	if code != "abc*1" || state != "s1" || oerr != "" {
		t.Fatalf("got %q %q %q", code, state, oerr)
	}
	if _, _, oerr := parseOAuthRedirect("https://www.tiktok.com/?error=access_denied&error_description=no"); !strings.Contains(oerr, "access_denied") {
		t.Fatalf("error not surfaced: %q", oerr)
	}
	if code, _, _ := parseOAuthRedirect("code=zz&state=q"); code != "zz" {
		t.Fatalf("bare query: %q", code)
	}
}

func TestTikTokConnectFlow(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("shop.vn", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TIKTOK_CLIENT_KEY", "ck123")
	t.Setenv("TIKTOK_CLIENT_SECRET", "secret")

	// Token badge uses the publisher's sanitized file name.
	if err := os.WriteFile("tiktok_token_shop_vn.json", []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body := get(t, s, "/publishers").Body.String()
	if !strings.Contains(body, "Kết nối lại") || !strings.Contains(body, "badge-ok") {
		t.Fatalf("publishers page should show connected TikTok:\n%s", body)
	}
	if strings.Contains(body, "secret") {
		t.Fatal("client secret leaked into page")
	}

	rec := get(t, s, "/publishers/tiktok/authorize?username=shop.vn")
	if rec.Code != http.StatusFound {
		t.Fatalf("authorize = %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	q := loc.Query()
	if loc.Host != "www.tiktok.com" || q.Get("client_key") != "ck123" || q.Get("state") == "" ||
		q.Get("scope") != "user.info.basic,video.upload" {
		t.Fatalf("bad authorize url %s", loc)
	}
	// Default = local Desktop flow: redirect back to this app + PKCE.
	if q.Get("redirect_uri") != "http://127.0.0.1:8080/publishers/tiktok/callback" ||
		len(q.Get("code_challenge")) != 64 || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("desktop flow params missing: %s", loc)
	}
	rec = get(t, s, "/publishers/tiktok/callback?code=c&state=forged")
	if l := rec.Header().Get("Location"); !strings.Contains(l, "err=") {
		t.Fatalf("callback accepted forged state: %s", l)
	}
	rec = get(t, s, "/publishers/tiktok/callback?error=access_denied&state="+q.Get("state"))
	if l, _ := url.QueryUnescape(rec.Header().Get("Location")); !strings.Contains(l, "access_denied") {
		t.Fatalf("callback error not surfaced: %s", l)
	}

	// Web flow (public redirect): no PKCE, paste form shown.
	t.Setenv("TIKTOK_REDIRECT_URI", "https://example.github.io/cb.html")
	if body := get(t, s, "/publishers").Body.String(); !strings.Contains(body, "Lưu token") {
		t.Fatal("paste form missing in web flow")
	}
	rec = get(t, s, "/publishers/tiktok/authorize?username=shop.vn")
	loc, _ = url.Parse(rec.Header().Get("Location"))
	q = loc.Query()
	if q.Get("code_challenge") != "" || q.Get("redirect_uri") != "https://example.github.io/cb.html" {
		t.Fatalf("web flow should not use PKCE: %s", loc)
	}

	// Wrong account / wrong state never reach TikTok.
	if _, err := s.Mgr.Add("other", "", ""); err != nil {
		t.Fatal(err)
	}
	rec = postForm(t, s, "/publishers/tiktok/connect", url.Values{
		"username": {"other"}, "redirect": {"https://www.tiktok.com/?code=c&state=" + q.Get("state")}})
	if l := rec.Header().Get("Location"); !strings.Contains(l, "err=") {
		t.Fatalf("state for another account accepted: %s", l)
	}
	rec = postForm(t, s, "/publishers/tiktok/connect", url.Values{
		"username": {"shop.vn"}, "redirect": {"https://www.tiktok.com/?code=c&state=forged"}})
	if l := rec.Header().Get("Location"); !strings.Contains(l, "err=") {
		t.Fatalf("forged state accepted: %s", l)
	}
	rec = get(t, s, "/publishers/tiktok/authorize?username=ghost")
	if l := rec.Header().Get("Location"); !strings.HasPrefix(l, "/publishers?err=") {
		t.Fatalf("unknown account: %s", l)
	}
}

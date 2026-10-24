package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func TestTikTokConnectFlow(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.Mgr.Add("shop.vn", "", ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TIKTOK_CLIENT_KEY", "ck123")
	t.Setenv("TIKTOK_CLIENT_SECRET", "secret")

	// Token badge reads the app-scoped token directory and sanitized name.
	tokenPath := publishers.TikTokTokenPath("shop.vn")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body := get(t, s, "/publishers").Body.String()
	if !strings.Contains(body, "Kết nối lại") || !strings.Contains(body, "Có token · API chưa kiểm") {
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
		q.Get("scope") != "user.info.basic,user.info.stats,video.list,video.upload" {
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

	rec = get(t, s, "/publishers/tiktok/authorize?username=ghost")
	if l := rec.Header().Get("Location"); !strings.HasPrefix(l, "/publishers?err=") {
		t.Fatalf("unknown account: %s", l)
	}
}

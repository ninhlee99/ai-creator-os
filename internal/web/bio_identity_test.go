package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func TestEnsureBioDeviceCookieUsesFirstPartyHTTPOnlyScope(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://example.test/bio/demo", nil)
	w := httptest.NewRecorder()
	token := ensureBioDeviceCookie(w, r)
	if !validBioDeviceToken(token) {
		t.Fatalf("invalid generated device token %q", token)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != bioDeviceCookie || cookie.Path != "/bio" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected device cookie: %+v", cookie)
	}

	request := httptest.NewRequest(http.MethodGet, "https://example.test/bio/demo/product/1", nil)
	request.AddCookie(cookie)
	again := httptest.NewRecorder()
	if got := ensureBioDeviceCookie(again, request); got != token {
		t.Fatalf("existing device token changed: %q != %q", got, token)
	}
	if len(again.Result().Cookies()) != 0 {
		t.Fatal("valid device token was unnecessarily rotated")
	}
}

func TestBioTryOnKeyIsStablePrivateAndIdentityScoped(t *testing.T) {
	dir := t.TempDir()
	s := &Server{DataDir: dir}
	first, err := s.bioTryOnKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.bioTryOnKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("try-on HMAC key changed between requests")
	}
	info, err := os.Stat(filepath.Join(dir, "bio-tryon.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %04o, want 0600", info.Mode().Perm())
	}

	device := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	remote := httptest.NewRequest(http.MethodPost, "https://example.test/bio/demo/products/1/try-on", nil)
	remote.RemoteAddr = "198.51.100.4:4321"
	remote.Header.Set("X-Forwarded-For", "198.51.100.99")
	ipHash, deviceHash, err := s.bioIdentityHashes(remote, device)
	if err != nil {
		t.Fatal(err)
	}
	if ipHash == "" || deviceHash == "" || ipHash == "198.51.100.4" || deviceHash == device {
		t.Fatalf("identity hashes exposed raw identity: ip=%q device=%q", ipHash, deviceHash)
	}
	peerOnly := httptest.NewRequest(http.MethodPost, "https://example.test/bio/demo/products/1/try-on", nil)
	peerOnly.RemoteAddr = "198.51.100.4:4321"
	peerHash, _, err := s.bioIdentityHashes(peerOnly, device)
	if err != nil {
		t.Fatal(err)
	}
	if ipHash != peerHash {
		t.Fatal("untrusted X-Forwarded-For changed quota identity")
	}

	loopback := httptest.NewRequest(http.MethodPost, "https://example.test/bio/demo/products/1/try-on", nil)
	loopback.RemoteAddr = "127.0.0.1:4321"
	loopbackHash, _, err := s.bioIdentityHashes(loopback, device)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewRequest(http.MethodPost, "https://example.test/bio/demo/products/1/try-on", nil)
	proxy.RemoteAddr = "127.0.0.1:4321"
	proxy.Header.Set("CF-Connecting-IP", "198.51.100.9")
	untrustedProxyHash, _, err := s.bioIdentityHashes(proxy, device)
	if err != nil {
		t.Fatal(err)
	}
	if untrustedProxyHash != loopbackHash {
		t.Fatal("CF-Connecting-IP was used while Cloudflare trust was disabled")
	}

	s.TrustCloudflareIP = true
	proxiedHash, _, err := s.bioIdentityHashes(proxy, device)
	if err != nil {
		t.Fatal(err)
	}
	cloudflarePeer := httptest.NewRequest(http.MethodPost, "https://example.test/bio/demo/products/1/try-on", nil)
	cloudflarePeer.RemoteAddr = "198.51.100.9:4321"
	expectedCloudflareHash, _, err := s.bioIdentityHashes(cloudflarePeer, device)
	if err != nil {
		t.Fatal(err)
	}
	if proxiedHash != expectedCloudflareHash || proxiedHash == loopbackHash {
		t.Fatal("trusted CF-Connecting-IP did not define quota identity")
	}
}

func TestBioAnalyticsDedupeSurvivesServerRestartWithoutRawIP(t *testing.T) {
	dir := t.TempDir()
	store, err := products.NewStore(filepath.Join(dir, "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet, "https://example.test/bio/creator", nil)
	request.RemoteAddr = "198.51.100.23:4321"
	first := &Server{Products: store, DataDir: dir}
	if !first.recordBioMetric(request, 1, 0, "profile_view", at, bioPageViewDedupeWindow) {
		t.Fatal("first profile view was not recorded")
	}

	second := &Server{Products: store, DataDir: dir}
	if second.recordBioMetric(request, 1, 0, "profile_view", at.Add(time.Second), bioPageViewDedupeWindow) {
		t.Fatal("same page view was counted again after server recreation")
	}
	if !second.recordBioMetric(request, 1, 12, "product_view", at.Add(2*time.Second), bioPageViewDedupeWindow) {
		t.Fatal("product detail view was incorrectly deduplicated against profile view")
	}

	daily, err := store.BioDailyMetrics(1, "2026-10-23")
	if err != nil || len(daily) != 1 || daily[0].ProfileViews != 1 || daily[0].ProductViews != 1 {
		t.Fatalf("daily metrics = %+v, err=%v; want one profile and one product view", daily, err)
	}
	key, err := second.bioAnalyticsDedupeKey("198.51.100.23", 1, 0, "profile_view")
	if err != nil {
		t.Fatal(err)
	}
	if key == "198.51.100.23" || len(key) != 64 {
		t.Fatalf("dedupe key exposes or misformats identity: %q", key)
	}
	info, err := os.Stat(filepath.Join(dir, "bio-analytics.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("analytics key permissions = %04o, want 0600", info.Mode().Perm())
	}
}

func TestPublicBioRouteAllowlistAndRemoteGate(t *testing.T) {
	s := &Server{RemoteNeedsPassword: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	gated := s.authGate(next)
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/bio"},
		{http.MethodGet, "/bio/@creator"},
		{http.MethodGet, "/bio/creator/product/12"},
		{http.MethodGet, "/bio/creator/go/12"},
		{http.MethodPost, "/bio/creator/products/12/voucher-copy"},
		{http.MethodPost, "/bio/creator/products/12/try-on"},
		{http.MethodGet, "/bio/try-on/aaaaaaaaaaaaaaaaaaaaaaaa"},
		{http.MethodGet, "/bio/try-on/aaaaaaaaaaaaaaaaaaaaaaaa/image"},
		{http.MethodGet, "/bio-go/12"},
	}
	for _, test := range allowed {
		r := httptest.NewRequest(test.method, test.path, nil)
		r.RemoteAddr = "203.0.113.8:9000"
		w := httptest.NewRecorder()
		gated.ServeHTTP(w, r)
		if w.Code != http.StatusAccepted {
			t.Errorf("public route %s %s = %d, want %d", test.method, test.path, w.Code, http.StatusAccepted)
		}
	}
	denied := []struct{ method, path string }{
		{http.MethodGet, "/bio-admin"},
		{http.MethodGet, "/bio-admin/analytics"},
		{http.MethodPost, "/bio-admin/product"},
		{http.MethodPost, "/bio/pin"},
		{http.MethodGet, "/products"},
		{http.MethodGet, "/api/missions/1"},
		{http.MethodGet, "/bio/creator/products/12/try-on"},
		{http.MethodGet, "/bio/creator/private/12"},
	}
	for _, test := range denied {
		r := httptest.NewRequest(test.method, test.path, nil)
		r.RemoteAddr = "203.0.113.8:9000"
		w := httptest.NewRecorder()
		gated.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("private route %s %s = %d, want %d", test.method, test.path, w.Code, http.StatusForbidden)
		}
	}
}

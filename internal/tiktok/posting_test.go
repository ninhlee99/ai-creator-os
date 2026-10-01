package tiktok

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TokenStore round-trip
// ---------------------------------------------------------------------------

func TestTokenStoreSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "tiktok_token_test.json")
	store := &TokenStore{Path: path}

	// Missing file -> zero tokens, no error.
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if got.AccessToken != "" || got.RefreshToken != "" {
		t.Fatalf("expected zero tokens, got %+v", got)
	}

	before := time.Now().Unix()
	in := Tokens{AccessToken: "acc-1", RefreshToken: "ref-1", ExpiresIn: 86400}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != "acc-1" || got.RefreshToken != "ref-1" || got.ExpiresIn != 86400 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.ObtainedAt < before || got.ObtainedAt > time.Now().Unix() {
		t.Fatalf("obtained_at not stamped: %d", got.ObtainedAt)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file should be 0600: %v %v", fi, err)
	}
}

// ---------------------------------------------------------------------------
// AccessToken: fresh reuse vs refresh with rotation persistence
// ---------------------------------------------------------------------------

func TestAccessTokenRefreshesAndPersistsRotation(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	// Seed an expired token pair.
	if err := store.Save(Tokens{AccessToken: "old", RefreshToken: "ref-old", ExpiresIn: 10}); err != nil {
		t.Fatal(err)
	}
	// Backdate obtained_at so it is expired.
	raw, _ := os.ReadFile(store.Path)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["obtained_at"] = time.Now().Unix() - 1000
	raw, _ = json.Marshal(m)
	_ = os.WriteFile(store.Path, raw, 0o600)

	var sawForm string
	fake := func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		sawForm = string(body)
		return 200, []byte(`{"access_token":"new-acc","refresh_token":"ref-rotated","expires_in":86400}`), nil
	}
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store, HTTP: fake}
	tok, err := c.AccessToken()
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok != "new-acc" {
		t.Fatalf("got %q", tok)
	}
	if !strings.Contains(sawForm, "grant_type=refresh_token") || !strings.Contains(sawForm, "refresh_token=ref-old") {
		t.Fatalf("refresh form wrong: %q", sawForm)
	}
	// Rotated tokens persisted.
	saved, _ := store.Load()
	if saved.AccessToken != "new-acc" || saved.RefreshToken != "ref-rotated" {
		t.Fatalf("rotation not persisted: %+v", saved)
	}

	// Second call reuses the fresh token without HTTP.
	c.HTTP = func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		t.Fatal("should not hit network for fresh token")
		return 0, nil, nil
	}
	tok2, err := c.AccessToken()
	if err != nil || tok2 != "new-acc" {
		t.Fatalf("fresh reuse: %q %v", tok2, err)
	}
}

func TestAuthorizeURL(t *testing.T) {
	c := &PostingClient{ClientKey: "KEY123", Store: &TokenStore{}}
	u := c.AuthorizeURL("https://app.example/cb", ScopeDirectPost, "st8", "")
	for _, want := range []string{"client_key=KEY123", "redirect_uri=", "response_type=code", "scope=video.publish", "state=st8"} {
		if !strings.Contains(u, want) {
			t.Fatalf("AuthorizeURL missing %q: %s", want, u)
		}
	}
}

func TestPKCEDesktopFlow(t *testing.T) {
	v, ch := NewPKCE()
	if len(v) != 64 || strings.Trim(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~") != "" {
		t.Fatalf("bad verifier %q", v)
	}
	sum := sha256.Sum256([]byte(v))
	if ch != hex.EncodeToString(sum[:]) {
		t.Fatal("challenge must be hex sha256(verifier)")
	}
	c := &PostingClient{ClientKey: "K", ClientSecret: "S", Store: &TokenStore{Path: filepath.Join(t.TempDir(), "t.json")}}
	u := c.AuthorizeURL("http://127.0.0.1:8080/cb", ScopeUploadDraft, "st", ch)
	if !strings.Contains(u, "code_challenge="+ch) || !strings.Contains(u, "code_challenge_method=S256") {
		t.Fatalf("missing PKCE params: %s", u)
	}
	var form string
	c.HTTP = func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		form = string(body)
		return 200, []byte(`{"access_token":"a","refresh_token":"r","expires_in":86400,"scope":"video.upload"}`), nil
	}
	if _, err := c.ExchangeCode("code1", "http://127.0.0.1:8080/cb", v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(form, "code_verifier="+v) {
		t.Fatalf("verifier not sent: %s", form)
	}
}

// ---------------------------------------------------------------------------
// PublishFile with fake HTTP: init -> PUT chunks with Content-Range
// ---------------------------------------------------------------------------

type putCall struct {
	url     string
	headers map[string]string
	n       int
}

func fakeUploadServer(t *testing.T, publishID string, puts *[]putCall) HTTPFunc {
	return func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		switch {
		case method == "POST" && strings.HasSuffix(url, "/init/"):
			var initBody map[string]any
			if err := json.Unmarshal(body, &initBody); err != nil {
				t.Fatalf("init body not JSON: %v", err)
			}
			src, _ := initBody["source_info"].(map[string]any)
			if src["source"] != "FILE_UPLOAD" {
				t.Fatalf("source_info.source = %v", src["source"])
			}
			return 200, []byte(`{"error":{"code":"ok"},"data":{"publish_id":"` + publishID + `","upload_url":"https://upload.example/x"}}`), nil
		case method == "PUT":
			*puts = append(*puts, putCall{url: url, headers: headers, n: len(body)})
			return 200, []byte(`{}`), nil
		default:
			t.Fatalf("unexpected %s %s", method, url)
			return 0, nil, nil
		}
	}
}

func writeSizedFile(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.mp4")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse write: seek to size-1 and write one byte.
	if _, err := f.Seek(size-1, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x1}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func TestPublishFileSingleChunk(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})

	var puts []putCall
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: fakeUploadServer(t, "pid-1", &puts)}

	path := writeSizedFile(t, 1024) // < 5MB -> single chunk
	id, err := c.PublishFile(path, "title", "", true /*draft*/)
	if err != nil {
		t.Fatalf("PublishFile: %v", err)
	}
	if id != "pid-1" {
		t.Fatalf("publish_id = %q", id)
	}
	if len(puts) != 1 {
		t.Fatalf("expected 1 PUT, got %d", len(puts))
	}
	if got := puts[0].headers["Content-Range"]; got != "bytes 0-1023/1024" {
		t.Fatalf("Content-Range = %q", got)
	}
	if puts[0].url != "https://upload.example/x" {
		t.Fatalf("upload url = %q", puts[0].url)
	}
}

func TestPublishFileMultiChunk(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})

	var puts []putCall
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: fakeUploadServer(t, "pid-2", &puts)}

	// > 64 MB -> 10 MB chunks, count = floor(size/10MB), last chunk takes
	// the remainder (TikTok rejects a ceil() count).
	const mb = 1024 * 1024
	const size = 75*mb + 123
	path := writeSizedFile(t, size)
	id, err := c.PublishFile(path, "title", "SELF_ONLY", false)
	if err != nil {
		t.Fatalf("PublishFile: %v", err)
	}
	if id != "pid-2" {
		t.Fatalf("publish_id = %q", id)
	}
	if len(puts) != 7 {
		t.Fatalf("expected 7 PUTs, got %d", len(puts))
	}
	if got, w := puts[0].headers["Content-Range"], fmt.Sprintf("bytes 0-%d/%d", 10*mb-1, size); got != w {
		t.Fatalf("first Content-Range = %q, want %q", got, w)
	}
	if got, w := puts[6].headers["Content-Range"], fmt.Sprintf("bytes %d-%d/%d", 60*mb, size-1, size); got != w {
		t.Fatalf("last Content-Range = %q, want %q", got, w)
	}
	if puts[6].n != 15*mb+123 {
		t.Fatalf("last chunk size = %d", puts[6].n)
	}
}

func TestPublishFileMidSizeIsSingleChunk(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})
	var puts []putCall
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: fakeUploadServer(t, "pid-3", &puts)}
	// A typical 30s 1080x1920 render (~45 MB) must go up whole.
	const size = 45*1024*1024 + 7
	if _, err := c.PublishFile(writeSizedFile(t, size), "t", "", true); err != nil {
		t.Fatalf("PublishFile: %v", err)
	}
	if len(puts) != 1 || puts[0].n != size {
		t.Fatalf("want 1 PUT of %d bytes, got %d PUTs", size, len(puts))
	}
}

func TestExchangeCodeErrorKeepsTokenFile(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "good", RefreshToken: "r-good", ExpiresIn: 86400})
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"error":"invalid_grant","error_description":"code expired"}`), nil
		}}
	if _, err := c.ExchangeCode("bad", "https://x/", ""); err == nil {
		t.Fatal("expected exchange error")
	}
	if _, err := c.AccessToken(); err != nil {
		t.Fatalf("token file should be untouched: %v", err)
	}
	got, _ := store.Load()
	if got.RefreshToken != "r-good" {
		t.Fatalf("refresh token clobbered: %+v", got)
	}
}

func TestRefreshErrorKeepsRefreshToken(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "old", RefreshToken: "r-keep", ExpiresIn: 1})
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"error":"temporarily_unavailable"}`), nil
		}}
	if _, err := c.AccessToken(); err == nil {
		t.Fatal("expected refresh error")
	}
	got, _ := store.Load()
	if got.RefreshToken != "r-keep" {
		t.Fatalf("refresh token wiped: %+v", got)
	}
}

func TestPublishFileInitError(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"error":{"code":"quota_exceeded","message":"nope"}}`), nil
		}}
	if _, err := c.PublishFile(writeSizedFile(t, 64), "t", "", false); err == nil {
		t.Fatal("expected init error")
	}
}

// ---------------------------------------------------------------------------
// PollStatus
// ---------------------------------------------------------------------------

func TestPollStatusComplete(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})

	calls := 0
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		PollInterval: time.Millisecond,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			if !strings.HasSuffix(url, "/status/fetch/") {
				t.Fatalf("unexpected url %s", url)
			}
			var b map[string]any
			_ = json.Unmarshal(body, &b)
			if b["publish_id"] != "pid-9" {
				t.Fatalf("publish_id = %v", b["publish_id"])
			}
			calls++
			st := "PROCESSING"
			if calls >= 3 {
				st = "PUBLISH_COMPLETE"
			}
			return 200, []byte(`{"data":{"status":"` + st + `"}}`), nil
		}}
	st, err := c.PollStatus("pid-9", 5*time.Second)
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if st != "PUBLISH_COMPLETE" {
		t.Fatalf("status = %q", st)
	}
	if calls != 3 {
		t.Fatalf("expected 3 polls, got %d", calls)
	}
}

func TestPollStatusFailed(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		PollInterval: time.Millisecond,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"data":{"status":"FAILED"}}`), nil
		}}
	st, err := c.PollStatus("pid-x", 5*time.Second)
	if err != nil || st != "FAILED" {
		t.Fatalf("got %q %v", st, err)
	}
}

func TestPollStatusTimeout(t *testing.T) {
	dir := t.TempDir()
	store := &TokenStore{Path: filepath.Join(dir, "tok.json")}
	_ = store.Save(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 86400})
	c := &PostingClient{ClientKey: "k", ClientSecret: "s", Store: store,
		PollInterval: time.Millisecond,
		HTTP: func(method, url string, headers map[string]string, body []byte) (int, []byte, error) {
			return 200, []byte(`{"data":{"status":"PROCESSING"}}`), nil
		}}
	if _, err := c.PollStatus("pid-x", 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
}

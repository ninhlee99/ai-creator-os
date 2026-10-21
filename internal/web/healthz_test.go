package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tick kẹt → needs_restart=true để watchdog ngoài tự khởi động lại.
func TestHealthzStaleTicks(t *testing.T) {
	s := newTestServer(t)
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("json: %v", err)
		}
		return body
	}

	if b := get(); b["needs_restart"] != false {
		t.Fatalf("needs_restart = %v, want false when nothing wired", b["needs_restart"])
	}

	s.StaleTicks = func() []string { return []string{"story"} }
	b := get()
	if b["needs_restart"] != true {
		t.Fatalf("needs_restart = %v, want true with a stale tick", b["needs_restart"])
	}
}

// Đợt AK: phát media phải nới deadline ghi — video lớn qua mạng chậm không
// bị WriteTimeout 60s của server cắt giữa chừng.
func TestServeMediaFileExtendsWriteDeadline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(path, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // vượt WriteTimeout bên dưới
		serveMediaFile(w, r, path)
	}))
	srv.Config.WriteTimeout = 50 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil || n != 1<<20 {
		t.Fatalf("read %d bytes, err=%v; want full file", n, err)
	}
}

// Đợt AK: CSS/JS mang ?v=<hash> → cập nhật app là trình duyệt lấy bản mới.
func TestStaticAssetsCacheBusted(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if want := "/static/style.css?v=" + assetVersion; !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("page missing %q", want)
	}
	for url, cc := range map[string]string{
		"/static/style.css?v=" + assetVersion: "immutable",
		"/static/style.css":                   "no-cache",
		"/static/app.js?v=old":                "no-cache",
	} {
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, cc) {
			t.Errorf("%s Cache-Control = %q, want %q", url, got, cc)
		}
	}
}

func TestDiskBadgeSeverity(t *testing.T) {
	for pct, want := range map[int]string{10: "badge-ok", 75: "badge-warn", 85: "badge-err", 97: "badge-err"} {
		if got := diskBadge(pct); got != want {
			t.Errorf("diskBadge(%d) = %s, want %s", pct, got, want)
		}
	}
}

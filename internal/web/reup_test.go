package web

// Reup Đợt D: test fail-closed (kho nil) và render trang /reup.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// newReupServer dựng server test có kho reup thật.
func newReupServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	rs, err := reup.NewStore(filepath.Join(t.TempDir(), "reup.db"))
	if err != nil {
		t.Fatalf("reup.NewStore: %v", err)
	}
	t.Cleanup(func() { rs.Close() })
	s.Reup = rs
	return s
}

func TestReupPageNoStore(t *testing.T) {
	s := newTestServer(t) // Reup = nil
	rec := get(t, s, "/reup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reup = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Kho Reup chưa sẵn sàng") {
		t.Fatalf("thiếu kho phải báo trung thực, không sập trang")
	}
}

func TestReupPageRenders(t *testing.T) {
	s := newReupServer(t)
	if _, err := s.Reup.AddSource("user", "than_tien", "Thần Tiên"); err != nil {
		t.Fatal(err)
	}
	rec := get(t, s, "/reup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reup = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Reup Douyin", "Thần Tiên", "Nguồn Douyin", "yt-dlp", "Hàng đợi tải", "Tự động"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /reup thiếu %q", want)
		}
	}
	// Sidebar phải có mục Reup (8 mục).
	if !strings.Contains(body, `href="/reup"`) {
		t.Errorf("sidebar thiếu link /reup")
	}
}

func TestReupScanNoStore(t *testing.T) {
	s := newTestServer(t)
	rec := postJSON(t, s, "/reup/scan", "{}")
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("thiếu kho phải 412, được %d", rec.Code)
	}
	var d map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("response phải là JSON: %v", err)
	}
	if _, ok := d["error"]; !ok {
		t.Errorf("thiếu field error: %v", d)
	}
}

func TestReupSourceAddToggleDelete(t *testing.T) {
	s := newReupServer(t)
	rec := postForm(t, s, "/reup/sources", url.Values{
		"kind": {"user"}, "value": {"@than_tien"}, "name": {"Thần Tiên"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("thêm nguồn phải 303, được %d", rec.Code)
	}
	srcs, _ := s.Reup.ListSources()
	if len(srcs) != 1 || srcs[0].Value != "than_tien" || !srcs[0].Enabled {
		t.Fatalf("nguồn chưa lưu đúng: %+v", srcs)
	}
	srcID := strconv.FormatInt(srcs[0].ID, 10)

	// Toggle → tắt.
	rec = postForm(t, s, "/reup/sources/"+srcID+"/toggle", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("toggle phải 303, được %d", rec.Code)
	}
	on, _ := s.Reup.EnabledSources()
	if len(on) != 0 {
		t.Errorf("toggle phải tắt nguồn")
	}

	// Xoá.
	rec = postForm(t, s, "/reup/sources/"+srcID+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("xoá phải 303, được %d", rec.Code)
	}
	if srcs, _ := s.Reup.ListSources(); len(srcs) != 0 {
		t.Errorf("nguồn phải bị xoá")
	}

	// Thêm với value trống → lỗi (redirect ?err=).
	rec = postForm(t, s, "/reup/sources", url.Values{"kind": {"user"}, "value": {""}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Errorf("value trống phải redirect kèm err, được %q", loc)
	}
}

func TestReupSettingsSave(t *testing.T) {
	s := newReupServer(t)
	rec := postJSON(t, s, "/reup/settings",
		`{"discover_on":false,"interval_hours":12,"per_source":5}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lưu settings phải 200, được %d", rec.Code)
	}
	if v := s.atSettingOn("reup.discover_enabled", true); v {
		t.Errorf("discover_on=false phải lưu thành tắt")
	}
	if v := s.atSettingInt("reup.discover_interval_hours", 6); v != 12 {
		t.Errorf("interval_hours phải = 12, được %d", v)
	}
	if v := s.atSettingInt("reup.videos_per_source", 3); v != 5 {
		t.Errorf("per_source phải = 5, được %d", v)
	}
	// JSON sai → 400.
	rec = postJSON(t, s, "/reup/settings", "{invalid")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("JSON sai phải 400, được %d", rec.Code)
	}
	// per_source vượt trần → cắt về 10.
	rec = postJSON(t, s, "/reup/settings", `{"per_source":99}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if v := s.atSettingInt("reup.videos_per_source", 3); v != 10 {
		t.Errorf("per_source phải cắt về 10, được %d", v)
	}
}

func TestReupVideoAddNoStore(t *testing.T) {
	s := newTestServer(t)
	rec := postForm(t, s, "/reup/videos/add", url.Values{"url": {"https://www.douyin.com/video/1"}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Errorf("thiếu kho phải redirect kèm err, được %q", loc)
	}
}

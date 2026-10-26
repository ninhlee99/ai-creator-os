package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func newViralServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	return s
}

func TestViralPageNoStudio(t *testing.T) {
	s := newTestServer(t) // Studio == nil
	rec := get(t, s, "/viral")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /viral = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Video nhảy") {
		t.Errorf("trang /viral thiếu tiêu đề: %s", body)
	}
}

func TestViralPageRenders(t *testing.T) {
	s := newViralServer(t)
	rec := get(t, s, "/viral")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /viral = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Video tham chiếu bắt buộc",
		"Tên video nhảy",
		"Video từ TikTok Radar",
		"Thay nhân vật",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /viral thiếu nội dung %q", want)
		}
	}
}

func TestViralCreate(t *testing.T) {
	s := newViralServer(t)

	// Test form create redirecting when editor is not ready
	rec := postForm(t, s, "/viral/create", url.Values{
		"topic":       {"3 Sai Lầm Tài Chính"},
		"genre":       {"lifehack"},
		"visual_mood": {"cinematic"},
		"duration":    {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /viral/create = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=") {
		t.Errorf("Location = %q, want err=", loc)
	}
}

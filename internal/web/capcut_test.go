package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestStudioCapCutUnknownJob(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	opened := 0
	old := openOnMac
	openOnMac = func(...string) error { opened++; return nil }
	t.Cleanup(func() { openOnMac = old })
	rec := postForm(t, s, "/studio/jobs/nope/capcut", nil)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("redirect = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if opened != 0 {
		t.Fatal("job lỗi thì không được mở Finder/CapCut")
	}
}

func TestMediaNameForPlayer(t *testing.T) {
	if got := mediaName("/abs/data/output/studio-1.mp4"); got != "studio-1.mp4" {
		t.Fatalf("mediaName = %q", got)
	}
	if mediaName("") != "" {
		t.Fatal("rỗng phải giữ rỗng")
	}
}

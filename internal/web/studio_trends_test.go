package web

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestStudioTrendsPageAndTopicActions(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st

	// 1. GET /studio/trends should render page with Top 5, Vietnam badge, and topics
	rec := get(t, s, "/studio/trends")
	if rec.Code != 200 {
		t.Fatalf("GET /studio/trends code = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Radar TikTok") {
		t.Errorf("body missing 'Radar TikTok'")
	}
	if !strings.Contains(body, "Chỉ tìm tại Việt Nam") && !strings.Contains(body, "Việt Nam") {
		t.Errorf("body missing Vietnam indicator")
	}
	if !strings.Contains(body, "Thêm chủ đề") {
		t.Errorf("body missing 'Thêm chủ đề' button")
	}

	// 2. POST /studio/trends/topics should add a new topic and redirect
	addRec := postForm(t, s, "/studio/trends/topics", url.Values{
		"new_topic": {"Review công nghệ"},
	})
	if addRec.Code != 303 {
		t.Fatalf("POST /studio/trends/topics code = %d, want 303", addRec.Code)
	}
	loc := addRec.Header().Get("Location")
	if !strings.Contains(loc, "topic=Review+c%C3%B4ng+ngh%E1%BB%87") {
		t.Errorf("expected redirect location with new topic, got %q", loc)
	}

	// 3. POST /studio/trends/topics/delete should delete topic and redirect
	delRec := postForm(t, s, "/studio/trends/topics/delete", url.Values{
		"topic": {"Review công nghệ"},
	})
	if delRec.Code != 303 {
		t.Fatalf("POST /studio/trends/topics/delete code = %d, want 303", delRec.Code)
	}

	// 4. POST /studio/trends/refresh should refresh current topic and redirect
	refRec := postForm(t, s, "/studio/trends/refresh", url.Values{
		"topic": {"Thời trang"},
	})
	if refRec.Code != 303 {
		t.Fatalf("POST /studio/trends/refresh code = %d, want 303", refRec.Code)
	}
	if !strings.Contains(refRec.Header().Get("Location"), "topic=Th%E1%BB%9Di+trang") {
		t.Errorf("expected refresh redirect location with topic, got %q", refRec.Header().Get("Location"))
	}
}

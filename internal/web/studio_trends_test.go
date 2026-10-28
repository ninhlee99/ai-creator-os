package web

import (
	"net/http/httptest"
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

	// Background refresh must update results only; replacing the topic controls
	// while the confirmation dialog is open can detach the delete form.
	pollReq := httptest.NewRequest("GET", "/studio/trends?topic=Th%E1%BB%9Di+trang", nil)
	pollReq.Header.Set("HX-Request", "true")
	pollReq.Header.Set("HX-Target", "studio-trends-results")
	pollRec := httptest.NewRecorder()
	serveLocalTestRoute(s, pollRec, pollReq)
	if pollRec.Code != 200 {
		t.Fatalf("HTMX results refresh code = %d, want 200", pollRec.Code)
	}
	if !strings.Contains(pollRec.Body.String(), `id="studio-trends-results"`) || strings.Contains(pollRec.Body.String(), `id="topic-chips-container"`) {
		t.Errorf("background refresh should return results only, got %q", pollRec.Body.String()[:min(len(pollRec.Body.String()), 300)])
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
	topicsAfterDelete, err := st.ListTikTokRadarTopics(httptest.NewRequest("GET", "/", nil).Context())
	if err != nil {
		t.Fatalf("ListTikTokRadarTopics after handler delete: %v", err)
	}
	for _, topic := range topicsAfterDelete {
		if topic == "Review công nghệ" {
			t.Fatalf("delete handler left custom topic %q in list", topic)
		}
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

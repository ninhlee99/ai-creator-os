package web

// Kể chuyện (Đợt F): test trang /stories — render, fail-closed khi Studio
// nil, tạo job redirect đúng, nút Đăng chặn khi job chưa xong.

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// newStoriesServer dựng server test có Studio thật (không providers —
// job chạy nền sẽ fail ở bước viết truyện, đúng fail-closed).
func newStoriesServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"),
		nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	return s
}

func TestStoriesPageNoStudio(t *testing.T) {
	s := newTestServer(t) // Studio = nil
	rec := get(t, s, "/stories")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /stories = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Studio chưa được khởi tạo") {
		t.Fatalf("studio nil phải báo trung thực, không sập trang")
	}
}

func TestStoriesPageRenders(t *testing.T) {
	s := newStoriesServer(t)
	rec := get(t, s, "/stories")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /stories = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Kể chuyện YouTube", "Kể chuyện mới", "Truyện đã tạo", "Mặc định", "Hàng đợi chủ đề", "Chờ duyệt mới đăng", `href="/stories"`} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /stories thiếu %q", want)
		}
	}
}

func TestStoriesCreateNoStudio(t *testing.T) {
	s := newTestServer(t) // Studio = nil
	rec := postForm(t, s, "/stories", url.Values{"topic": {"đêm mưa"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /stories = %d, want 303", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("studio nil phải redirect kèm ?err=")
	}
}

func TestStoriesCreateEmptyTopic(t *testing.T) {
	s := newStoriesServer(t)
	rec := postForm(t, s, "/stories", url.Values{"topic": {"  "}})
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("chủ đề trống phải bị chặn")
	}
}

func TestStoriesCreateQueuesJob(t *testing.T) {
	s := newStoriesServer(t)
	rec := postForm(t, s, "/stories", url.Values{
		"topic":    {"đêm mưa ở Đà Lạt"},
		"genre":    {"tâm lý"},
		"words":    {"400"},
		"scenes":   {"4"},
		"music_on": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /stories = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("Location=%q, want ?ok=", loc)
	}
	jobs := s.Studio.StoryJobs(5)
	if len(jobs) != 1 {
		t.Fatalf("storyJobs=%d, want 1", len(jobs))
	}
	if jobs[0].Status == studio.StatusFailed {
		t.Logf("job fail nhanh (không có providers) — đúng fail-closed")
	}
}

func TestStoriesPublishNotReady(t *testing.T) {
	s := newStoriesServer(t)
	rec := postForm(t, s, "/stories/st-khong-co/publish", url.Values{"account": {"kenh-x"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST publish = %d, want 303", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("job không tồn tại phải fail-closed kèm ?err=")
	}
}

func TestStoriesPublishWaitsQC(t *testing.T) {
	s := newStoriesServer(t)
	id, err := s.Studio.CreateStoryJob(studio.StoryParams{Topic: "chưa xong"})
	if err != nil {
		t.Fatal(err)
	}
	rec := postForm(t, s, "/stories/"+id+"/publish", url.Values{"account": {"kenh-x"}})
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("job chưa dựng xong phải bị chặn đăng")
	}
}

func TestStoriesDelete(t *testing.T) {
	s := newStoriesServer(t)
	id, err := s.Studio.CreateStoryJob(studio.StoryParams{Topic: "xoá tôi"})
	if err != nil {
		t.Fatal(err)
	}
	rec := postForm(t, s, "/stories/"+id+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST delete = %d, want 303", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "ok=") {
		t.Fatalf("Location=%q, want ?ok=", rec.Header().Get("Location"))
	}
	if _, ok := s.Studio.GetJob(id); ok {
		t.Fatal("job vẫn còn sau khi xoá")
	}
	// Xoá job không tồn tại → kèm ?err=.
	rec2 := postForm(t, s, "/stories/khong-co/delete", url.Values{})
	if !strings.Contains(rec2.Header().Get("Location"), "err=") {
		t.Fatal("xoá job không tồn tại phải fail-closed")
	}
}

func TestStoriesSettingsSaves(t *testing.T) {
	s := newStoriesServer(t)
	rec := postForm(t, s, "/stories/settings", url.Values{
		"words":          {"500"},
		"scenes":         {"5"},
		"genre":          {"kinh dị"},
		"interval_hours": {"12"},
		"music_on":       {"1"},
		"tick_on":        {"1"},
		"topics":         {"chủ đề 1\nchủ đề 2"},
	})
	if !strings.Contains(rec.Header().Get("Location"), "ok=") {
		t.Fatalf("Location=%q, want ?ok=", rec.Header().Get("Location"))
	}
	for key, want := range map[string]string{
		"story.words":  "500",
		"story.scenes": "5",
		"story.genre":  "kinh dị",
		"story.topics": "chủ đề 1\nchủ đề 2",
	} {
		v, ok, err := s.Ledger.GetSetting(key)
		if err != nil || !ok || v != want {
			t.Errorf("%s=%q ok=%v err=%v, want %q", key, v, ok, err, want)
		}
	}
	// auto_publish không check = tắt (mặc định chờ duyệt).
	if v, ok, _ := s.Ledger.GetSetting("story.auto_publish"); !ok || v != "0" {
		t.Errorf("story.auto_publish=%q ok=%v, want \"0\"", v, ok)
	}
	// Trang render lại phải giữ giá trị đã lưu (không rơi về mặc định).
	time.Sleep(50 * 1000000)
	rec2 := get(t, s, "/stories")
	if body := rec2.Body.String(); !strings.Contains(body, "chủ đề 1\nchủ đề 2") {
		t.Error("trang /stories không hiện chủ đề đã lưu")
	}
}

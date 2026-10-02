package web

// Reup Đợt E: test handler transform/post/file/settings + tab Cài đặt · Reup.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// queueDownloadedFake tạo video downloaded với file giả (transform nền sẽ
// fail — test chỉ kiểm tra handler fail-closed + tạo bài đúng).
func queueDownloadedFake(t *testing.T, s *Server, douyinID string) reup.Video {
	t.Helper()
	v, err := s.Reup.QueueVideo(reup.Video{DouyinID: douyinID, URL: "https://douyin/x", Title: "Clip " + douyinID})
	if err != nil {
		t.Fatal(err)
	}
	fp := filepath.Join(t.TempDir(), douyinID+".mp4")
	if err := os.WriteFile(fp, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Reup.MarkDownloaded(v.ID, fp, "sha-"+douyinID, "ytdlp", false, 5, "", "size-only"); err != nil {
		t.Fatal(err)
	}
	v, err = s.Reup.GetVideo(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReupPageTransformSection(t *testing.T) {
	s := newReupServer(t)
	rec := get(t, s, "/reup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reup = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Bài đăng (transform)", "giảm rủi ro", "Transform", "Kill"} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /reup thiếu %q", want)
		}
	}
}

func TestReupTransformNoStore(t *testing.T) {
	s := newTestServer(t) // Reup = nil
	rec := postJSON(t, s, "/reup/videos/1/transform", `{"level":1}`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("thiếu kho phải 412, được %d", rec.Code)
	}
}

func TestReupTransformNotDownloaded(t *testing.T) {
	s := newReupServer(t)
	v, err := s.Reup.QueueVideo(reup.Video{DouyinID: "q1", URL: "https://douyin/x"})
	if err != nil {
		t.Fatal(err)
	}
	rec := postJSON(t, s, "/reup/videos/1/transform", `{"level":1}`)
	_ = v
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("video chưa tải phải 412, được %d", rec.Code)
	}
}

func TestReupTransformStartsAndStatus(t *testing.T) {
	s := newReupServer(t)
	v := queueDownloadedFake(t, s, "t1")
	rec := postJSON(t, s, "/reup/videos/"+strconv.FormatInt(v.ID, 10)+"/transform", `{"level":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("transform phải 200, được %d: %s", rec.Code, rec.Body.String())
	}
	var d struct {
		OK     bool  `json:"ok"`
		PostID int64 `json:"post_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || !d.OK || d.PostID == 0 {
		t.Fatalf("response sai: %s", rec.Body.String())
	}
	// Poll status: transforming hoặc failed (file giả → fail nhanh).
	deadline := time.Now().Add(10 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		rec2 := get(t, s, "/reup/posts/"+strconv.FormatInt(d.PostID, 10)+"/status")
		if rec2.Code != http.StatusOK {
			t.Fatalf("status = %d", rec2.Code)
		}
		var sd struct {
			Status string `json:"status"`
			Label  string `json:"label"`
		}
		_ = json.Unmarshal(rec2.Body.Bytes(), &sd)
		status = sd.Status
		if status == reup.PostFailed || status == reup.PostTransformed {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if status != reup.PostFailed && status != reup.PostTransformed {
		t.Errorf("trạng thái cuối phải failed/transformed, được %q", status)
	}
	// File không tồn tại → 404.
	rec3 := get(t, s, "/reup/file/999999")
	if rec3.Code != http.StatusNotFound {
		t.Errorf("file lạ phải 404, được %d", rec3.Code)
	}
	rec4 := get(t, s, "/reup/videos/999999/file")
	if rec4.Code != http.StatusNotFound {
		t.Errorf("video lạ phải 404, được %d", rec4.Code)
	}
}

func TestReupPostPublishValidation(t *testing.T) {
	s := newReupServer(t)
	// Bài không tồn tại.
	rec := postForm(t, s, "/reup/posts/999999/publish", url.Values{"account": {"kenh1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish phải redirect, được %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Errorf("bài lạ phải redirect kèm err, được %q", loc)
	}
	// Thiếu kênh.
	v := queueDownloadedFake(t, s, "t2")
	p, err := s.Reup.CreatePost([]int64{v.ID}, reup.Level1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Reup.SetPostTransformed(p.ID, "/tmp/x.mp4")
	rec = postForm(t, s, "/reup/posts/"+strconv.FormatInt(p.ID, 10)+"/publish", url.Values{})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Errorf("thiếu kênh phải redirect kèm err, được %q", loc)
	}
}

func TestReupSettingsExtended(t *testing.T) {
	s := newReupServer(t)
	rec := postJSON(t, s, "/reup/settings",
		`{"transform_on":true,"transform_level":2,"voiceover_on":false,"videos_per_day":5,"kill_on":true,"kill_n":7,"warmup_on":true,"post_account":"kenh1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("lưu settings phải 200, được %d: %s", rec.Code, rec.Body.String())
	}
	if v := s.atSettingInt(automation.KeyReupTransformLevel, 1); v != 2 {
		t.Errorf("transform_level phải 2, được %d", v)
	}
	if s.atSettingOn(automation.KeyReupVoiceoverEnabled, true) {
		t.Errorf("voiceover_on=false phải lưu thành tắt")
	}
	if v := s.atSettingInt(automation.KeyReupVideosPerDay, 3); v != 5 {
		t.Errorf("videos_per_day phải 5, được %d", v)
	}
	if v := s.atSettingInt(automation.KeyReupKillZeroN, 5); v != 7 {
		t.Errorf("kill_n phải 7, được %d", v)
	}
	// kill_n vượt trần → cắt về 20.
	_ = postJSON(t, s, "/reup/settings", `{"kill_n":99}`)
	if v := s.atSettingInt(automation.KeyReupKillZeroN, 5); v != 20 {
		t.Errorf("kill_n phải cắt về 20, được %d", v)
	}
	// JSON sai → 400.
	rec = postJSON(t, s, "/reup/settings", "{invalid")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("JSON sai phải 400, được %d", rec.Code)
	}
}

func TestSettingsReupTab(t *testing.T) {
	s := newReupServer(t)
	rec := get(t, s, "/settings/reup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/reup = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Cài đặt · Reup", "Kill rule", "Warm-up", "giảm rủi ro", "không đảm bảo", "Quét nguồn"} {
		if !strings.Contains(body, want) {
			t.Errorf("tab reup thiếu %q", want)
		}
	}
	// Nav có tab Reup.
	if !strings.Contains(body, `href="/settings/reup"`) {
		t.Errorf("settings nav thiếu tab Reup")
	}
	// Lưu form.
	rec = postForm(t, s, "/settings/reup/save", url.Values{
		"transform_on":    {"1"},
		"transform_level": {"2"},
		"voiceover_on":    {"0"},
		"post_on":         {"1"},
		"videos_per_day":  {"4"},
		"post_account":    {"kenh1"},
		"kill_on":         {"1"},
		"kill_n":          {"6"},
		"warmup_on":       {"1"},
		"discover_on":     {"1"},
		"discover_hours":  {"12"},
		"videos_per_source": {"5"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("lưu form phải redirect, được %d", rec.Code)
	}
	if v := s.atSettingInt(automation.KeyReupTransformLevel, 1); v != 2 {
		t.Errorf("transform_level phải 2, được %d", v)
	}
	if v := s.atSettingInt(automation.KeyReupKillZeroN, 5); v != 6 {
		t.Errorf("kill_n phải 6, được %d", v)
	}
	if s.atSettingOn(automation.KeyReupVoiceoverEnabled, true) {
		t.Errorf("voiceover phải tắt")
	}
	// Đợt G: discover settings có UI + lưu đúng.
	if v := s.atSettingInt(automation.KeyReupDiscoverIntervalH, 6); v != 12 {
		t.Errorf("discover_hours phải 12, được %d", v)
	}
	if v := s.atSettingInt(automation.KeyReupVideosPerSource, 3); v != 5 {
		t.Errorf("videos_per_source phải 5, được %d", v)
	}
	if !s.atSettingOn(automation.KeyReupDiscoverEnabled, false) {
		t.Errorf("discover phải bật")
	}
}


func TestReupLevel2OptionOnlyWhenEnoughClips(t *testing.T) {
	s := newReupServer(t)
	src, err := s.Reup.AddSource("user", "than_tien", "Than Tien")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(douyinID string, sourceID int64) {
		v, err := s.Reup.QueueVideo(reup.Video{SourceID: sourceID, DouyinID: douyinID, URL: "https://douyin/" + douyinID})
		if err != nil {
			t.Fatal(err)
		}
		fp := filepath.Join(t.TempDir(), douyinID+".mp4")
		if err := os.WriteFile(fp, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := s.Reup.MarkDownloaded(v.ID, fp, "sha-"+douyinID, "ytdlp", true, 5, "", "size-only"); err != nil {
			t.Fatal(err)
		}
	}
	// 3 clip cùng nguồn → đủ Mức 2; 1 clip nguồn khác → chỉ Mức 1.
	mk("c1", src.ID)
	mk("c2", src.ID)
	mk("c3", src.ID)
	mk("solo", 9999)
	rec := get(t, s, "/reup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reup = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Count(body, "Mức 2 — 3 clip") != 3 {
		t.Errorf("3 video đủ clip phải có option Mức 2, đếm được %d", strings.Count(body, "Mức 2 — 3 clip"))
	}
	if !strings.Contains(body, "Mức 1</option>") {
		t.Errorf("mọi video downloaded phải có option Mức 1")
	}
}

func TestReupTransformDuplicateBlocked(t *testing.T) {
	s := newReupServer(t)
	v := queueDownloadedFake(t, s, "dup1")
	// Bài còn hiệu lực đã tồn tại → 412, không tạo trùng.
	if _, err := s.Reup.CreatePost([]int64{v.ID}, reup.Level1); err != nil {
		t.Fatal(err)
	}
	rec := postJSON(t, s, "/reup/videos/"+strconv.FormatInt(v.ID, 10)+"/transform", `{"level":1}`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("transform trùng phải 412, được %d: %s", rec.Code, rec.Body.String())
	}
	posts, err := s.Reup.ListPosts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Errorf("không được tạo bài trùng, có %d bài", len(posts))
	}
	// Bài failed → được transform lại.
	if err := s.Reup.SetPostStatus(posts[0].ID, reup.PostFailed, "lỗi test"); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(t, s, "/reup/videos/"+strconv.FormatInt(v.ID, 10)+"/transform", `{"level":1}`)
	if rec.Code != http.StatusOK {
		t.Errorf("bài failed được làm lại, phải 200, được %d: %s", rec.Code, rec.Body.String())
	}
}

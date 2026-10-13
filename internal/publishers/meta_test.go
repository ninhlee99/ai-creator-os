package publishers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMeta giả lập Graph/rupload/Threads: ghi lại path + body từng call.
func fakeMeta(t *testing.T, handle func(path string, r *http.Request) string) (calls *[]string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, handle(r.URL.Path, r))
	}))
	t.Cleanup(srv.Close)
	old := []string{metaGraphURL, metaVideoURL, metaRuploadURL, threadsURL}
	metaGraphURL, metaVideoURL, metaRuploadURL, threadsURL = srv.URL+"/g", srv.URL+"/v", srv.URL+"/r", srv.URL+"/t"
	oldEvery := igPollEvery
	igPollEvery = time.Millisecond
	t.Cleanup(func() {
		metaGraphURL, metaVideoURL, metaRuploadURL, threadsURL = old[0], old[1], old[2], old[3]
		igPollEvery = oldEvery
	})
	return &got
}

func tmpVideo(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "v.mp4")
	if err := os.WriteFile(p, []byte("fake-mp4-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFacebookGraphUpload(t *testing.T) {
	t.Setenv("FB_PAGE_ID", "123")
	t.Setenv("META_PAGE_TOKEN", "tok")
	var gotPublished, gotFile string
	fakeMeta(t, func(path string, r *http.Request) string {
		if path != "/v/123/videos" {
			t.Errorf("path = %s", path)
		}
		if err := r.ParseMultipartForm(1 << 20); err == nil {
			gotPublished = r.FormValue("published")
			if f, _, ferr := r.FormFile("source"); ferr == nil {
				b, _ := io.ReadAll(f)
				gotFile = string(b)
			}
		}
		return `{"id":"vid9"}`
	})
	p := NewFacebookPublisher("shop")
	if !p.IsConfigured() {
		t.Fatal("token + page id phải đủ cấu hình (không cần facebook-cli)")
	}
	res := p.Publish(context.Background(), tmpVideo(t), "Tiêu đề", "mô tả", "short_video")
	if !res.Ok || res.RemoteID != "vid9" || !res.Draft {
		t.Fatalf("res = %+v", res)
	}
	if gotPublished != "false" || gotFile != "fake-mp4-bytes" {
		t.Fatalf("published=%q file=%q (mặc định phải là nháp, gửi đúng file)", gotPublished, gotFile)
	}
}

func TestInstagramReelsDraftByDefault(t *testing.T) {
	t.Setenv("IG_USER_ID", "ig1")
	t.Setenv("META_PAGE_TOKEN", "tok")
	polls := 0
	calls := fakeMeta(t, func(path string, r *http.Request) string {
		switch {
		case path == "/g/ig1/media":
			if r.FormValue("media_type") != "REELS" || r.FormValue("upload_type") != "resumable" {
				t.Errorf("container form sai: %v", r.Form)
			}
			return `{"id":"c1","uri":"x"}`
		case path == "/r/c1":
			if r.Header.Get("Authorization") != "OAuth tok" || r.Header.Get("file_size") != "14" {
				t.Errorf("rupload header sai: %v", r.Header)
			}
			return `{"success":true}`
		case path == "/g/c1":
			polls++
			if polls < 2 {
				return `{"status_code":"IN_PROGRESS"}`
			}
			return `{"status_code":"FINISHED"}`
		}
		t.Errorf("call lạ: %s", path)
		return `{}`
	})
	res := NewInstagramPublisher("shop").Publish(context.Background(), tmpVideo(t), "T", "D", "short_video")
	if !res.Ok || !res.Draft || res.RemoteID != "c1" {
		t.Fatalf("res = %+v", res)
	}
	for _, c := range *calls {
		if strings.Contains(c, "media_publish") {
			t.Fatal("mặc định không được đăng công khai lên Instagram")
		}
	}
}

func TestInstagramAutoPublish(t *testing.T) {
	t.Setenv("IG_USER_ID", "ig1")
	t.Setenv("META_PAGE_TOKEN", "tok")
	t.Setenv("IG_AUTO_PUBLISH", "1")
	fakeMeta(t, func(path string, r *http.Request) string {
		switch path {
		case "/g/ig1/media":
			return `{"id":"c1"}`
		case "/g/c1":
			return `{"status_code":"FINISHED"}`
		case "/g/ig1/media_publish":
			return `{"id":"media7"}`
		}
		return `{"success":true}`
	})
	res := NewInstagramPublisher("shop").Publish(context.Background(), tmpVideo(t), "T", "D", "short_video")
	if !res.Ok || res.Draft || res.RemoteID != "media7" {
		t.Fatalf("res = %+v", res)
	}
}

func TestInstagramProcessingError(t *testing.T) {
	t.Setenv("IG_USER_ID", "ig1")
	t.Setenv("META_PAGE_TOKEN", "tok")
	fakeMeta(t, func(path string, r *http.Request) string {
		if path == "/g/c1" {
			return `{"status_code":"ERROR"}`
		}
		return `{"id":"c1"}`
	})
	res := NewInstagramPublisher("shop").Publish(context.Background(), tmpVideo(t), "T", "D", "short_video")
	if res.Ok || !strings.Contains(res.Error, "ERROR") {
		t.Fatalf("res = %+v", res)
	}
}

func TestThreadsTextPost(t *testing.T) {
	t.Setenv("THREADS_USER_ID", "th1")
	t.Setenv("THREADS_TOKEN", "tt")
	var text string
	fakeMeta(t, func(path string, r *http.Request) string {
		switch path {
		case "/t/th1/threads":
			text = r.FormValue("text")
			return `{"id":"tc"}`
		case "/t/th1/threads_publish":
			return `{"id":"post1"}`
		}
		return `{"error":{"message":"bad"}}`
	})
	res := NewThreadsPublisher("shop").Publish(context.Background(), "", "Túi đẹp", "Mua tại https://s.at/x", "short_video")
	if !res.Ok || res.RemoteID != "post1" {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(text, "https://s.at/x") {
		t.Fatalf("bài Threads phải giữ link: %q", text)
	}
}

func TestMetaAPIErrorSurfaces(t *testing.T) {
	t.Setenv("THREADS_USER_ID", "th1")
	t.Setenv("THREADS_TOKEN", "tt")
	fakeMeta(t, func(string, *http.Request) string {
		return `{"error":{"message":"Invalid OAuth access token"}}`
	})
	res := NewThreadsPublisher("shop").Publish(context.Background(), "", "a", "b", "short_video")
	if res.Ok || !strings.Contains(res.Error, "Invalid OAuth") {
		t.Fatalf("res = %+v", res)
	}
}

func TestFactoryIncludesInstagramThreads(t *testing.T) {
	t.Setenv("IG_USER_ID", "ig1")
	t.Setenv("META_PAGE_TOKEN", "tok")
	t.Setenv("THREADS_USER_ID", "th1")
	t.Setenv("THREADS_TOKEN", "tt")
	names := map[string]bool{}
	for _, p := range BuildPublishers("shop", "", nil, "short_video") {
		names[p.Name()] = true
	}
	if !names["instagram"] || !names["threads"] {
		t.Fatalf("factory thiếu IG/Threads: %v", names)
	}
}

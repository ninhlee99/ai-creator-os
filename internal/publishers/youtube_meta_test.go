package publishers

// Đợt M1: PublishMeta — tags trong snippet + thumbnail tùy chỉnh.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func metaTestPublisher(t *testing.T, thumbStatus int) (*YouTubePublisher, *map[string]any, *bool) {
	t.Helper()
	clearPublishEnv(t)
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
	t.Setenv("YOUTUBE_CLIENT_ID", "cid")
	t.Setenv("YOUTUBE_CLIENT_SECRET", "csec")
	if err := os.WriteFile("youtube_token_u.json", []byte(`{"refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("v.mp4", make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("thumb.jpg", []byte{0xFF, 0xD8, 0xFF, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	var gotInitBody map[string]any
	thumbCalled := false
	p := NewYouTubePublisher("u", []string{"short_film"}, "ch")
	p.HTTP = func(method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
		switch {
		case method == "POST" && strings.Contains(url, "oauth2.googleapis.com"):
			return 200, nil, []byte(`{"access_token":"ya29.test","expires_in":3600}`), nil
		case method == "POST" && strings.Contains(url, "thumbnails/set"):
			thumbCalled = true
			if !strings.Contains(url, "videoId=vidABC") {
				t.Fatalf("thumbnail URL thiếu videoId: %s", url)
			}
			if headers["Content-Type"] != "image/jpeg" {
				t.Fatalf("thumbnail Content-Type = %q", headers["Content-Type"])
			}
			return thumbStatus, nil, []byte(`{}`), nil
		case method == "POST":
			_ = json.Unmarshal(body, &gotInitBody)
			return 200, map[string]string{"Location": "https://upload.example/sess"}, []byte(`{}`), nil
		case method == "PUT":
			return 200, nil, []byte(`{"id":"vidABC"}`), nil
		}
		t.Fatalf("unexpected %s %s", method, url)
		return 0, nil, nil, nil
	}
	return p, &gotInitBody, &thumbCalled
}

func TestPublishMetaTagsAndThumbnail(t *testing.T) {
	p, gotInitBody, thumbCalled := metaTestPublisher(t, 200)
	res := p.PublishMeta(context.Background(), "v.mp4", VideoMeta{
		Title:         "Tiêu đề SEO",
		Description:   "Mô tả.",
		Tags:          []string{"kể chuyện", "truyện hay"},
		ThumbnailPath: "thumb.jpg",
	}, "short_film")
	if !res.Ok {
		t.Fatalf("publish failed: %+v", res)
	}
	snippet := (*gotInitBody)["snippet"].(map[string]any)
	tags, _ := snippet["tags"].([]any)
	if len(tags) != 2 || tags[0] != "kể chuyện" {
		t.Fatalf("tags = %v, want 2 tags", tags)
	}
	if !*thumbCalled {
		t.Fatal("phải gọi thumbnails/set")
	}
	if !res.ThumbnailSet || res.ThumbnailError != "" {
		t.Fatalf("ThumbnailSet=%v err=%q", res.ThumbnailSet, res.ThumbnailError)
	}
}

func TestPublishMetaThumbnailFailureKeepsVideo(t *testing.T) {
	p, _, thumbCalled := metaTestPublisher(t, 403) // kênh chưa xác minh
	res := p.PublishMeta(context.Background(), "v.mp4", VideoMeta{
		Title:         "T",
		ThumbnailPath: "thumb.jpg",
	}, "short_film")
	if !res.Ok {
		t.Fatal("lỗi thumbnail không được làm fail cả video")
	}
	if !*thumbCalled {
		t.Fatal("phải thử gọi thumbnails/set")
	}
	if res.ThumbnailSet || res.ThumbnailError == "" {
		t.Fatalf("ThumbnailSet=%v err=%q, want lỗi rõ ràng", res.ThumbnailSet, res.ThumbnailError)
	}
}

func TestPublishBackwardCompatible(t *testing.T) {
	p, gotInitBody, thumbCalled := metaTestPublisher(t, 200)
	res := p.Publish(context.Background(), "v.mp4", "T", "D", "short_film")
	if !res.Ok {
		t.Fatalf("publish failed: %+v", res)
	}
	snippet := (*gotInitBody)["snippet"].(map[string]any)
	if _, has := snippet["tags"]; has {
		t.Fatalf("Publish cũ không tags, got %v", snippet["tags"])
	}
	if *thumbCalled {
		t.Fatal("Publish cũ không gọi thumbnail")
	}
}

func TestUpdatePrivacy(t *testing.T) {
	p, _, _ := metaTestPublisher(t, 200)
	// ghi đè HTTP để bắt PUT videos
	var gotMethod, gotURL string
	var gotBody map[string]any
	p.HTTP = func(method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
		if strings.Contains(url, "oauth2.googleapis.com") {
			return 200, nil, []byte(`{"access_token":"ya29.test","expires_in":3600}`), nil
		}
		gotMethod, gotURL = method, url
		_ = json.Unmarshal(body, &gotBody)
		return 200, nil, []byte(`{}`), nil
	}
	if err := p.UpdatePrivacy("vidABC", "public"); err != nil {
		t.Fatalf("UpdatePrivacy: %v", err)
	}
	if gotMethod != "PUT" || !strings.Contains(gotURL, "youtube/v3/videos") {
		t.Fatalf("method=%q url=%q", gotMethod, gotURL)
	}
	if gotBody["id"] != "vidABC" {
		t.Fatalf("body id = %v", gotBody["id"])
	}
	status := gotBody["status"].(map[string]any)
	if status["privacyStatus"] != "public" {
		t.Fatalf("privacyStatus = %v", status["privacyStatus"])
	}
	if err := p.UpdatePrivacy("vidABC", "bogus"); err == nil {
		t.Fatal("privacy không hợp lệ phải trả lỗi")
	}
}

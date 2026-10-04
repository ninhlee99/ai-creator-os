package growth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testAnalyticsServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const analyticsReport = `{
  "columnHeaders": [{"name": "views"}, {"name": "estimatedMinutesWatched"}, {"name": "subscribersGained"}],
  "rows": [[12345, 6000, 42]]
}`

func TestYouTubeAnalyticsHappy(t *testing.T) {
	srv := testAnalyticsServer(t, 200, analyticsReport)
	defer srv.Close()
	y := &YouTubeAnalyticsSource{
		TokenFor: func(u string) (string, error) { return "tok123", nil },
		BaseURL:  srv.URL,
		Client:   srv.Client(),
	}
	snap, err := y.Fetch(context.Background(), SourceAccount{ID: 7, Username: "u", YoutubeChannel: "UCx"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if snap.Views30d == nil || *snap.Views30d != 12345 {
		t.Errorf("Views30d = %v, want 12345", snap.Views30d)
	}
	if snap.Extra["watch_hours_30d"] != 100.0 {
		t.Errorf("watch_hours_30d = %v, want 100", snap.Extra["watch_hours_30d"])
	}
	if snap.Extra["subs_gained_30d"] != 42.0 {
		t.Errorf("subs_gained_30d = %v, want 42", snap.Extra["subs_gained_30d"])
	}
	if y.Name() != "youtube_analytics" {
		t.Errorf("Name = %q", y.Name())
	}
}

func TestYouTubeAnalyticsFailClosed(t *testing.T) {
	y := &YouTubeAnalyticsSource{
		TokenFor: func(u string) (string, error) { return "tok123", nil },
	}
	// Chưa gắn kênh → ErrNotConnected, không gọi mạng.
	if _, err := y.Fetch(context.Background(), SourceAccount{Username: "u"}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("no channel: err = %v, want ErrNotConnected", err)
	}
	// Nil TokenFor → ErrNotConnected.
	y2 := &YouTubeAnalyticsSource{}
	if _, err := y2.Fetch(context.Background(), SourceAccount{Username: "u", YoutubeChannel: "UCx"}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("nil TokenFor: err = %v, want ErrNotConnected", err)
	}
	// Token lỗi → ErrNotConnected.
	y3 := &YouTubeAnalyticsSource{TokenFor: func(u string) (string, error) { return "", errors.New("nope") }}
	if _, err := y3.Fetch(context.Background(), SourceAccount{Username: "u", YoutubeChannel: "UCx"}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("token error: err = %v, want ErrNotConnected", err)
	}
}

func TestYouTubeAnalytics403Honest(t *testing.T) {
	srv := testAnalyticsServer(t, 403, `{"error": {"message": "insufficient permissions"}}`)
	defer srv.Close()
	y := &YouTubeAnalyticsSource{
		TokenFor: func(u string) (string, error) { return "tok123", nil },
		BaseURL:  srv.URL,
		Client:   srv.Client(),
	}
	_, err := y.Fetch(context.Background(), SourceAccount{Username: "u", YoutubeChannel: "UCx"})
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if got := err.Error(); !strings.Contains(got, "yt-analytics.readonly") {
		t.Errorf("403 error should name the missing scope, got: %q", got)
	}
}

func TestYouTubeAnalyticsEmptyRows(t *testing.T) {
	srv := testAnalyticsServer(t, 200, `{"columnHeaders": [], "rows": []}`)
	defer srv.Close()
	y := &YouTubeAnalyticsSource{
		TokenFor: func(u string) (string, error) { return "tok123", nil },
		BaseURL:  srv.URL,
		Client:   srv.Client(),
	}
	snap, err := y.Fetch(context.Background(), SourceAccount{ID: 7, Username: "u", YoutubeChannel: "UCx"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Không bịa số: không có row → Views30d nil.
	if snap.Views30d != nil {
		t.Errorf("Views30d = %v, want nil (no fabrication)", *snap.Views30d)
	}
}

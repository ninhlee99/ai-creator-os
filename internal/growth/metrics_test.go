package growth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYouTubeNotConnectedWithoutKey(t *testing.T) {
	src := NewYouTubeSource("")
	_, err := src.Fetch(context.Background(), SourceAccount{ID: 1, Username: "a", YoutubeChannel: "UC123"})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no key must be ErrNotConnected, got %v", err)
	}
}

func TestYouTubeNotConnectedWithoutChannel(t *testing.T) {
	src := NewYouTubeSource("key")
	_, err := src.Fetch(context.Background(), SourceAccount{ID: 1, Username: "a"})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no channel mapping must be ErrNotConnected, got %v", err)
	}
}

func TestTikTokNeverConnectedInMVP(t *testing.T) {
	_, err := TikTokSource{}.Fetch(context.Background(), SourceAccount{ID: 1, Username: "a"})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("TikTok MVP source must be ErrNotConnected, got %v", err)
	}
}

func TestYouTubeFetchParsesStatistics(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"statistics":{"viewCount":"123456","subscriberCount":"789","videoCount":"42"}}]}`))
	}))
	defer srv.Close()

	src := &YouTubeSource{APIKey: "test-key", BaseURL: srv.URL}
	snap, err := src.Fetch(context.Background(), SourceAccount{ID: 2, Username: "a", YoutubeChannel: "@kenhdep"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Followers == nil || *snap.Followers != 789 {
		t.Errorf("followers = %v, want 789", snap.Followers)
	}
	if snap.Videos == nil || *snap.Videos != 42 {
		t.Errorf("videos = %v, want 42", snap.Videos)
	}
	if snap.Views30d != nil {
		t.Error("Data API cannot give 30-day views; Views30d must stay nil (no invented numbers)")
	}
	if snap.Extra["views_total"] != int64(123456) {
		t.Errorf("extra views_total = %v, want 123456", snap.Extra["views_total"])
	}
	if gotQuery == "" || !strings.Contains(gotQuery, "forHandle=") {
		t.Errorf("handle lookups must use forHandle, query=%s", gotQuery)
	}
}

func TestYouTubeFetchHTTPErrorFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"quotaExceeded"}}`, http.StatusForbidden)
	}))
	defer srv.Close()
	src := &YouTubeSource{APIKey: "k", BaseURL: srv.URL}
	if _, err := src.Fetch(context.Background(), SourceAccount{ID: 1, YoutubeChannel: "UCx"}); err == nil {
		t.Fatal("HTTP 403 must surface an error, never a fake snapshot")
	}
}

func TestYouTubeUnknownChannelErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	src := &YouTubeSource{APIKey: "k", BaseURL: srv.URL}
	if _, err := src.Fetch(context.Background(), SourceAccount{ID: 1, YoutubeChannel: "UCnope"}); err == nil {
		t.Fatal("unknown channel must error")
	}
}

func TestRecordSnapshotPersistsOnlyOnSuccess(t *testing.T) {
	st := testStore(t)
	acct := SourceAccount{ID: 21, Username: "a", YoutubeChannel: "UCx"}

	// Failure path: nothing written.
	if _, err := RecordSnapshot(context.Background(), st, NewYouTubeSource(""), acct); err == nil {
		t.Fatal("expected error")
	}
	if latest, _ := st.LatestSnapshot(21); latest != nil {
		t.Fatal("failed fetch must not write a snapshot")
	}

	// Success path via fake transport.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"statistics":{"viewCount":"10","subscriberCount":"55","videoCount":"3"}}]}`))
	}))
	defer srv.Close()
	src := &YouTubeSource{APIKey: "k", BaseURL: srv.URL}
	snap, err := RecordSnapshot(context.Background(), st, src, acct)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source != "youtube_api" {
		t.Errorf("source = %q, want youtube_api", snap.Source)
	}
	latest, err := st.LatestSnapshot(21)
	if err != nil || latest == nil || latest.Followers == nil || *latest.Followers != 55 {
		t.Fatalf("stored snapshot = %+v, err %v", latest, err)
	}
}

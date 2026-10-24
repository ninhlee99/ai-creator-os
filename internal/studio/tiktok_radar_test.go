package studio

import (
	"context"
	"strings"
	"testing"
)

func TestFormatCount(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500"},
		{1500, "2K"},
		{25000, "25K"},
		{1500000, "1.5M"},
		{25700000, "25.7M"},
	}
	for _, tc := range tests {
		if got := FormatCount(tc.input); got != tc.want {
			t.Errorf("FormatCount(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseTikTokRadarFeedMapsVideoAndSoundFromProvider(t *testing.T) {
	feed := `{"code":0,"data":[
		{"video_id":"123","title":"Dance challenge","play_count":1200000,"digg_count":45000,"cover":"https://img.example/123.jpg","author":{"nickname":"Creator","unique_id":"creator"},"music_info":{"title":"Original dance sound"}},
		{"video_id":"","title":"discard"}
	]}`
	videos, err := parseTikTokRadarFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 {
		t.Fatalf("got %d videos, want 1 valid Radar result", len(videos))
	}
	got := videos[0]
	if got.Title != "Dance challenge" || got.Music != "Original dance sound" || got.PlayCount != 1200000 || got.DiggCount != 45000 {
		t.Fatalf("provider metadata not preserved: %+v", got)
	}
	if got.VideoURL != "https://www.tiktok.com/@creator/video/123" || got.Views != "1.2M" || got.Likes != "45K" {
		t.Fatalf("derived Radar fields incorrect: %+v", got)
	}
}

func TestParseTikTokRadarFeedReturnsProviderError(t *testing.T) {
	if _, err := parseTikTokRadarFeed(strings.NewReader(`{"code":12,"msg":"feed unavailable"}`)); err == nil || !strings.Contains(err.Error(), "feed unavailable") {
		t.Fatalf("error=%v, want provider message", err)
	}
}

func TestIsTikTokVideoURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.tiktok.com/@dance/video/123", true},
		{"https://vm.tiktok.com/ZTest/", true},
		{"http://www.tiktok.com/@dance/video/123", false},
		{"https://tiktok.com.evil.example/@dance/video/123", false},
		{"https://example.com/@dance/video/123", false},
	}
	for _, tc := range tests {
		if got := IsTikTokVideoURL(tc.url); got != tc.want {
			t.Errorf("IsTikTokVideoURL(%q) = %t, want %t", tc.url, got, tc.want)
		}
	}
}

func TestFindTikTokRadarVideoRequiresCollectedSource(t *testing.T) {
	videos := []TikTokTrendVideo{
		{VideoID: "123", Title: "Viral dance", Music: "Dance sound", VideoURL: "https://www.tiktok.com/@creator/video/123"},
		{VideoID: "456", Title: "No sound metadata", VideoURL: "https://www.tiktok.com/@creator/video/456"},
	}

	got, err := findTikTokRadarVideo(videos, "https://www.tiktok.com/@creator/video/123?lang=vi-VN")
	if err != nil || got.VideoID != "123" || got.Music != "Dance sound" {
		t.Fatalf("got (%+v, %v), want Radar video 123 and its sound", got, err)
	}
	if _, err := findTikTokRadarVideo(videos, "https://www.tiktok.com/@other/video/999"); err == nil {
		t.Fatal("accepted video missing from Radar")
	}
	if got, err := findTikTokRadarVideo(videos, "https://www.tiktok.com/@creator/video/456"); err != nil || got.VideoID != "456" {
		t.Fatalf("Radar source without title metadata should still be selectable: got=%+v err=%v", got, err)
	}
	if _, err := findTikTokRadarVideo(videos, "https://vm.tiktok.com/ZTest/"); err == nil {
		t.Fatal("accepted short link without a Radar video ID")
	}
}

func TestResolveTikTokRadarMusicRequiresOperatorSelection(t *testing.T) {
	if _, err := ResolveTikTokRadarMusic(context.Background()); err == nil || !strings.Contains(err.Error(), "chọn video nhảy thủ công") {
		t.Fatalf("ResolveTikTokRadarMusic error = %v, want manual Radar selection", err)
	}
}

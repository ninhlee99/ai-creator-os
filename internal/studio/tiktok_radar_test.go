package studio

import (
	"context"
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
		got := FormatCount(tc.input)
		if got != tc.want {
			t.Errorf("FormatCount(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFetchTikTokTrendingVideosGraceful(t *testing.T) {
	ctx := context.Background()
	videos, err := FetchTikTokTrendingVideos(ctx)
	if err != nil {
		t.Fatalf("FetchTikTokTrendingVideos returned error: %v", err)
	}
	if len(videos) == 0 {
		t.Errorf("FetchTikTokTrendingVideos returned 0 videos")
	}
}

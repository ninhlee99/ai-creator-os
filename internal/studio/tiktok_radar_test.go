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

func TestParseDramaItems(t *testing.T) {
	items := []rssItem{
		{
			Title:       "Huy Khánh sau ly hôn: 'Đến lúc ngừng hy vọng'",
			Link:        "https://kenh14.vn/huy-khanh-ly-hon.chn",
			Description: `<a href="#"><img src="https://example.com/huykhanh.jpg"></a> Huy Khánh chia sẻ về cuộc sống sau khi ly hôn.`,
			PubDate:     "Sat, 17 Oct 2026 06:21:00 +07",
		},
		{
			Title:       "Từ Nghệ Dương và Hoàng Tử Thao bí mật sinh con tại Mỹ",
			Link:        "https://kenh14.vn/tu-nghe-duong-hoang-tu-thao.chn",
			Description: `<a href="#"><img src="https://example.com/cbiz.jpg"></a> Cặp đôi showbiz Trung Quốc lộ diện hình ảnh con đầu lòng.`,
			PubDate:     "Sat, 17 Oct 2026 07:00:00 +07",
		},
	}

	dramas := parseDramaItems(items)
	if len(dramas) != 2 {
		t.Fatalf("dramas len = %d, want 2", len(dramas))
	}

	// Drama 1: Showbiz Việt
	if dramas[0].Region != "VN" || dramas[0].Category != "Showbiz Việt (V-Biz)" {
		t.Errorf("drama[0] category = %q, want Showbiz Việt", dramas[0].Category)
	}
	if dramas[0].ImageURL != "https://example.com/huykhanh.jpg" {
		t.Errorf("drama[0] image = %q, want https://example.com/huykhanh.jpg", dramas[0].ImageURL)
	}

	// Drama 2: Showbiz Trung
	if dramas[1].Region != "CN" || dramas[1].Category != "Showbiz Trung (C-Biz)" {
		t.Errorf("drama[1] category = %q, want Showbiz Trung", dramas[1].Category)
	}
	if dramas[1].ImageURL != "https://example.com/cbiz.jpg" {
		t.Errorf("drama[1] image = %q, want https://example.com/cbiz.jpg", dramas[1].ImageURL)
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

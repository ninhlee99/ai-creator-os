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

func TestTopicVideoGroups(t *testing.T) {
	// 1. Kiểm tra PresetTopicGroups: nhóm nhảy và sự kiện VN phải luôn có đúng 4 video
	nhayGroup, ok := PresetTopicGroups["nhay"]
	if !ok {
		t.Fatalf("PresetTopicGroups missing 'nhay'")
	}
	if len(nhayGroup.Videos) != 4 {
		t.Errorf("nhay group videos count = %d, want 4", len(nhayGroup.Videos))
	}
	for i, v := range nhayGroup.Videos {
		if v.Title == "" || v.UniqueID == "" || v.Views == "" {
			t.Errorf("nhay video [%d] missing essential info: %+v", i, v)
		}
	}

	suKienGroup, ok := PresetTopicGroups["su_kien_vn"]
	if !ok {
		t.Fatalf("PresetTopicGroups missing 'su_kien_vn'")
	}
	if len(suKienGroup.Videos) != 4 {
		t.Errorf("su_kien_vn group videos count = %d, want 4", len(suKienGroup.Videos))
	}

	// 2. Kiểm tra GetAllPresetTopicGroups
	presets := GetAllPresetTopicGroups()
	if len(presets) < 5 {
		t.Errorf("GetAllPresetTopicGroups() len = %d, want >= 5", len(presets))
	}

	// 3. Kiểm tra GetConfiguredTopicGroups với danh sách tùy chọn
	configured := GetConfiguredTopicGroups([]string{"nhay", "su_kien_vn", "Bóng Đá Việt Nam"})
	if len(configured) != 3 {
		t.Fatalf("GetConfiguredTopicGroups count = %d, want 3", len(configured))
	}
	if configured[0].TopicID != "nhay" || len(configured[0].Videos) != 4 {
		t.Errorf("configured[0] mismatch: %+v", configured[0])
	}
	if configured[1].TopicID != "su_kien_vn" || len(configured[1].Videos) != 4 {
		t.Errorf("configured[1] mismatch: %+v", configured[1])
	}
	// Custom topic
	customGrp := configured[2]
	if customGrp.TopicID != "Bóng Đá Việt Nam" || len(customGrp.Videos) != 4 {
		t.Errorf("customGrp mismatch: %+v", customGrp)
	}

	// 4. Kiểm tra GetVideosForCustomTopic
	customVids := GetVideosForCustomTopic("ẩm thực sài gòn")
	if len(customVids) != 4 {
		t.Fatalf("GetVideosForCustomTopic len = %d, want 4", len(customVids))
	}
	for i, cv := range customVids {
		if cv.Rank != i+1 {
			t.Errorf("custom video rank = %d, want %d", cv.Rank, i+1)
		}
		if cv.PlayURL == "" || cv.CoverURL == "" {
			t.Errorf("custom video [%d] missing media URLs", i)
		}
	}
}


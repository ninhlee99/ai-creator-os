package studio

import (
	"reflect"
	"testing"
)

func TestSplitTikTokRadarTopics(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{"nhảy, vũ đạo, dance", []string{"nhảy", "vũ đạo", "dance"}, false},
		{"  thời trang , , mỹ phẩm  ", []string{"thời trang", "mỹ phẩm"}, false},
		{"dance, DANCE, Dance", []string{"dance"}, false},
		{"", nil, true},
		{"   ", nil, true},
		{"a, b, c, d", nil, true}, // vượt quá 3 topics
	}

	for _, tc := range tests {
		got, err := splitTikTokRadarTopics(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("splitTikTokRadarTopics(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitTikTokRadarTopics(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeTikTokRadarTopic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#ĐiệuNhảy", "dieunhay"},
		{"điệu nhảy", "dieunhay"},
		{"dieu nhay", "dieunhay"},
		{"Thời Trang Nữ!", "thoitrangnu"},
		{"Đồ gia dụng", "dogiadung"},
		{"123 Dance", "123dance"},
	}

	for _, tc := range tests {
		if got := normalizeTikTokRadarTopic(tc.input); got != tc.want {
			t.Errorf("normalizeTikTokRadarTopic(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSelectTikTokBusinessHashtags(t *testing.T) {
	hashtags := []tikTokBusinessHashtag{
		{ID: "1", Name: "thoitrang", TopCountryList: []string{"VN"}},
		{ID: "2", Name: "dieunhay", TopCountryList: []string{"VN", "TH"}},
		{ID: "3", Name: "dancevn", TopCountryList: []string{"VN"}},
		{ID: "4", Name: "usonly", TopCountryList: []string{"US"}},
		{ID: "5", Name: "dogiadung", TopCountryList: nil}, // empty country list should be preserved
	}

	// 1. Tìm theo chủ đề có dấu tiếng Việt "điệu nhảy" khớp "#dieunhay"
	got := selectTikTokBusinessHashtags(hashtags, "điệu nhảy", "VN")
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("selectTikTokBusinessHashtags('điệu nhảy') = %+v, want hashtag 2", got)
	}

	// 2. Tìm theo chủ đề "dance" khớp "#dancevn"
	got = selectTikTokBusinessHashtags(hashtags, "dance", "VN")
	if len(got) != 1 || got[0].ID != "3" {
		t.Fatalf("selectTikTokBusinessHashtags('dance') = %+v, want hashtag 3", got)
	}

	// 3. Tìm chủ đề có chữ "đ" như "đồ gia dụng" khớp hashtag rỗng TopCountryList
	got = selectTikTokBusinessHashtags(hashtags, "đồ gia dụng", "VN")
	if len(got) != 1 || got[0].ID != "5" {
		t.Fatalf("selectTikTokBusinessHashtags('đồ gia dụng') = %+v, want hashtag 5", got)
	}

	// 4. Chủ đề rỗng -> trả về tất cả hashtag thuộc VN hoặc không bị giới hạn country list
	got = selectTikTokBusinessHashtags(hashtags, "", "VN")
	if len(got) != 4 {
		t.Fatalf("selectTikTokBusinessHashtags('') = %d hashtags, want 4 local hashtags", len(got))
	}
}

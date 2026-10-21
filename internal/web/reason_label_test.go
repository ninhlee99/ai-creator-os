package web

import "testing"

func TestReasonLabelVietnamese(t *testing.T) {
	cases := map[string]string{
		"topic resolved (source=fallback)":                                  "đã chốt chủ đề (nguồn: mặc định theo tính cách, vì chưa có AI)",
		"assigned 📖 Storyteller for researched niche 'kể chuyện đêm khuya'": "gán tính cách 📖 Storyteller cho chủ đề “kể chuyện đêm khuya”",
		"persona_assigned -> growing":                                       "Đã gán persona → Đang tăng trưởng",
		"đã chốt chủ đề (nguồn: hint)":                                      "đã chốt chủ đề (nguồn: gợi ý của bạn)",
	}
	for in, want := range cases {
		if got := reasonLabel(in); got != want {
			t.Errorf("reasonLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

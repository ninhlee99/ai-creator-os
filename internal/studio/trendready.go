package studio

// Điểm "sẵn sàng lên xu hướng" cho video affiliate đã dựng xong.
//
// KHÔNG phải dự đoán viral — không ai đảm bảo được thuật toán. Đây là
// checklist các yếu tố phân phối đã biết (khổ dọc, độ dài Shorts/TikTok,
// có âm thanh, dùng nhạc đang thịnh hành, caption có hashtag + lời kêu
// gọi, có link kiếm tiền), đo từ file và tham số thật.

import (
	"context"
	"strings"
)

// TrendCheck là một tiêu chí của checklist.
type TrendCheck struct {
	Label string `json:"label"`
	Ok    bool   `json:"ok"`
	Tip   string `json:"tip,omitempty"` // cách sửa khi chưa đạt
}

// TrendReport là kết quả chấm: Score/Total tiêu chí đạt.
type TrendReport struct {
	Score  int          `json:"score"`
	Total  int          `json:"total"`
	Checks []TrendCheck `json:"checks"`
}

// TrendReadiness chấm video tại path với tham số + caption của job.
func TrendReadiness(ctx context.Context, path string, p AffiliateParams, caption string) TrendReport {
	w, h := ProbeDims(ctx, path)
	dur := storyProbeDuration(ctx, path)
	checks := []TrendCheck{
		{Label: "Khổ dọc 9:16 (1080×1920)", Ok: w == 1080 && h == 1920,
			Tip: "Tạo lại ở khổ dọc — TikTok/Reels/Shorts ưu tiên video dọc."},
		{Label: "Độ dài 7–35 giây", Ok: dur >= 7 && dur <= 35,
			Tip: "Video ngắn xem hết nhiều hơn — đặt thời lượng 15–30s ở Tùy chọn nâng cao."},
		{Label: "Có âm thanh", Ok: ProbeHasAudio(ctx, path),
			Tip: "Tải file nhạc lên — video câm bị phân phối kém."},
		{Label: "Gắn bài nhạc thịnh hành (đã chọn bài + có file nhạc)",
			Ok:  strings.TrimSpace(p.MusicTitle) != "" && strings.TrimSpace(p.MusicPath) != "",
			Tip: "Chọn 1 bài trong bảng Nhạc thịnh hành và tải đúng file bài đó lên (app không tự nghe để đối chiếu bài)."},
		{Label: "Caption có hashtag", Ok: strings.Contains(caption, "#"),
			Tip: "Thêm 3–5 hashtag đúng chủ đề vào caption."},
		{Label: "Caption có lời kêu gọi (link/mua/bio)", Ok: hasCTA(caption),
			Tip: "Thêm câu kêu gọi hành động, ví dụ “Link mua ở mô tả”."},
		{Label: "Có link affiliate để ra tiền", Ok: strings.TrimSpace(p.AffLink) != "",
			Tip: "Nhập mã Accesstrade để app tự tạo link theo dõi hoa hồng."},
	}
	r := TrendReport{Total: len(checks), Checks: checks}
	for _, c := range checks {
		if c.Ok {
			r.Score++
		}
	}
	return r
}

func hasCTA(caption string) bool {
	c := strings.ToLower(caption)
	for _, k := range []string{"link", "mua", "bio", "mô tả"} {
		if strings.Contains(c, k) {
			return true
		}
	}
	return false
}

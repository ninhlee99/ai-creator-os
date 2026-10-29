package studio

// Điểm "sẵn sàng lên xu hướng" cho video affiliate đã dựng xong.
//
// KHÔNG phải dự đoán viral — không ai đảm bảo được thuật toán. Đây là
// checklist các yếu tố đo được từ file và tham số thật (khổ dọc, thời lượng,
// âm thanh, caption, lời kêu gọi và link kiếm tiền).

import (
	"context"
	"fmt"
	"os/exec"
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
	dur := mediaDuration(ctx, path)
	checks := []TrendCheck{
		{Label: "Khổ dọc 9:16 (1080×1920)", Ok: w == 1080 && h == 1920,
			Tip: "Tạo lại ở khổ dọc 9:16 để khớp định dạng video TikTok."},
		{Label: "Độ dài 7–35 giây", Ok: dur >= 7 && dur <= 35,
			Tip: "Video ngắn xem hết nhiều hơn — đặt thời lượng 15–30s ở Tùy chọn nâng cao."},
		{Label: "Có sound tham chiếu từ Radar", Ok: strings.TrimSpace(p.SourceMusicTitle) != "" && strings.TrimSpace(p.SourceMusicVideoURL) != "",
			Tip: "Studio tách âm thanh gốc từ video nhảy Radar và ghép vào affiliate."},
		{Label: "Caption có hashtag", Ok: strings.Contains(caption, "#"),
			Tip: "Thêm 3–5 hashtag đúng chủ đề vào caption."},
		{Label: "Caption có lời kêu gọi (link/mua/bio)", Ok: hasCTA(caption),
			Tip: "Thêm câu kêu gọi hành động, ví dụ “Link mua ở mô tả”."},
		{Label: "Có link affiliate để ra tiền", Ok: strings.TrimSpace(p.AffLink) != "",
			Tip: "Nhập link affiliate hoặc chọn sản phẩm từ kệ hàng để theo dõi hoa hồng."},
	}
	r := TrendReport{Total: len(checks), Checks: checks}
	for _, c := range checks {
		if c.Ok {
			r.Score++
		}
	}
	return r
}

func mediaDuration(ctx context.Context, path string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var duration float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &duration); err != nil || duration < 0 {
		return 0
	}
	return duration
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

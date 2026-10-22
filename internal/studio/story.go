package studio

import (
	"context"
	"fmt"
	"strings"
)

// KindStory là job kind cho pipeline kể chuyện.

// StoryLogTail trả về dòng log cuối cùng của job — cho bảng /stories
// (ListJobs không select cột log để nhẹ).
func (s *Studio) StoryLogTail(id string) string {
	var lg string
	if err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&lg); err != nil {
		return ""
	}
	lg = strings.TrimSpace(lg)
	if lg == "" {
		return ""
	}
	lines := strings.Split(lg, "\n")
	return lines[len(lines)-1]
}

// StoryLogFull trả về toàn bộ log của job (cho trang Nhật ký, đợt Y).
func (s *Studio) StoryLogFull(id string) string {
	var lg string
	if err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&lg); err != nil {
		return ""
	}
	return lg
}

// DeleteStoryJob xoá 1 job kể chuyện: assets, file output, thư mục work,
// các row DB. Chỉ xoá job kind=story (không đụng job kind khác).
func (s *Studio) DeleteStoryJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s", id)
	}
	if j.Kind != KindStory {
		return fmt.Errorf("job %s không phải truyện kể", id)
	}
	return s.DeleteJob(id)
}

// CreateStoryJob kiểm tra tham số, xếp job kể chuyện và chạy nền.
func (s *Studio) CreateStoryJob(p StoryParams) (string, error) {
	if strings.TrimSpace(p.Topic) == "" {
		return "", fmt.Errorf("cần chủ đề truyện")
	}

	if p.DurationMinutes <= 0 {
		p.DurationMinutes = 30 // Mặc định 30 phút
	}
	if p.DurationMinutes < 30 {
		p.DurationMinutes = 30 // Thời lượng tối thiểu từ 30 phút đến 60 phút
	}
	if p.DurationMinutes > 60 {
		p.DurationMinutes = 60 // Tối đa 60 phút
	}

	// Tốc độ audio YouTube chuẩn: ~150 từ/phút
	calcWords := p.DurationMinutes * 150
	if p.Words < 200 || p.Words < calcWords {
		p.Words = calcWords
	}
	if p.Words > 15000 {
		p.Words = 15000
	}

	// Số cảnh minh họa 16:9 với chuyển cảnh Ken Burns
	if p.Scenes < 3 {
		p.Scenes = p.DurationMinutes / 3
		if p.Scenes < 6 {
			p.Scenes = 6
		}
	}
	if p.Scenes > 24 {
		p.Scenes = 24
	}

	title := "Kể chuyện: " + strings.TrimSpace(p.Topic)
	if p.DurationMinutes > 0 {
		title = fmt.Sprintf("Kể chuyện [%dp]: %s", p.DurationMinutes, strings.TrimSpace(p.Topic))
	}
	if r := []rune(title); len(r) > 85 {
		title = string(r[:85]) + "…"
	}
	id, err := s.insertJob(KindStory, title, p)
	if err != nil {
		return "", err
	}
	go s.runStory(id, p)
	return id, nil
}

// GenerateTopics nhờ LLM nghĩ ra n chủ đề truyện mới theo thể loại.
// Dùng để tự refill hàng đợi khi Ninh không nhập tay (zero-touch).
// Nil LLM → lỗi rõ ràng (caller giữ note trung thực, không bịa chủ đề).
func (s *Studio) GenerateTopics(ctx context.Context, genre string, n int) ([]string, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để nghĩ chủ đề (chưa cấu hình)")
	}
	if n < 1 {
		n = 5
	}
	if n > 20 {
		n = 20
	}
	genre = strings.TrimSpace(genre)
	if genre == "" {
		genre = "tâm lý"
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là biên tập truyện Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		fmt.Sprintf(`Nghĩ ra đúng %d chủ đề truyện ngắn ngôi thứ nhất, thể loại %s, `+
			`hợp gu khán giả Việt Nam xem YouTube (đời thường, cảm xúc, kịch tính vừa phải, `+
			`không kinh dị máu me, không chính trị). Mỗi chủ đề 1 dòng ngắn gọn, cụ thể, `+
			`khác nhau, không trùng lặp ý tưởng.\n\nChỉ trả JSON: {"topics": ["...", ...]}`,
			n, genre))
	if err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var raw struct {
		Topics []string `json:"topics"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var out []string
	for _, t := range raw.Topics {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nghĩ chủ đề: LLM không trả chủ đề nào")
	}
	return out, nil
}

// StoryJobs trả về các job kể chuyện mới nhất.
func (s *Studio) StoryJobs(limit int) []Job {
	var out []Job
	for _, j := range s.ListJobs(limit * 3) {
		if j.Kind == KindStory {
			out = append(out, j)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

package studio

// SEO cho video truyện đăng YouTube (Đợt M1).
//
// Trước đây PublishStoryNow đăng với title = tên chủ đề thô, description
// 1 dòng + disclosure, không tags, không thumbnail — video private thì
// không sao, nhưng khi Ninh chuyển public thì metadata yếu làm giảm khả
// năng được đề xuất. GenerateSEOMeta nhờ LLM viết title/description/tags
// tiếng Việt; thiếu LLM hoặc parse lỗi → trả lỗi để caller dùng template
// trung thực (không bịa metadata "tối ưu" giả).

import (
	"context"
	"fmt"
	"strings"
)

// SEOMeta là metadata đăng YouTube cho 1 video truyện.
type SEOMeta struct {
	Title       string
	Description string
	Tags        []string
}

// GenerateSEOMeta sinh metadata YouTube từ chủ đề + thể loại.
// title/description/tags đều tiếng Việt, title ≤100 ký tự (giới hạn YouTube).
func (s *Studio) GenerateSEOMeta(ctx context.Context, topic, genre, fallbackTitle string) (SEOMeta, error) {
	if s.llm == nil {
		return SEOMeta{}, fmt.Errorf("cần LLM để viết SEO (chưa cấu hình)")
	}
	genre = strings.TrimSpace(genre)
	if genre == "" {
		genre = "tâm lý"
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = strings.TrimSpace(fallbackTitle)
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là chuyên gia SEO YouTube Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		fmt.Sprintf(`Viết metadata YouTube cho video kể chuyện ngôi thứ nhất, thể loại %s, chủ đề: %q.

Yêu cầu:
- title: tiếng Việt, tối đa 90 ký tự, gợi tò mò nhưng KHÔNG giật tít sai sự thật, có từ khóa chính.
- description: 2-3 câu tóm tắt hấp dẫn + 1 dòng kêu gọi đăng ký kênh, tiếng Việt.
- tags: 5-8 từ khóa tiếng Việt, không dấu #.

Chỉ trả JSON: {"title": "...", "description": "...", "tags": ["...", ...]}`,
			genre, topic))
	if err != nil {
		return SEOMeta{}, fmt.Errorf("viết SEO: %w", err)
	}
	var raw struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return SEOMeta{}, fmt.Errorf("viết SEO: %w", err)
	}
	m := SEOMeta{
		Title:       strings.TrimSpace(raw.Title),
		Description: strings.TrimSpace(raw.Description),
	}
	if m.Title == "" {
		m.Title = strings.TrimSpace(fallbackTitle)
	}
	// YouTube giới hạn title 100 ký tự — cắt an toàn theo rune.
	m.Title = truncateRunes(m.Title, 100)
	for _, t := range raw.Tags {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t != "" && len(m.Tags) < 10 {
			m.Tags = append(m.Tags, truncateRunes(t, 60))
		}
	}
	return m, nil
}

// TemplateSEOMeta là fallback trung thực khi không có LLM: không hứa hẹn
// "tối ưu", chỉ dùng tên truyện + thể loại.
func TemplateSEOMeta(topic, genre, disclosure string) SEOMeta {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Kể chuyện"
	}
	title := truncateRunes(topic+" | Truyện kể", 100)
	tags := []string{"kể chuyện", "truyện hay", "truyện việt nam"}
	if g := strings.TrimSpace(genre); g != "" {
		tags = append(tags, "truyện "+g)
	}
	desc := topic + "\n\nĐăng ký kênh để nghe thêm nhiều câu chuyện khác.\n\n" + disclosure
	return SEOMeta{Title: title, Description: desc, Tags: tags}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

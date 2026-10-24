package growth

import (
	"fmt"
	"sort"
	"strings"
)

// VariantSeconds returns target TikTok video length for organic content.
func VariantSeconds(string) int { return 60 }

// VariantAspect keeps all TikTok output vertical.
func VariantAspect(string) string { return "9:16" }

// VariantKind returns the TikTok video kind.
func VariantKind(string) string { return "short_video" }

// hashtagsFor derives tags from the topic and includes AI disclosure tags.
func hashtagsFor(topic string) string {
	toks := TokensOf(topic)
	tags := []string{"#ai", "#aivideo", "#xuhuong"}
	keys := make([]string, 0, len(toks))
	for token := range toks {
		if len(token) >= 4 {
			keys = append(keys, token)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for i, key := range keys {
		if i >= 3 {
			break
		}
		tags = append(tags, "#"+key)
	}
	return strings.Join(tags, " ")
}

// VariantTitle returns the TikTok title for a plan item.
func VariantTitle(it PlanItem) string {
	topic := strings.TrimSpace(it.Topic)
	if topic == "" {
		topic = strings.TrimSpace(it.FormatID)
	}
	return topic
}

// VariantCaption returns the TikTok caption base.
func VariantCaption(it PlanItem) string {
	topic := strings.TrimSpace(it.Topic)
	ep := ""
	if it.SeriesEp > 0 {
		ep = fmt.Sprintf(" (tập %d)", it.SeriesEp)
	}
	return fmt.Sprintf("%s%s %s", topic, ep, hashtagsFor(topic))
}

const DisclosureLine = "Nội dung được tạo/hỗ trợ bởi AI (synthetic content)."

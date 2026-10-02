package growth

import (
	"fmt"
	"sort"
	"strings"
)

// ------------------------------------------------- production variants
//
// One idea, several platform variants (docs §5.2). Every variant gets its
// own Studio job (never the same file twice), its own title/caption set,
// and a staggered schedule: TikTok first, the Short 24h later, the
// long-form after 72h so the Short's hook is proven before the long cut
// is produced and published.

// VariantLagDays is the schedule stagger per variant, in days after the
// plan date.
func VariantLagDays(variant string) int {
	switch variant {
	case VariantShorts:
		return 1
	case VariantYouTube:
		return 3
	default: // tiktok
		return 0
	}
}

// VariantSeconds is the target render length for the persona (film) path.
// Fashion/product accounts render the fixed 30s affiliate format instead.
func VariantSeconds(variant string) int {
	switch variant {
	case VariantShorts:
		return 45
	case VariantYouTube:
		return 150
	default: // tiktok
		return 60
	}
}

// VariantAspect is the delivery frame for a plan variant. Ninh decided
// 2026-10-02 (final): EVERY film is 16:9 — the cinematic standard,
// including the YouTube long-form cut. Vertical 9:16 trailers are cut
// automatically from trailer_worthy shots by center-crop (see
// CropCenterVertical); nothing is ever re-shot. Shorts/TikTok variants
// stay 9:16 (they are trailers, not films).
func VariantAspect(variant string) string {
	if variant == VariantYouTube {
		return "16:9"
	}
	return "9:16"
}

// VariantKind maps a plan variant to the publishers content kind used for
// channel gating and YouTube category selection.
func VariantKind(variant string) string {
	if variant == VariantYouTube {
		return "short_film"
	}
	return "short_video"
}

// hashtagsFor derives a small tag set from the topic tokens (the niche
// keywords live with the persona, not the plan; topic keywords are the
// honest signal available here). Always includes the AI-disclosure tag.
func hashtagsFor(topic string) string {
	toks := TokensOf(topic)
	var tags []string
	for _, t := range []string{"ai", "aivideo", "xuhuong"} {
		tags = append(tags, "#"+t)
	}
	seen := map[string]bool{}
	// Stable order: longest tokens first reads like keywords, not noise.
	keys := make([]string, 0, len(toks))
	for t := range toks {
		if len(t) >= 4 && !seen[t] {
			seen[t] = true
			keys = append(keys, t)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for i, k := range keys {
		if i >= 3 {
			break
		}
		tags = append(tags, "#"+k)
	}
	return strings.Join(tags, " ")
}

// VariantTitle is the per-platform title decided at production time. The
// same concept deliberately reads differently per surface (doc §5.2).
func VariantTitle(it PlanItem) string {
	topic := strings.TrimSpace(it.Topic)
	if topic == "" {
		topic = strings.TrimSpace(it.FormatID)
	}
	switch it.Variant {
	case VariantShorts:
		return topic + " #Shorts"
	case VariantYouTube:
		return topic + " — bản đầy đủ"
	default: // tiktok
		return topic
	}
}

// VariantCaption is the per-platform caption/description base decided at
// production time. The publisher appends the synthetic-content disclosure
// line and any sibling (Related Video) link at upload time.
func VariantCaption(it PlanItem) string {
	topic := strings.TrimSpace(it.Topic)
	ep := ""
	if it.SeriesEp > 0 {
		ep = fmt.Sprintf(" (tập %d)", it.SeriesEp)
	}
	switch it.Variant {
	case VariantShorts:
		return fmt.Sprintf("%s%s — bản Shorts. %s", topic, ep, hashtagsFor(topic))
	case VariantYouTube:
		return fmt.Sprintf("%s%s — phiên bản đầy đủ.\n%s", topic, ep, hashtagsFor(topic))
	default: // tiktok
		return fmt.Sprintf("%s%s %s", topic, ep, hashtagsFor(topic))
	}
}

// DisclosureLine is the synthetic-content disclosure appended to every
// published description (docs §3.6: the flag is mandatory, never off).
const DisclosureLine = "Nội dung được tạo/hỗ trợ bởi AI (synthetic content)."

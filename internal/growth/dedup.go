package growth

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ---------------------------------------------------------------- dedup
//
// Anti-duplicate guard (docs/CHANNEL_GROWTH.md §3.1/§5.2): the same idea
// must never go out as the same script on two accounts. Adaptations across
// platforms for ONE account are a designed funnel, so every plan item
// carries a variant group; the guard compares the *concept* (format family
// + topic, episode suffix stripped) of a candidate against everything the
// network already produced, and when it is too close to another account's
// concept it forces a different angle instead of blocking production.

// DedupSimilarityThreshold is the Jaccard token-set similarity at or above
// which two concepts count as "the same script idea".
const DedupSimilarityThreshold = 0.8

// episodeSuffix marks the series-episode tail GeneratePlan appends
// ("<pillar> — tập 3" / "<pillar> — bản dài tập 3"). The concept of a
// series is its family, not the episode number.
const episodeSuffix = " — tập "

// ConceptOf returns the comparable concept text for an item: format family
// plus topic with the series-episode suffix stripped.
func ConceptOf(formatID, topic string) string {
	t := strings.TrimSpace(topic)
	// Cut at the earliest series tail marker (" — tập 3", " — bản dài...").
	cut := len(t)
	for _, marker := range []string{episodeSuffix, " — bản dài"} {
		if i := strings.Index(t, marker); i > 0 && i < cut {
			cut = i
		}
	}
	t = strings.TrimSpace(t[:cut])
	return strings.TrimSpace(formatID + " " + t)
}

// TokensOf normalizes free text into a comparable token set: lowercase,
// Vietnamese diacritics folded, split on anything that is not a letter or
// digit, one-character tokens dropped as noise.
func TokensOf(text string) map[string]bool {
	out := map[string]bool{}
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 2 {
			out[cur.String()] = true
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(text) {
		if f, ok := viFold[r]; ok {
			r = f
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// ConceptHash fingerprints a concept: sha256 over its sorted token set.
// Two concepts with the same hash have identical token sets.
func ConceptHash(concept string) string {
	toks := TokensOf(concept)
	sorted := make([]string, 0, len(toks))
	for t := range toks {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, " ")))
	return hex.EncodeToString(sum[:])
}

// TokenSimilarity is the Jaccard index of two token sets (0..1). An empty
// set never matches anything (similarity 0).
func TokenSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// angles are the deterministic re-generation angles applied when the dedup
// guard fires: same format family, genuinely different treatment, so the
// Studio director writes a different script instead of a near-copy.
var angles = []string{
	"góc nhìn người mới bắt đầu",
	"so sánh trước và sau",
	"3 lỗi thường gặp cần tránh",
	"trải nghiệm thực tế một tuần",
	"hỏi đáp nhanh các thắc mắc thường gặp",
	"hậu trường ít người biết",
	"xếp hạng từ tệ nhất đến tốt nhất",
	"thử thách 7 ngày liên tiếp",
}

// AngleFor picks the re-generation angle deterministically from the item
// id and the attempt count, so retries keep moving to a fresh angle
// instead of stacking the same suffix.
func AngleFor(itemID int64, attempt int) string {
	n := int(itemID) + attempt
	if n < 0 {
		n = -n
	}
	return angles[n%len(angles)]
}

// ApplyAngle rewrites a topic with a new angle. The episode suffix (if
// any) is dropped first: after a re-generation the item is a standalone
// treatment, not an episode of the colliding series.
func ApplyAngle(topic, angle string) string {
	t := strings.TrimSpace(topic)
	cut := len(t)
	for _, marker := range []string{episodeSuffix, " — bản dài"} {
		if i := strings.Index(t, marker); i > 0 && i < cut {
			cut = i
		}
	}
	t = strings.TrimSpace(t[:cut])
	if t == "" {
		t = "nội dung"
	}
	return t + " — " + angle
}

// SameLine reports whether two items belong to the same exempt lineage:
// variants of one variant group (cross-platform adaptations of one idea),
// or the same account's own series line (same account + same format
// family — Studio writes a fresh script per episode). Everything else the
// dedup guard compares.
func SameLine(a, b PlanItem) bool {
	if a.VariantGroup != "" && a.VariantGroup == b.VariantGroup && a.AccountID == b.AccountID {
		return true
	}
	return a.AccountID == b.AccountID && a.FormatID == b.FormatID
}

// DecideDedup reports whether a candidate concept collides with recent
// network production (already lineage-filtered token sets). A similarity
// at or above the threshold counts as "the same script idea". Returns
// (collision, best similarity seen).
func DecideDedup(candidate map[string]bool, recents []map[string]bool) (bool, float64) {
	best := 0.0
	for _, r := range recents {
		if s := TokenSimilarity(candidate, r); s > best {
			best = s
		}
	}
	return best >= DedupSimilarityThreshold, best
}

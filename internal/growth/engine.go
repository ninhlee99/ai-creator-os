package growth

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------- targets

// DefaultTargets returns follower milestones for a TikTok channel.
func DefaultTargets() []Target {
	return []Target{
		{Metric: "followers", Threshold: 100, Label: "Mốc 100 follower"},
		{Metric: "followers", Threshold: 1000, Label: "Mốc 1.000 follower"},
		{Metric: "followers", Threshold: 10000, Label: "Mốc 10.000 follower"},
	}
}

// ---------------------------------------------------------------- slugify

var viFold = map[rune]rune{
	'à': 'a', 'á': 'a', 'ả': 'a', 'ã': 'a', 'ạ': 'a',
	'ă': 'a', 'ắ': 'a', 'ằ': 'a', 'ẳ': 'a', 'ẵ': 'a', 'ặ': 'a',
	'â': 'a', 'ấ': 'a', 'ầ': 'a', 'ẩ': 'a', 'ẫ': 'a', 'ậ': 'a',
	'è': 'e', 'é': 'e', 'ẻ': 'e', 'ẽ': 'e', 'ẹ': 'e',
	'ê': 'e', 'ế': 'e', 'ề': 'e', 'ể': 'e', 'ễ': 'e', 'ệ': 'e',
	'ì': 'i', 'í': 'i', 'ỉ': 'i', 'ĩ': 'i', 'ị': 'i',
	'ò': 'o', 'ó': 'o', 'ỏ': 'o', 'õ': 'o', 'ọ': 'o',
	'ô': 'o', 'ố': 'o', 'ồ': 'o', 'ổ': 'o', 'ỗ': 'o', 'ộ': 'o',
	'ơ': 'o', 'ớ': 'o', 'ờ': 'o', 'ở': 'o', 'ỡ': 'o', 'ợ': 'o',
	'ù': 'u', 'ú': 'u', 'ủ': 'u', 'ũ': 'u', 'ụ': 'u',
	'ư': 'u', 'ứ': 'u', 'ừ': 'u', 'ử': 'u', 'ữ': 'u', 'ự': 'u',
	'ỳ': 'y', 'ý': 'y', 'ỷ': 'y', 'ỹ': 'y', 'ỵ': 'y',
	'đ': 'd',
}

// Slugify turns a persona content pillar ("truyện ma") into a stable
// format_id ("truyen_ma"). Empty/invalid input falls back to "format".
func Slugify(s string) string {
	var b strings.Builder
	lastUnderscore := true // trims leading separators
	for _, r := range strings.ToLower(s) {
		if f, ok := viFold[r]; ok {
			r = f
		}
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "format"
	}
	return out
}

// ------------------------------------------------------------------- plan

// PlanRequest describes one 30-day TikTok plan generation.
type PlanRequest struct {
	Stage         string
	Pillars       []string
	Start         time.Time
	Days          int
	KilledFormats []string
}

// PlanDraft is one item GeneratePlan wants scheduled.
type PlanDraft struct {
	Date     string
	FormatID string
	Variant  string
	Topic    string
	Hook     string
	SeriesEp int
	Group    string
}

var hookByVariant = map[string]string{
	VariantTikTok: "Hook 3 giây đầu: nêu kết quả hoặc câu hỏi gây tò mò, payoff ở cuối video.",
}

func dailyCadence(stage string, dayIndex int) int {
	if stage == StageColdStart && dayIndex < CanaryDays {
		return 1
	}
	switch stage {
	case StageScaling, StageMonetizationPush:
		return 2
	default:
		return 1
	}
}

// GeneratePlan builds a deterministic TikTok plan. Pillars rotate daily;
// scaling stages may schedule two pieces per day.
func GeneratePlan(req PlanRequest) []PlanDraft {
	pillars := req.Pillars
	if len(pillars) == 0 {
		pillars = []string{"nội dung ngắn"}
	}
	killed := map[string]bool{}
	for _, k := range req.KilledFormats {
		killed[k] = true
	}
	var live []string
	for _, p := range pillars {
		if !killed[Slugify(p)] {
			live = append(live, p)
		}
	}
	if len(live) > 0 {
		pillars = live
	}
	days := req.Days
	if days <= 0 {
		days = 30
	}
	series := req.Stage == StageScaling || req.Stage == StageMonetizationPush || req.Stage == StageMonetized
	episodes := map[string]int{}
	var out []PlanDraft
	for d := 0; d < days; d++ {
		date := req.Start.AddDate(0, 0, d)
		dateStr := date.Format("2006-01-02")
		pillar := pillars[d%len(pillars)]
		second := pillars[(d+1)%len(pillars)]
		for i := 0; i < dailyCadence(req.Stage, d); i++ {
			selected := pillar
			if i == 1 && len(pillars) > 1 {
				selected = second
			}
			formatID := Slugify(selected)
			episode, topic := 0, selected
			if series {
				episodes[formatID]++
				episode = episodes[formatID]
				topic = fmt.Sprintf("%s — tập %d", selected, episode)
			}
			out = append(out, PlanDraft{
				Date: dateStr, FormatID: formatID, Variant: VariantTikTok,
				Topic: topic, Hook: hookByVariant[VariantTikTok], SeriesEp: episode,
				Group: dateStr + "|" + formatID,
			})
		}
	}
	return out
}

// -------------------------------------------------------------- decisions

// FormatInput is the evidence the kill rule reads for one format family.

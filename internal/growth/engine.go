package growth

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------- targets

// DefaultTargets returns the doc §4.1 KPI ladder for a platform. YouTube
// YPP rows carry the 2027-02-01 deadline (new applicants face doubled
// thresholds after it); the two YPP-full paths (watch hours OR Shorts
// views) are separate rows because YouTube does not sum them.
func DefaultTargets(platform string) []Target {
	switch platform {
	case "youtube":
		return []Target{
			{Metric: "subs", Threshold: 100, Label: "Mốc 100 subscriber"},
			{Metric: "subs", Threshold: 500, Label: "YPP tier 1 (500 sub)", Deadline: YPPDeadline},
			{Metric: "subs", Threshold: 1000, Label: "YPP full — subscriber", Deadline: YPPDeadline},
			{Metric: "watch_hours_12m", Threshold: 4000, Label: "YPP full — 4.000 giờ xem", Deadline: YPPDeadline},
			{Metric: "shorts_views_90d", Threshold: 10000000, Label: "YPP full — 10 triệu view Shorts", Deadline: YPPDeadline},
		}
	default: // tiktok
		return []Target{
			{Metric: "followers", Threshold: 100, Label: "Mốc 100 follower"},
			{Metric: "followers", Threshold: 1000, Label: "Mở LIVE (1.000 follower)"},
			{Metric: "followers", Threshold: 10000, Label: "Mốc uy tín / Shop (10.000)"},
		}
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

// PlanRequest describes one 30-day plan generation.
type PlanRequest struct {
	Stage      string
	HasYouTube bool     // account has a YouTube channel attached
	Pillars    []string // persona content pillars = initial format families
	Start      time.Time
	Days       int // horizon (30 for the rolling plan)
	// KilledFormats: format_id đã bị kill (VerdictKilled) — plan mới không
	// lên lịch lại các format này (đợt AA).
	KilledFormats []string
}

// PlanDraft is one item GeneratePlan wants scheduled.
type PlanDraft struct {
	Date     string // YYYY-MM-DD
	FormatID string
	Variant  string
	Topic    string
	Hook     string
	SeriesEp int    // >0 for numbered series episodes (scaling+)
	Group    string // variant lineage group: "<date>|<format_id>"
}

// hookByVariant is the retention contract per surface (docs §2/§5.2): the
// plan describes the hook obligation; the script itself is Studio's job.
var hookByVariant = map[string]string{
	VariantTikTok:  "Hook 3 giây đầu: nêu kết quả/câu hỏi gây tò mò, payoff ở cuối video (giữ completion)",
	VariantShorts:  "Hook 1–2 giây đầu: khung hình thị giác mạnh chống swipe, vòng lặp liền mạch",
	VariantYouTube: "Title/thumbnail tối ưu CTR + hook 30 giây đầu, ghim Related Video từ Short thắng",
}

// dailyCadence returns how many items of each variant one day gets under a
// stage (dayIndex is 0-based from plan start). Cadence caps come from doc
// §4.1/§5.3: TikTok max 3/day, canary week is minimum cadence, long-form
// only opens at scaling (Short winners become long-form sources).
func dailyCadence(stage string, dayIndex int, hasYouTube bool) (tiktok, shorts, long int) {
	if stage == StageColdStart && dayIndex < CanaryDays {
		// Canary: 1 video/day TikTok, 1 Short/day, no long-form, no LIVE.
		tiktok = 1
		if hasYouTube {
			shorts = 1
		}
		return tiktok, shorts, 0
	}
	switch stage {
	case StageColdStart, StageFormatTesting, StageStalled:
		tiktok = 1
		if hasYouTube {
			shorts = 1
		}
	case StageScaling:
		tiktok = 2
		if hasYouTube {
			shorts = 1
		}
	case StageMonetizationPush:
		tiktok = 2
		if hasYouTube {
			shorts = 1
		}
	case StageMonetized:
		tiktok = 1
		if hasYouTube {
			shorts = 1
		}
	}
	return tiktok, shorts, 0
}

// longFormDays marks which plan days carry a YouTube long-form item:
// scaling 2/week (ngày 2 & 5 của mỗi tuần), monetization_push 3/week,
// monetized 2/week. Never during canary/testing (doc §4.3).
func longFormDay(stage string, dayIndex int) bool {
	wd := dayIndex % 7
	switch stage {
	case StageScaling, StageMonetized:
		return wd == 1 || wd == 4
	case StageMonetizationPush:
		return wd == 0 || wd == 2 || wd == 4
	}
	return false
}

// GeneratePlan builds the rolling plan deterministically: pillars rotate
// daily so every format family gets tested evenly, and from the scaling
// stage on the same family repeats as numbered series episodes (series are
// what converts views into followers — doc §2.1.6). Pure function: same
// input, same plan.
func GeneratePlan(req PlanRequest) []PlanDraft {
	pillars := req.Pillars
	if len(pillars) == 0 {
		pillars = []string{"nội dung ngắn"}
	}
	// Đợt AA: loại pillar đã bị kill khỏi vòng xoay.
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
	// Nếu TẤT CẢ pillar đều bị kill: giữ nguyên để plan không trống rỗng,
	// nhưng đánh dấu bằng decision ở caller (generatePlanFor).
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
		tk, sh, _ := dailyCadence(req.Stage, d, req.HasYouTube)

		emit := func(p string, variant string) {
			fid := Slugify(p)
			ep := 0
			topic := p
			if series {
				episodes[fid]++
				ep = episodes[fid]
				topic = fmt.Sprintf("%s — tập %d", p, ep)
			}
			out = append(out, PlanDraft{
				Date: dateStr, FormatID: fid, Variant: variant,
				Topic: topic, Hook: hookByVariant[variant], SeriesEp: ep,
				Group: dateStr + "|" + fid,
			})
		}
		for i := 0; i < tk; i++ {
			if i == 1 && len(pillars) > 1 {
				emit(second, VariantTikTok)
			} else {
				emit(pillar, VariantTikTok)
			}
		}
		for i := 0; i < sh; i++ {
			emit(pillar, VariantShorts)
		}
		if req.HasYouTube && longFormDay(req.Stage, d) {
			topic := pillar
			ep := 0
			if series {
				episodes[Slugify(pillar)]++
				ep = episodes[Slugify(pillar)]
				topic = fmt.Sprintf("%s — bản dài tập %d", pillar, ep)
			} else {
				topic = pillar + " — bản dài"
			}
			out = append(out, PlanDraft{
				Date: date.Format("2006-01-02"), FormatID: Slugify(pillar),
				Variant: VariantYouTube, Topic: topic, Hook: hookByVariant[VariantYouTube],
				SeriesEp: ep, Group: date.Format("2006-01-02") + "|" + Slugify(pillar),
			})
		}
	}
	return out
}

// -------------------------------------------------------------- decisions

// FormatInput is the evidence the kill rule reads for one format family.

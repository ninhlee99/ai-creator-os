package growth

import (
	"testing"
	"time"
)

func fptr(f float64) *float64 { return &f }

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"truyện ma":            "truyen_ma",
		"Từ vựng theo chủ đề":  "tu_vung_theo_chu_de",
		"tiếng Anh qua truyện": "tieng_anh_qua_truyen",
		"Đồ công nghệ xinh":    "do_cong_nghe_xinh",
		"  nhiều   khoảng  ":   "nhieu_khoang",
		"":                     "format",
		"!!!":                  "format",
		"full gameplay":        "full_gameplay",
		"luyện nghe (cơ bản)":  "luyen_nghe_co_ban",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeneratePlanColdStartCanary(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) // Monday
	items := GeneratePlan(PlanRequest{
		Stage: StageColdStart, HasYouTube: true,
		Pillars: []string{"truyện ngắn", "truyện ma"}, Start: start, Days: 30,
	})
	if len(items) == 0 {
		t.Fatal("empty plan")
	}
	for _, it := range items {
		if it.Variant == VariantYouTube {
			t.Errorf("cold_start must not schedule long-form, got %+v", it)
		}
	}
	// Canary week: exactly 1 tiktok + 1 shorts per day.
	perDay := map[string]map[string]int{}
	for _, it := range items {
		if perDay[it.Date] == nil {
			perDay[it.Date] = map[string]int{}
		}
		perDay[it.Date][it.Variant]++
	}
	day0 := start.Format("2006-01-02")
	if perDay[day0][VariantTikTok] != 1 || perDay[day0][VariantShorts] != 1 {
		t.Errorf("canary day0 cadence = %v, want 1 tiktok + 1 shorts", perDay[day0])
	}
}

func TestGeneratePlanScalingCadenceAndSeries(t *testing.T) {
	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	items := GeneratePlan(PlanRequest{
		Stage: StageScaling, HasYouTube: true,
		Pillars: []string{"truyện ngắn"}, Start: start, Days: 14,
	})
	perDay := map[string]map[string]int{}
	longDays := 0
	for _, it := range items {
		if perDay[it.Date] == nil {
			perDay[it.Date] = map[string]int{}
		}
		perDay[it.Date][it.Variant]++
		if it.Variant == VariantYouTube {
			longDays++
		}
		if it.Variant == VariantTikTok && it.SeriesEp == 0 {
			t.Errorf("scaling tiktok items must be numbered series episodes: %+v", it)
		}
	}
	day0 := start.Format("2006-01-02")
	if perDay[day0][VariantTikTok] != 2 {
		t.Errorf("scaling day0 tiktok = %d, want 2", perDay[day0][VariantTikTok])
	}
	if longDays != 4 { // 2/week * 2 weeks
		t.Errorf("scaling long-form items = %d, want 4", longDays)
	}
}

func TestGeneratePlanNoYouTube(t *testing.T) {
	items := GeneratePlan(PlanRequest{
		Stage: StageFormatTesting, HasYouTube: false,
		Pillars: []string{"tip 60s"}, Start: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Days: 7,
	})
	for _, it := range items {
		if it.Variant != VariantTikTok {
			t.Errorf("no-YouTube account must only get tiktok items, got %s", it.Variant)
		}
	}
	if len(items) != 7 {
		t.Errorf("format_testing 7 days tiktok-only = %d items, want 7", len(items))
	}
}

func TestDecideKill(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -20)
	cases := []struct {
		name string
		in   FormatInput
		want bool
	}{
		{"enough data, low completion", FormatInput{Videos: 9, FirstVideoAt: old, MedianCompletion: fptr(0.20)}, true},
		{"not enough videos", FormatInput{Videos: 7, FirstVideoAt: old, MedianCompletion: fptr(0.20)}, false},
		{"not enough days", FormatInput{Videos: 12, FirstVideoAt: now.AddDate(0, 0, -5), MedianCompletion: fptr(0.20)}, false},
		{"completion above floor", FormatInput{Videos: 9, FirstVideoAt: old, MedianCompletion: fptr(0.50)}, false},
		{"exactly at floor survives", FormatInput{Videos: 8, FirstVideoAt: old, MedianCompletion: fptr(0.35)}, false},
		{"no completion, low proxy", FormatInput{Videos: 9, FirstVideoAt: old, MedianProxyRate: fptr(0.005)}, true},
		{"no completion, healthy proxy", FormatInput{Videos: 9, FirstVideoAt: old, MedianProxyRate: fptr(0.03)}, false},
		{"no data at all", FormatInput{Videos: 9, FirstVideoAt: old}, false},
	}
	for _, c := range cases {
		if got, _ := DecideKill(cfg, c.in, now); got != c.want {
			t.Errorf("%s: DecideKill = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDecideDoubleDownBreakout(t *testing.T) {
	cfg := DefaultConfig()
	v := VideoStat{Views: 5000}
	if !DecideDoubleDown(cfg, 1000, v) {
		t.Error("5x median must double down")
	}
	if DecideDoubleDown(cfg, 1000, VideoStat{Views: 4999}) {
		t.Error("below 5x must not double down")
	}
	if DecideDoubleDown(cfg, 0, v) {
		t.Error("no baseline median must not double down")
	}
	if !DecideBreakout(cfg, 400, v) {
		t.Error("12.5x median must break out")
	}
	if DecideBreakout(cfg, 1000, v) {
		t.Error("5x median is double-down, not breakout")
	}
}

func TestDecidePenalty(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		name string
		in   PenaltyInput
		want bool
	}{
		{"policy error", PenaltyInput{PolicyErrors: 1}, true},
		{"takedown", PenaltyInput{RemovedVideos: 1}, true},
		{"views collapse 80%", PenaltyInput{HasViews: true, Views30dPrev: 10000, Views30dNow: 2000}, true},
		{"views dip 50% survives", PenaltyInput{HasViews: true, Views30dPrev: 10000, Views30dNow: 5000}, false},
		{"no views data", PenaltyInput{}, false},
		{"prev zero", PenaltyInput{HasViews: true, Views30dPrev: 0, Views30dNow: 0}, false},
	}
	for _, c := range cases {
		if got, _ := DecidePenalty(cfg, c.in); got != c.want {
			t.Errorf("%s: DecidePenalty = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDecideStalled(t *testing.T) {
	cfg := DefaultConfig()
	if !DecideStalled(cfg, StalledInput{Days: 14, FollowerGrowth: 0.01, MedianCompletion: fptr(0.2)}) {
		t.Error("14d flat + weak completion must be stalled")
	}
	if DecideStalled(cfg, StalledInput{Days: 10, FollowerGrowth: 0.0}) {
		t.Error("window too short must not be stalled")
	}
	if DecideStalled(cfg, StalledInput{Days: 20, FollowerGrowth: 0.05}) {
		t.Error("growing followers must not be stalled")
	}
	if DecideStalled(cfg, StalledInput{Days: 20, FollowerGrowth: 0.0, MedianCompletion: fptr(0.6)}) {
		t.Error("strong completion must not be stalled")
	}
	if !DecideStalled(cfg, StalledInput{Days: 20, FollowerGrowth: 0.0}) {
		t.Error("unknown completion + flat growth must be stalled")
	}
}

func TestAdvanceStage(t *testing.T) {
	cases := []struct {
		cur  string
		in   StageInput
		want string
	}{
		{StageColdStart, StageInput{DaysSinceStart: 6, CanaryClean: true}, StageColdStart},
		{StageColdStart, StageInput{DaysSinceStart: 7, CanaryClean: true}, StageFormatTesting},
		{StageColdStart, StageInput{DaysSinceStart: 30, CanaryClean: false}, StageColdStart},
		{StageFormatTesting, StageInput{HasWinner: false}, StageFormatTesting},
		{StageFormatTesting, StageInput{HasWinner: true}, StageScaling},
		{StageScaling, StageInput{NearMonetization: true}, StageMonetizationPush},
		{StageMonetizationPush, StageInput{Monetized: true}, StageMonetized},
		{StageMonetized, StageInput{}, StageMonetized},
		{StageStalled, StageInput{CanaryClean: true}, StageFormatTesting},
		{StageStalled, StageInput{CanaryClean: false}, StageStalled},
	}
	for _, c := range cases {
		if got := AdvanceStage(c.cur, c.in); got != c.want {
			t.Errorf("AdvanceStage(%s, %+v) = %s, want %s", c.cur, c.in, got, c.want)
		}
	}
}

func TestDefaultTargets(t *testing.T) {
	yt := DefaultTargets("youtube")
	found := map[string]bool{}
	for _, tg := range yt {
		found[tg.Label] = true
		if tg.Metric == "subs" && tg.Threshold == 1000 && tg.Deadline != YPPDeadline {
			t.Error("YPP full subs target must carry the 2027-02-01 deadline")
		}
	}
	if !found["YPP full — 4.000 giờ xem"] || !found["YPP full — 10 triệu view Shorts"] {
		t.Error("YouTube ladder must include both YPP-full paths")
	}
	tk := DefaultTargets("tiktok")
	hasLive := false
	for _, tg := range tk {
		if tg.Threshold == 1000 {
			hasLive = true
		}
	}
	if !hasLive {
		t.Error("TikTok ladder must include the 1.000-follower LIVE milestone")
	}
}

func TestMedian(t *testing.T) {
	if Median(nil) != 0 || Median([]float64{5}) != 5 || Median([]float64{1, 3}) != 2 || Median([]float64{4, 1, 2}) != 2 {
		t.Error("median wrong")
	}
}

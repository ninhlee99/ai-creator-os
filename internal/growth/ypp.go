package growth

// Tiến độ YouTube Partner Program (YPP) — tính từ snapshot thật.
//
// Ngưỡng công khai của YouTube:
//   - Mức đầy đủ (chia sẻ doanh thu quảng cáo): 1.000 người đăng ký VÀ
//     (4.000 giờ xem công khai trong 12 tháng HOẶC 10 triệu lượt xem
//     Shorts trong 90 ngày).
//   - Mức sớm (fan funding: Super Thanks, hội viên…): 500 người đăng ký
//     VÀ (3.000 giờ xem / 12 tháng HOẶC 3 triệu view Shorts / 90 ngày)
//     — kèm 3 video công khai trong 90 ngày (app không kiểm tra điều này).
//
// Giờ xem 12 tháng đọc từ Analytics (estimatedMinutesWatched) là ƯỚC
// TÍNH: YouTube chỉ tính giờ xem video dài công khai — số thật trong
// YouTube Studio có thể thấp hơn. Thiếu số liệu → báo thiếu, không bịa.

// YPPTier là một mức ngưỡng YPP.
type YPPTier struct {
	Name       string
	Subs       float64
	WatchHours float64
	ShortsView float64
}

// YPPTiers: mức sớm trước, mức đầy đủ sau.
var YPPTiers = []YPPTier{
	{Name: "Mức sớm (fan funding)", Subs: 500, WatchHours: 3000, ShortsView: 3_000_000},
	{Name: "Mức đầy đủ (chia sẻ quảng cáo)", Subs: 1000, WatchHours: 4000, ShortsView: 10_000_000},
}

// YPPTierProgress là tiến độ của một kênh tới một mức.
type YPPTierProgress struct {
	Tier        YPPTier
	SubsPct     float64 // 0–100
	WatchPct    float64 // 0–100, -1 = thiếu số liệu
	ShortsPct   float64 // 0–100, -1 = thiếu số liệu
	BestPath    string  // "giờ xem" | "Shorts" | ""
	Eligible    bool
	MissingData bool
	OverallPct  float64 // min(subs, đường tốt nhất)
}

// YPPProgress tính tiến độ mọi mức từ snapshot mới nhất (nil = chưa có).
func YPPProgress(snap *Snapshot) []YPPTierProgress {
	var subs, wh, sv float64
	hasSubs, hasWH, hasSV := false, false, false
	if snap != nil {
		if snap.Followers != nil {
			subs, hasSubs = float64(*snap.Followers), true
		}
		wh, hasWH = extraNum(snap, "watch_hours_12m")
		sv, hasSV = extraNum(snap, "shorts_views_90d")
	}
	out := make([]YPPTierProgress, 0, len(YPPTiers))
	for _, t := range YPPTiers {
		p := YPPTierProgress{Tier: t, WatchPct: -1, ShortsPct: -1}
		p.MissingData = !hasSubs || (!hasWH && !hasSV)
		p.SubsPct = pct(subs, t.Subs)
		if hasWH {
			p.WatchPct = pct(wh, t.WatchHours)
		}
		if hasSV {
			p.ShortsPct = pct(sv, t.ShortsView)
		}
		best := p.WatchPct
		p.BestPath = "giờ xem"
		if p.ShortsPct > best {
			best, p.BestPath = p.ShortsPct, "Shorts"
		}
		if best < 0 {
			best, p.BestPath = 0, ""
		}
		p.OverallPct = p.SubsPct
		if best < p.OverallPct {
			p.OverallPct = best
		}
		p.Eligible = hasSubs && p.SubsPct >= 100 && best >= 100
		out = append(out, p)
	}
	return out
}

func pct(v, target float64) float64 {
	if target <= 0 {
		return 0
	}
	p := v / target * 100
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

func extraNum(snap *Snapshot, key string) (float64, bool) {
	if snap == nil || snap.Extra == nil {
		return 0, false
	}
	switch n := snap.Extra[key].(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

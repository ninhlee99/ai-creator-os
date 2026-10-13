package growth

import "testing"

func TestYPPProgress(t *testing.T) {
	if p := YPPProgress(nil); len(p) != 2 || !p[0].MissingData || p[0].Eligible {
		t.Fatalf("nil snapshot: %+v", p)
	}
	subs := int64(600)
	snap := &Snapshot{Followers: &subs, Extra: map[string]any{
		"watch_hours_12m": 1500.0, "shorts_views_90d": 3_500_000.0,
	}}
	p := YPPProgress(snap)
	early, full := p[0], p[1]
	if !early.Eligible || early.BestPath != "Shorts" {
		t.Fatalf("600 sub + 3.5M Shorts phải đủ mức sớm: %+v", early)
	}
	if full.Eligible || full.SubsPct != 60 || full.OverallPct != 37.5 || full.BestPath != "giờ xem" {
		t.Fatalf("mức đầy đủ: %+v", full)
	}
	if full.WatchPct < 37.4 || full.WatchPct > 37.6 {
		t.Fatalf("watch pct = %v", full.WatchPct)
	}
	// Chỉ có sub, thiếu giờ xem lẫn Shorts → thiếu số liệu, không đủ điều kiện.
	only := &Snapshot{Followers: &subs, Extra: map[string]any{}}
	if q := YPPProgress(only); !q[0].MissingData || q[0].Eligible || q[0].WatchPct != -1 {
		t.Fatalf("thiếu số liệu: %+v", q[0])
	}
}

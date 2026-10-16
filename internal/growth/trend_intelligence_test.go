package growth

import (
	"context"
	"testing"
)

func TestAnalyzeTrendSignals(t *testing.T) {
	brand := DefaultBrandDNA("mỹ phẩm", 1)

	signals := []TrendSignal{
		{
			Title:    "Nơi Pháo Hoa Rực Rỡ",
			Artist:   "Hòa Minzy & Đức Phúc",
			Movement: "+12",
			Source:   "Kworb TikTok VN",
			Rank:     1,
		},
		{
			Title:    "Thiên Lý Ơi",
			Artist:   "Jack",
			Movement: "NEW",
			Source:   "Kworb TikTok VN",
			Rank:     3,
		},
	}

	opps := AnalyzeTrendSignals(context.Background(), brand, signals)

	if len(opps) != 2 {
		t.Fatalf("expected 2 opportunities, got %d", len(opps))
	}

	for _, opp := range opps {
		if opp.OpportunityScore <= 0 || opp.OpportunityScore > 100 {
			t.Errorf("invalid opportunity score: %d", opp.OpportunityScore)
		}
		if opp.VelocityScore <= 0 {
			t.Errorf("expected positive velocity score, got %d", opp.VelocityScore)
		}
		if len(opp.OriginalAngles) < 3 {
			t.Errorf("expected at least 3 original angles, got %d", len(opp.OriginalAngles))
		}
		if opp.AudienceDemand == "" {
			t.Error("expected non-empty audience demand")
		}
		if opp.Confidence != 85 {
			t.Errorf("expected honest 85%% confidence, got %d", opp.Confidence)
		}
	}
}

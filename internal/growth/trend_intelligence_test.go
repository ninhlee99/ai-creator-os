package growth

import (
	"context"
	"strings"
	"testing"
)

func TestAnalyzeTrendSignals(t *testing.T) {
	brand := DefaultBrandDNA("mỹ phẩm", 1)

	signals := []TrendSignal{
		{
			Title:  "Dance trend in Vietnam",
			Music:  "Nơi Pháo Hoa Rực Rỡ",
			Source: "TikWM TikTok VN feed",
			Rank:   1,
		},
		{
			Title:  "Video dance viral",
			Music:  "Thiên Lý Ơi",
			Source: "TikWM TikTok VN feed",
			Rank:   3,
		},
	}

	opps := AnalyzeTrendSignals(context.Background(), brand, signals)

	if len(opps) != 2 {
		t.Fatalf("expected 2 opportunities, got %d", len(opps))
	}

	for _, opp := range opps {
		if opp.OpportunityScore != nil {
			t.Errorf("expected unknown opportunity score without market evidence, got %d", *opp.OpportunityScore)
		}
		if opp.VelocityScore != nil || opp.BrandFitScore != nil {
			t.Error("expected unsupported velocity and brand-fit scores to remain unknown")
		}
		if len(opp.OriginalAngles) < 3 {
			t.Errorf("expected at least 3 original angles, got %d", len(opp.OriginalAngles))
		}
		if !strings.Contains(opp.AudienceDemand, "Chưa có dữ liệu") || !strings.Contains(opp.CompetitionLevel, "Chưa có dữ liệu") {
			t.Errorf("expected audience demand and competition to remain unknown, got audience=%q competition=%q", opp.AudienceDemand, opp.CompetitionLevel)
		}
		if opp.Confidence != nil {
			t.Errorf("expected uncalibrated confidence to remain unknown, got %d", *opp.Confidence)
		}
	}
}

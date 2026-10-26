package growth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TrendSignal represents a video trend and its sound from a configured feed.
type TrendSignal struct {
	Title    string `json:"title"`
	Music    string `json:"music,omitempty"`
	Movement string `json:"movement,omitempty"` // e.g. "+15", "=", "NEW"
	Source   string `json:"source"`
	Rank     int    `json:"rank,omitempty"`
}

// TrendOpportunity represents an analyzed original content opportunity
// derived from trend signals and tailored to a channel's Brand DNA.
type TrendOpportunity struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Source            string   `json:"source"`
	Category          string   `json:"category"` // Loại tín hiệu từ video viral
	Rank              int      `json:"rank,omitempty"`
	MovementSignal    string   `json:"movement_signal,omitempty"`
	VelocityScore     *int     `json:"velocity_score,omitempty"`    // Requires dated view snapshots
	AudienceDemand    string   `json:"audience_demand"`             // Unknown without audience behavior evidence
	CompetitionLevel  string   `json:"competition_level"`           // Unknown without comparable topic sample
	BrandFitScore     *int     `json:"brand_fit_score,omitempty"`   // Title overlap is not audience fit
	OpportunityScore  *int     `json:"opportunity_score,omitempty"` // Requires sufficient market evidence
	OriginalAngles    []string `json:"original_angles"`             // 3-4 góc khai thác nguyên bản (không reup)
	RecommendedFormat string   `json:"recommended_format"`          // TikTok 9:16
	Confidence        *int     `json:"confidence,omitempty"`        // Requires calibration data
	EvidenceNote      string   `json:"evidence_note"`
	AnalyzedAt        string   `json:"analyzed_at"`
}

// AnalyzeTrendSignals preserves source signals and suggests original angles.
// Market metrics stay unknown until the feed provides the evidence they need.
func AnalyzeTrendSignals(_ context.Context, brand *BrandDNA, signals []TrendSignal) []TrendOpportunity {
	now := time.Now()
	var opportunities []TrendOpportunity

	for i, sig := range signals {
		oppID := fmt.Sprintf("trend-opp-%d-%d", now.UnixNano(), i+1)
		opp := TrendOpportunity{
			ID:               oppID,
			Title:            sig.Title,
			Source:           sig.Source,
			Rank:             sig.Rank,
			MovementSignal:   strings.TrimSpace(sig.Movement),
			AudienceDemand:   "Chưa có dữ liệu hành vi khán giả để kết luận nhu cầu.",
			CompetitionLevel: "Chưa có dữ liệu video cùng chủ đề để đánh giá cạnh tranh.",
			EvidenceNote:     "Nguồn chỉ cung cấp chủ đề, thứ hạng và movement; chưa có snapshot lượt xem theo thời gian, mẫu video cạnh tranh hoặc hành vi khán giả.",
			AnalyzedAt:       now.Format(time.RFC3339Nano),
		}

		if sig.Music != "" {
			opp.Title = sig.Title + " — " + sig.Music
			opp.Category = "Video viral TikTok & âm thanh"
		} else {
			opp.Category = "Video viral TikTok"
		}

		// Ranking and movement alone do not establish view velocity, audience demand,
		// competition, or a calibrated opportunity score.
		opp.OriginalAngles = generateOriginalAngles(brand, sig)
		opp.RecommendedFormat = "Video dọc 9:16; thử thời lượng và hook theo dữ liệu kênh"

		opportunities = append(opportunities, opp)
	}

	return opportunities
}

func generateOriginalAngles(brand *BrandDNA, sig TrendSignal) []string {
	title := sig.Title
	if sig.Music != "" {
		title += " — " + sig.Music
	}

	angles := []string{
		fmt.Sprintf("Mở video bằng một câu hỏi hoặc tình huống gắn với chủ đề '%s'; chỉ nêu điều có thể kiểm chứng.", title),
		fmt.Sprintf("Dùng '%s' làm bối cảnh cho một thử nghiệm hoặc so sánh có tiêu chí rõ và dữ liệu riêng.", title),
		fmt.Sprintf("Kể trải nghiệm mới của nhân vật kênh quanh '%s'; không sao chép lời thoại hoặc bố cục video nguồn.", title),
	}

	if brand != nil && len(brand.HookRules) > 0 {
		angles = append(angles, fmt.Sprintf("Áp dụng hook kênh đã cấu hình: '%s'; nội dung vẫn cần bằng chứng riêng.", brand.HookRules[0]))
	}

	return angles
}

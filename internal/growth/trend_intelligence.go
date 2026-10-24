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
	Category          string   `json:"category"`           // Loại tín hiệu từ video viral
	VelocityScore     int      `json:"velocity_score"`     // 0-100 (tốc độ tăng trưởng)
	AudienceDemand    string   `json:"audience_demand"`    // Câu hỏi / nhu cầu tiềm ẩn của khán giả
	CompetitionLevel  string   `json:"competition_level"`  // Thấp, Trung bình, Bão hòa
	BrandFitScore     int      `json:"brand_fit_score"`    // 0-100 (độ tương thích với Brand DNA)
	OpportunityScore  int      `json:"opportunity_score"`  // 0-100 (điểm cơ hội tổng hợp)
	OriginalAngles    []string `json:"original_angles"`    // 3-4 góc khai thác nguyên bản (không reup)
	RecommendedFormat string   `json:"recommended_format"` // TikTok 9:16
	Confidence        int      `json:"confidence"`         // Độ tin cậy dữ liệu %
	AnalyzedAt        string   `json:"analyzed_at"`
}

// AnalyzeTrendSignals transforms viral-video feed signals into actionable original
// content opportunities matched against a specific Brand DNA.
func AnalyzeTrendSignals(ctx context.Context, brand *BrandDNA, signals []TrendSignal) []TrendOpportunity {
	now := time.Now().Format("2006-01-02 15:04:05")
	var opportunities []TrendOpportunity

	niche := "chung"
	if brand != nil && brand.Positioning != "" {
		niche = strings.ToLower(brand.Positioning)
	}

	for i, sig := range signals {
		oppID := fmt.Sprintf("trend-opp-%d-%d", time.Now().Unix(), i+1)
		opp := TrendOpportunity{
			ID:         oppID,
			Title:      sig.Title,
			Source:     sig.Source,
			AnalyzedAt: now,
			Confidence: 85,
		}

		if sig.Music != "" {
			opp.Title = sig.Title + " — " + sig.Music
			opp.Category = "Video viral TikTok & âm thanh"
		} else {
			opp.Category = "Video viral TikTok"
		}

		// Calculate Velocity based on movement and rank
		opp.VelocityScore = calculateVelocity(sig)

		// Calculate Brand Fit
		opp.BrandFitScore = calculateBrandFit(brand, sig)

		// Determine Competition and Demand
		if opp.VelocityScore > 80 {
			opp.CompetitionLevel = "Trung bình (Đang tăng nhanh)"
		} else if opp.VelocityScore > 60 {
			opp.CompetitionLevel = "Thấp (Cơ hội đón đầu)"
		} else {
			opp.CompetitionLevel = "Bão hòa"
		}

		// Composite Opportunity Score = (Velocity * 0.4) + (BrandFit * 0.4) + (Gap * 0.2)
		gapScore := 70
		if opp.CompetitionLevel == "Thấp (Cơ hội đón đầu)" {
			gapScore = 95
		} else if opp.CompetitionLevel == "Bão hòa" {
			gapScore = 40
		}
		opp.OpportunityScore = int(float64(opp.VelocityScore)*0.4 + float64(opp.BrandFitScore)*0.4 + float64(gapScore)*0.2)

		// Generate Original Angles based on Brand DNA and Trend
		opp.OriginalAngles = generateOriginalAngles(brand, sig, opp)
		opp.AudienceDemand = extractAudienceDemand(niche, sig)
		opp.RecommendedFormat = "Video ngắn 9:16 (30–50s, 4 Hook 3s)"

		opportunities = append(opportunities, opp)
	}

	return opportunities
}

func calculateVelocity(sig TrendSignal) int {
	score := 65
	if strings.HasPrefix(sig.Movement, "+") {
		score = 85
	} else if sig.Movement == "NEW" {
		score = 95
	} else if sig.Movement == "=" {
		score = 70
	} else if strings.HasPrefix(sig.Movement, "-") {
		score = 50
	}
	if sig.Rank > 0 && sig.Rank <= 10 {
		score += 10
	}
	if score > 100 {
		score = 100
	}
	return score
}

func calculateBrandFit(brand *BrandDNA, sig TrendSignal) int {
	if brand == nil {
		return 75
	}
	text := strings.ToLower(sig.Title + " " + sig.Music)
	pos := strings.ToLower(brand.Positioning)

	fit := 70
	for _, p := range brand.ContentPillars {
		pLower := strings.ToLower(p)
		for _, w := range strings.Fields(pLower) {
			if len(w) > 3 && strings.Contains(text, w) {
				fit += 8
				break
			}
		}
	}
	if strings.Contains(pos, "mỹ phẩm") || strings.Contains(pos, "skincare") {
		fit += 10
	}
	if fit > 98 {
		fit = 98
	}
	return fit
}

func extractAudienceDemand(niche string, sig TrendSignal) string {
	switch {
	case strings.Contains(niche, "mỹ phẩm") || strings.Contains(niche, "skincare"):
		return "Khán giả tò mò về hiệu quả thực tế và cách phối hợp thành phần an toàn theo nhịp điệu bài hát."
	case strings.Contains(niche, "công nghệ") || strings.Contains(niche, "tech"):
		return "Người xem muốn thấy màn thử nghiệm tính năng nhanh, dứt khoát khớp với từng nhịp beat."
	case strings.Contains(niche, "gia dụng"):
		return "Khán giả tìm kiếm mẹo giải phóng sức lao động trong 30 giây thư giãn cùng âm nhạc."
	case strings.Contains(niche, "kể chuyện") || strings.Contains(niche, "lịch sử"):
		return "Khán giả khao khát lắng nghe một câu chuyện nhân văn hoặc nút thắt bất ngờ trên nền nhạc sâu lắng."
	default:
		return "Khán giả quan tâm giải pháp thực tế, súc tích và minh bạch trong từng giây nội dung."
	}
}

func generateOriginalAngles(brand *BrandDNA, sig TrendSignal, opp TrendOpportunity) []string {
	title := sig.Title
	if sig.Music != "" {
		title += " — " + sig.Music
	}

	angles := []string{
		fmt.Sprintf("Góc 1 (Thử thách & Phá vỡ định kiến): Bắt đầu bằng âm thanh '%s' và lật ngược 1 quan niệm sai lầm phổ biến", title),
		fmt.Sprintf("Góc 2 (So sánh trực diện 60s): Sử dụng đoạn drop của '%s' để tiết lộ sự khác biệt giữa 2 lựa chọn hàng đầu", title),
		fmt.Sprintf("Góc 3 (Kể chuyện ngôi thứ nhất): Tình huống đời thực vượt qua khó khăn, dùng '%s' làm nhạc nền cảm xúc", title),
	}

	if brand != nil && len(brand.HookRules) > 0 {
		angles = append(angles, fmt.Sprintf("Góc 4 (Hook độc quyền theo Brand DNA): Áp dụng công thức '%s' đón đầu sóng viral", brand.HookRules[0]))
	}

	return angles
}

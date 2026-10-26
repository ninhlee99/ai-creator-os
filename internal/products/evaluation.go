package products

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// CandidateEvaluation reports evidence available to the affiliate reviewer.
// It is intentionally advisory: missing market or audience data must never
// turn into an automatic approval or a fabricated negative verdict.
type CandidateEvaluation struct {
	Recommendation        string
	RecommendationTone    string
	OfferReady            bool
	OfferGaps             []string
	CommissionPerOrder    float64
	CommissionPerOrderMax float64
	CommissionAmount      float64
	CommissionCurrency    string
	CommissionFloor       float64
	HasCommissionFloor    bool
	BelowCommissionFloor  bool
	DemandVerified        bool
	SalesEvidence         string
	RatingEvidence        string
	TrendEvidence         string
	TrendSourceURL        string
	AudienceEvidence      string
}

// EvaluateCandidate summarizes offer and demand evidence without treating
// unavailable TikTok market or demographic metrics as a pass or a fail.
func EvaluateCandidate(p Product, now time.Time) CandidateEvaluation {
	if now.IsZero() {
		now = time.Now()
	}
	e := CandidateEvaluation{
		DemandVerified:   p.HasDemandSignalsAt(now),
		TrendEvidence:    "Chưa ghép sản phẩm với video/chủ đề trong TikTok Radar; chưa đủ dữ liệu để kết luận xu hướng thị trường.",
		AudienceEvidence: "Chưa có dữ liệu nhân khẩu học tuổi từ quyền TikTok hiện tại; chưa thể kết luận phù hợp giới trẻ.",
	}
	if finitePositive(p.Price) && finitePositive(p.CommissionRate) && p.CommissionRate <= 1 {
		e.CommissionPerOrder = p.Price * p.CommissionRate
		if finitePositive(p.PriceMax) && p.PriceMax > p.Price {
			e.CommissionPerOrderMax = p.PriceMax * p.CommissionRate
		}
	}
	if finitePositive(p.CommissionAmount) {
		e.CommissionAmount = p.CommissionAmount
		e.CommissionCurrency = strings.TrimSpace(p.CommissionCurrency)
	}
	if strings.TrimSpace(p.TikTokRadarTrendEvidence) != "" {
		e.TrendEvidence = strings.TrimSpace(p.TikTokRadarTrendEvidence)
		e.TrendSourceURL = strings.TrimSpace(p.TikTokRadarTrendURL)
	}

	if !finitePositive(p.Price) {
		e.OfferGaps = append(e.OfferGaps, "thiếu giá bán hợp lệ")
	}
	if strings.TrimSpace(p.Currency) == "" {
		e.OfferGaps = append(e.OfferGaps, "thiếu loại tiền")
	}
	if !finitePositive(p.CommissionRate) || p.CommissionRate > 1 {
		e.OfferGaps = append(e.OfferGaps, "thiếu tỷ lệ hoa hồng hợp lệ")
	}
	if !validDemandEvidenceURL(p.ProductPageURL) && !validDemandEvidenceURL(p.ProductURL) {
		e.OfferGaps = append(e.OfferGaps, "thiếu listing hoặc tracking link HTTPS")
	}
	e.OfferReady = len(e.OfferGaps) == 0

	if theme, ok := DescribeTheme(strings.TrimSpace(p.Theme)); ok && theme.DefaultMinCommission > 0 {
		e.HasCommissionFloor = true
		e.CommissionFloor = theme.DefaultMinCommission
		e.BelowCommissionFloor = p.CommissionRate > 0 && p.CommissionRate < e.CommissionFloor
	}

	sourceName := strings.TrimSpace(p.Source)
	switch {
	case p.SoldCountKnown && sourceName == "manual":
		e.SalesEvidence = "Số đã bán do người dùng nhập; chưa được provider xác minh."
	case p.SoldCountKnown && sourceName == "accesstrade":
		e.SalesEvidence = "AccessTrade báo tổng " + formatSoldCount(p.SoldCount) + " units_sold; feed không nêu kỳ đo nên chưa thể kết luận tốc độ bán gần đây."
		if updatedAt := strings.TrimSpace(p.DataUpdatedAt); updatedAt != "" {
			e.SalesEvidence += " Thời điểm cập nhật do nguồn báo: " + updatedAt + "."
		} else if capturedAt := strings.TrimSpace(p.DemandEvidenceAt); capturedAt != "" {
			e.SalesEvidence += " Ứng dụng lấy dữ liệu lúc: " + capturedAt + "."
		}
		if p.SalesChangeMeasured {
			if p.SalesUnitsChange < 0 {
				e.SalesEvidence += " Hai snapshot app cách nhau " + formatDays(p.SalesChangeDays) + " ngày cho thấy số nguồn giảm " + formatSoldCount(-p.SalesUnitsChange) + "; có thể là hiệu chỉnh dữ liệu, không tính tốc độ bán."
			} else {
				e.SalesEvidence += " Hai snapshot app cách nhau " + formatDays(p.SalesChangeDays) + " ngày ghi nhận mức tăng " + formatSoldCount(p.SalesUnitsChange) + " units (khoảng " + formatDays(float64(p.SalesUnitsChange)/p.SalesChangeDays) + "/ngày); nguồn có thể cập nhật trễ."
			}
		} else {
			e.SalesEvidence += " Chưa đủ hai snapshot cách nhau 6 giờ–14 ngày để ước tính thay đổi gần đây."
		}
	case p.SoldCountKnown && validDemandEvidenceURL(p.DemandEvidenceURL) && p.DemandEvidenceAt != "":
		e.SalesEvidence = "Ứng dụng lấy dữ liệu lúc " + strings.TrimSpace(p.DemandEvidenceAt) + "; nguồn báo " + formatSoldCount(p.SoldCount) + " lượt bán."
	case p.SoldCountKnown:
		e.SalesEvidence = "Nguồn báo " + formatSoldCount(p.SoldCount) + " lượt bán nhưng thiếu URL hoặc thời điểm lấy dữ liệu."
	default:
		e.SalesEvidence = "Nguồn hiện chưa trả số lượt bán."
	}

	switch {
	case p.Rating > 0 && e.DemandVerified:
		e.RatingEvidence = "Nguồn sales + rating còn hạn; rating " + formatRating(p.Rating) + "."
	case p.Rating > 0 && sourceName == "manual":
		e.RatingEvidence = "Rating do người dùng nhập; chưa được provider xác minh. Số lượt đánh giá chưa có dữ liệu."
	case p.Rating > 0:
		e.RatingEvidence = "Rating có giá trị nhưng thiếu bằng chứng nguồn hoặc đã cũ. Số lượt đánh giá chưa có dữ liệu."
	case sourceName == "accesstrade":
		e.RatingEvidence = "AccessTrade product feed hiện không trả rating hoặc số lượt đánh giá."
	default:
		e.RatingEvidence = "Nguồn hiện chưa trả rating hoặc số lượt đánh giá."
	}

	if !e.OfferReady {
		e.Recommendation = "Chưa đủ dữ liệu offer để duyệt"
		e.RecommendationTone = "warn"
	} else if e.BelowCommissionFloor {
		e.Recommendation = "Hoa hồng dưới mốc tham khảo của danh mục"
		e.RecommendationTone = "warn"
	} else if !e.DemandVerified {
		e.Recommendation = "Cần xem lại: chưa đủ bằng chứng lượt bán và rating"
		e.RecommendationTone = "warn"
	} else {
		e.Recommendation = "Offer đủ thông tin; cần đánh giá xu hướng/kênh"
		e.RecommendationTone = "info"
	}

	return e
}

// FiveAxisEvaluation models the 5-axis analysis required by the autonomous creator OS:
// 1. Offer & Economics (Commission amount/rate, validity)
// 2. Demand & Credibility (Sales count, velocity, rating)
// 3. Trend & Market (TikTok radar trending signal)
// 4. Youth & Channel Audience Fit (Price elasticity vs Gen Z purchasing power)
// 5. Video-ability & Viral Propensity (Visual demonstration potential)
type FiveAxisEvaluation struct {
	OfferScore         float64 `json:"offer_score"`
	DemandScore        float64 `json:"demand_score"`
	TrendScore         float64 `json:"trend_score"`
	YouthAudienceScore float64 `json:"youth_audience_score"`
	VideoabilityScore  float64 `json:"videoability_score"`
	TotalScore         float64 `json:"total_score"`
	Verdict            string  `json:"verdict"` // approved | rejected | needs_review
	RejectionCode      string  `json:"rejection_code,omitempty"`
	RejectionReason    string  `json:"rejection_reason,omitempty"`
	Confidence         float64 `json:"confidence"`
	EvidenceSummary    string  `json:"evidence_summary"`
}

// AutoEvalOptions configures thresholds for automated product decisions.
type AutoEvalOptions struct {
	MinCommissionRate float64
	YouthPriceCeiling float64 // VND or local currency; e.g. 300,000 VND
	MinRating         float64
	ScoreThreshold    float64
	RadarKeywords     []string
}

// DefaultAutoEvalOptions provides sensible defaults for TikTok Affiliate.
func DefaultAutoEvalOptions() AutoEvalOptions {
	return AutoEvalOptions{
		MinCommissionRate: 0.10,   // 10%
		YouthPriceCeiling: 350000, // 350k VND max for high youth impulse-buy
		MinRating:         4.2,
		ScoreThreshold:    70.0,
	}
}

// Evaluate5Axis performs deep multi-dimensional analysis on an affiliate candidate.
func Evaluate5Axis(p Product, opts AutoEvalOptions, now time.Time) FiveAxisEvaluation {
	if now.IsZero() {
		now = time.Now()
	}
	if opts.MinCommissionRate <= 0 {
		opts.MinCommissionRate = 0.10
	}
	if opts.YouthPriceCeiling <= 0 {
		opts.YouthPriceCeiling = 350000
	}
	if opts.MinRating <= 0 {
		opts.MinRating = 4.0
	}
	if opts.ScoreThreshold <= 0 {
		opts.ScoreThreshold = 70.0
	}

	eval := FiveAxisEvaluation{
		Confidence: 0.85,
	}

	// 1. Offer & Economics Axis
	offerScore := 0.0
	if !finitePositive(p.Price) || strings.TrimSpace(p.Currency) == "" {
		eval.Verdict = ReviewRejected
		eval.RejectionCode = "ERR_INVALID_PRICE"
		eval.RejectionReason = "Sản phẩm thiếu thông tin giá hoặc đơn vị tiền tệ hợp lệ."
		return eval
	}
	if !validDemandEvidenceURL(p.ProductPageURL) && !validDemandEvidenceURL(p.ProductURL) {
		eval.Verdict = ReviewRejected
		eval.RejectionCode = "ERR_MISSING_LINK"
		eval.RejectionReason = "Thiếu tracking link hoặc product listing HTTPS hợp lệ."
		return eval
	}

	commRate := p.CommissionRate
	if commRate < opts.MinCommissionRate && p.CommissionAmount <= 0 {
		eval.Verdict = ReviewRejected
		eval.RejectionCode = "COMMISSION_TOO_LOW"
		eval.RejectionReason = fmt.Sprintf("Hoa hồng %.1f%% thấp hơn ngưỡng tối thiểu %.1f%%.", commRate*100, opts.MinCommissionRate*100)
		return eval
	}
	// Score commission: 10% => 60pts, 20% => 85pts, >=30% => 100pts
	offerScore = math.Min(commRate/0.30*100, 100)
	if offerScore < 50 {
		offerScore = 50
	}
	eval.OfferScore = offerScore

	// 2. Demand & Credibility Axis
	demandScore := 50.0 // neutral default if provider lacks signals
	if p.HasDemandSignalsAt(now) {
		if p.Rating > 0 && p.Rating < opts.MinRating {
			eval.Verdict = ReviewRejected
			eval.RejectionCode = "POOR_RATING"
			eval.RejectionReason = fmt.Sprintf("Đánh giá sản phẩm %.1f sao thấp hơn tiêu chuẩn tối thiểu %.1f sao.", p.Rating, opts.MinRating)
			return eval
		}
		// Scale by sold count (log scale) and rating
		soldScore := math.Min(math.Log10(float64(p.SoldCount)+1)/4.0*100, 100)
		ratingScore := (p.Rating / 5.0) * 100
		demandScore = 0.6*soldScore + 0.4*ratingScore
	} else if p.SoldCountKnown && p.SoldCount > 0 {
		demandScore = 65.0
	}
	eval.DemandScore = demandScore

	// 3. Trend & Market Axis
	trendScore := 55.0
	titleLower := strings.ToLower(p.Title)
	matchedTrends := 0
	for _, kw := range opts.RadarKeywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" && strings.Contains(titleLower, kw) {
			matchedTrends++
		}
	}
	if matchedTrends > 0 {
		trendScore = math.Min(75.0+float64(matchedTrends)*10.0, 95.0)
	} else if p.TikTokRadarTrendEvidence != "" {
		trendScore = 80.0
	}
	eval.TrendScore = trendScore

	// 4. Youth & Channel Audience Fit Axis (Price tier vs Gen Z purchasing power)
	youthScore := 70.0
	if p.Price > 0 {
		if p.Price <= opts.YouthPriceCeiling {
			// Optimal price for youth impulse buy (< 350k VND)
			youthScore = 95.0
		} else if p.Price <= opts.YouthPriceCeiling*1.8 {
			// Moderate price (350k - 630k)
			youthScore = 75.0
		} else if p.Price <= opts.YouthPriceCeiling*3.0 {
			// High price (630k - 1.05M)
			youthScore = 50.0
		} else {
			// Luxury / Too expensive for youth impulse buy (> 1M VND)
			youthScore = 30.0
			eval.Verdict = ReviewRejected
			eval.RejectionCode = "PRICE_TOO_HIGH_FOR_YOUTH"
			eval.RejectionReason = fmt.Sprintf("Giá niêm yết (%.0f %s) vượt quá ngưỡng chi tiêu nhanh của giới trẻ (trần: %.0f %s).",
				p.Price, p.Currency, opts.YouthPriceCeiling, p.Currency)
			eval.YouthAudienceScore = youthScore
			return eval
		}
	}
	eval.YouthAudienceScore = youthScore

	// 5. Video-ability & Viral Propensity Axis
	// Evaluates how easily the product can be demonstrated visually in 9:16 short video
	videoScore := 70.0
	category := strings.ToLower(p.Category + " " + p.Theme + " " + p.Title)
	switch {
	case strings.Contains(category, "son") || strings.Contains(category, "kem") || strings.Contains(category, "mỹ phẩm") || strings.Contains(category, "beauty"):
		videoScore = 95.0 // Swatch, before/after, skincare
	case strings.Contains(category, "áo") || strings.Contains(category, "quần") || strings.Contains(category, "váy") || strings.Contains(category, "thời trang"):
		videoScore = 90.0 // Try-on, styling, transition
	case strings.Contains(category, "tai nghe") || strings.Contains(category, "ốp") || strings.Contains(category, "công nghệ") || strings.Contains(category, "phụ kiện"):
		videoScore = 88.0 // Unboxing, sound test, aesthetic desk setup
	case strings.Contains(category, "gia dụng") || strings.Contains(category, "bình nước") || strings.Contains(category, "decor"):
		videoScore = 85.0 // Demo utility
	case strings.Contains(category, "sách") || strings.Contains(category, "tài liệu"):
		videoScore = 50.0
	}
	eval.VideoabilityScore = videoScore

	// Calculate weighted total score
	eval.TotalScore = 0.30*eval.OfferScore + 0.20*eval.DemandScore + 0.15*eval.TrendScore + 0.20*eval.YouthAudienceScore + 0.15*eval.VideoabilityScore

	if eval.TotalScore >= opts.ScoreThreshold {
		eval.Verdict = ReviewApproved
		eval.EvidenceSummary = fmt.Sprintf("Đạt tiêu chuẩn tự động (Tổng điểm: %.1f/100). Hoa hồng: %.1f%%, Phù hợp giới trẻ: %.0f/100, Trực quan video: %.0f/100.",
			eval.TotalScore, commRate*100, eval.YouthAudienceScore, eval.VideoabilityScore)
	} else {
		eval.Verdict = ReviewRejected
		eval.RejectionCode = "SCORE_BELOW_THRESHOLD"
		eval.RejectionReason = fmt.Sprintf("Tổng điểm tiềm năng affiliate đạt %.1f/100 (ngưỡng duyệt: %.1f). Cần tối ưu thêm hoa hồng hoặc bằng chứng nhu cầu.",
			eval.TotalScore, opts.ScoreThreshold)
	}

	return eval
}

func (p Product) CandidateEvaluation() CandidateEvaluation {
	return EvaluateCandidate(p, time.Now())
}

func formatSoldCount(count int64) string {
	return fmt.Sprintf("%d", count)
}

func formatRating(rating float64) string {
	return fmt.Sprintf("%.1f", rating)
}

func formatDays(days float64) string {
	return fmt.Sprintf("%.1f", days)
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

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
	Recommendation       string
	RecommendationTone   string
	OfferReady           bool
	OfferGaps            []string
	CommissionPerOrder   float64
	CommissionFloor      float64
	HasCommissionFloor   bool
	BelowCommissionFloor bool
	DemandVerified       bool
	SalesEvidence        string
	RatingEvidence       string
	TrendEvidence        string
	AudienceEvidence     string
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
	case p.SoldCountKnown && validDemandEvidenceURL(p.DemandEvidenceURL) && p.DemandEvidenceAt != "":
		e.SalesEvidence = "Nguồn ghi nhận " + formatSoldCount(p.SoldCount) + " lượt bán tại thời điểm " + strings.TrimSpace(p.DemandEvidenceAt) + "."
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

func (p Product) CandidateEvaluation() CandidateEvaluation {
	return EvaluateCandidate(p, time.Now())
}

func formatSoldCount(count int64) string {
	return fmt.Sprintf("%d", count)
}

func formatRating(rating float64) string {
	return fmt.Sprintf("%.1f", rating)
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

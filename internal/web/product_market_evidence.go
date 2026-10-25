package web

import (
	"context"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"golang.org/x/text/unicode/norm"
)

const productTrendLookback = 7 * 24 * time.Hour

var tiktokTrendHashtagPattern = regexp.MustCompile(`#([\p{L}\p{M}\p{N}_]+)`)

// attachTikTokRadarEvidence adds recent Radar evidence to display copies only.
func (s *Server) attachTikTokRadarEvidence(ctx context.Context, candidates []products.Product) []products.Product {
	if s == nil || s.Studio == nil || len(candidates) == 0 {
		return candidates
	}
	videos, err := s.Studio.RecentTikTokRadarVideos(ctx, time.Now().Add(-productTrendLookback), 500)
	if err != nil {
		log.Printf("web: đọc evidence TikTok Radar cho sản phẩm: %v", err)
		return candidates
	}
	for i := range candidates {
		evidence := productTikTokTrendEvidence(candidates[i], videos)
		candidates[i].TikTokRadarTrendEvidence = evidence.Text
		candidates[i].TikTokRadarTrendURL = evidence.VideoURL
	}
	return candidates
}

type productTrendEvidence struct {
	Text     string
	VideoURL string
}

func productTikTokTrendEvidence(product products.Product, videos []studio.TikTokTrendVideo) productTrendEvidence {
	productTerm := normalizeTrendText(product.Title)
	categoryTerms := productCategoryTrendTerms(product)
	if len(videos) == 0 {
		return productTrendEvidence{Text: "Radar chưa có video thu thập trong 7 ngày; chưa đủ dữ liệu để đánh giá xu hướng."}
	}
	if productTerm == "" && len(categoryTerms) == 0 {
		return productTrendEvidence{Text: "Chưa có từ khóa category để đối chiếu với video TikTok Radar."}
	}

	productMatched := 0
	categoryMatched := 0
	matchedTerms := make(map[string]bool)
	var leadingProduct, leadingCategory studio.TikTokTrendVideo
	for _, video := range videos {
		text := video.Title + " " + video.Caption
		productHit := productTerm != "" && trendTextMatches(text, productTerm)
		categoryHit := false
		for _, term := range categoryTerms {
			if trendTextMatches(text, term) {
				categoryHit = true
				matchedTerms[term] = true
			}
		}
		if productHit {
			productMatched++
			if productMatched == 1 || radarVideoLeads(video, leadingProduct) {
				leadingProduct = video
			}
		}
		if categoryHit {
			categoryMatched++
			if categoryMatched == 1 || radarVideoLeads(video, leadingCategory) {
				leadingCategory = video
			}
		}
	}

	if productMatched == 0 && categoryMatched == 0 {
		termsHint := strings.Join(categoryTerms, ", ")
		if termsHint == "" {
			termsHint = "chưa có từ khóa ngành hàng"
		}
		return productTrendEvidence{Text: fmt.Sprintf(
			"Radar đã thu thập %d video trong 7 ngày nhưng chưa thấy title/caption khớp tên sản phẩm hoặc từ khóa ngành hàng (%s); mẫu này chưa đủ để kết luận sản phẩm không phù hợp.",
			len(videos), termsHint)}
	}

	var hitTerms []string
	for _, term := range categoryTerms {
		if matchedTerms[term] {
			hitTerms = append(hitTerms, term)
			if len(hitTerms) == 3 {
				break
			}
		}
	}
	detail := fmt.Sprintf("Radar 7 ngày: %d video khớp tên sản phẩm", productMatched)
	if len(categoryTerms) > 0 {
		categoryHits := strings.Join(hitTerms, ", ")
		if categoryHits == "" {
			categoryHits = "chưa có từ khóa khớp"
		}
		detail += fmt.Sprintf("; %d video khớp từ khóa ngành hàng (%s)", categoryMatched, categoryHits)
	}
	detail += ". Tín hiệu nội dung không xác minh lượt bán."
	leading := leadingProduct
	leadingGroup := "tên sản phẩm"
	if productMatched == 0 {
		leading = leadingCategory
		leadingGroup = "ngành hàng"
	}
	if leading.VelocityKnown && finiteTrendMetric(leading.ViewsPerHour) {
		detail += fmt.Sprintf(" Video dẫn đầu nhóm khớp %s tăng khoảng %.0f lượt xem/giờ.", leadingGroup, leading.ViewsPerHour)
	} else if leading.PlayCountKnown {
		detail += fmt.Sprintf(" Video dẫn đầu nhóm khớp %s có %s lượt xem hiện tại; chưa có tốc độ tăng view.", leadingGroup, studio.FormatCount(leading.PlayCount))
	} else {
		detail += fmt.Sprintf(" Radar chưa có số lượt xem hoặc tốc độ tăng view cho video khớp %s.", leadingGroup)
	}
	return productTrendEvidence{Text: detail, VideoURL: leading.VideoURL}
}

func productCategoryTrendTerms(product products.Product) []string {
	var rawTerms []string
	if theme, ok := products.DescribeTheme(strings.TrimSpace(product.Theme)); ok {
		rawTerms = append(rawTerms, theme.Keywords...)
	}
	if strings.TrimSpace(product.Category) != "" {
		rawTerms = append(rawTerms, product.Category)
	}
	terms := make([]string, 0, len(rawTerms))
	seen := make(map[string]bool, len(rawTerms))
	for _, raw := range rawTerms {
		term := normalizeTrendText(raw)
		if len(term) < 3 || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func trendTextMatches(text, term string) bool {
	text = strings.TrimSpace(text)
	term = normalizeTrendText(term)
	if text == "" || term == "" {
		return false
	}
	normalizedText := normalizeTrendText(text)
	if strings.Contains(" "+normalizedText+" ", " "+term+" ") {
		return true
	}
	// Captions often join Vietnamese hashtags (for example #mypham), but
	// substring matching ordinary words creates false trend evidence.
	compactTerm := strings.ReplaceAll(term, " ", "")
	for _, match := range tiktokTrendHashtagPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 && normalizeTrendText(match[1]) == compactTerm {
			return true
		}
	}
	return false
}

func normalizeTrendText(value string) string {
	value = strings.ToLower(norm.NFD.String(value))
	var out strings.Builder
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func radarVideoLeads(candidate, current studio.TikTokTrendVideo) bool {
	if candidate.VelocityKnown && finiteTrendMetric(candidate.ViewsPerHour) {
		if !current.VelocityKnown || !finiteTrendMetric(current.ViewsPerHour) {
			return true
		}
		if candidate.ViewsPerHour != current.ViewsPerHour {
			return candidate.ViewsPerHour > current.ViewsPerHour
		}
	} else if current.VelocityKnown && finiteTrendMetric(current.ViewsPerHour) {
		return false
	}
	if candidate.PlayCountKnown && (!current.PlayCountKnown || candidate.PlayCount > current.PlayCount) {
		return true
	}
	return !current.PlayCountKnown && candidate.CollectedAt > current.CollectedAt
}

func finiteTrendMetric(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

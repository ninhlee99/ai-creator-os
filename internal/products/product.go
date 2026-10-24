package products

import (
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Product is one affiliate candidate from a provider.
type Product struct {
	ID                      int64
	Source                  string // provider name, e.g. "accesstrade", "manual"
	SourceID                string // provider-side product id
	Title                   string
	ImageURLs               []string // listing photos (ground truth for product lock)
	Price                   float64
	PriceMax                float64
	OriginalPrice           float64 // provider-supplied list price; 0 means unknown
	Currency                string
	CommissionRate          float64 // 0..1 — the king metric
	ShopName                string
	Rating                  float64 // 0..5
	SoldCount               int64
	SoldCountKnown          bool   // provider/user explicitly supplied count; zero can be a real value
	DemandEvidenceURL       string // listing page where rating and sold count were observed
	DemandEvidenceAt        string // RFC3339 capture time; empty means unknown
	DemandEvidenceRating    float64
	DemandEvidenceSoldCount int64
	Category                string
	ProductURL              string
	ProductPageURL          string // provider listing URL, not affiliate tracking URL
	Theme                   string // theme it was discovered under
	FoundAt                 string
	DataUpdatedAt           string // provider timestamp; empty means unknown
	// Shelf state (R2-W1): "shelf"/"scaled" = curated onto the Kệ hàng
	// shelf; "" = discovery pool only. Lives in this same store so the
	// autopilot sees shelf-added products immediately.
	ShelfStatus  string
	ShelfScore   float64
	ReviewStatus string // pending | approved | rejected
	ReviewNote   string
	ReviewedAt   string
	// TikTok Radar evidence is attached only for web evaluation; it is not persisted.
	TikTokRadarTrendEvidence string `json:"-"`
	TikTokRadarTrendURL      string `json:"-"`
}

const (
	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"
)

// Query describes what the autopilot is hunting.
type Query struct {
	Theme         string
	Keywords      []string
	MinCommission float64 // 0..1, e.g. 0.10 = 10%
	MinSold       int64
	MaxPrice      float64 // 0 = no cap
	Limit         int
}

// HasDemandSignals reports whether product has provider-sourced sales and
// rating values tied to a recent HTTPS source. Manual values and URLs are
// user-reported; they do not establish sales scope, review sample size, or
// independent verification, so they cannot pass the automated affiliate gate.
func (p Product) HasDemandSignals() bool {
	return p.HasDemandSignalsAt(time.Now())
}

// HasDemandSignalsAt is the time-stable form used by views and decision paths.
func (p Product) HasDemandSignalsAt(now time.Time) bool {
	source := strings.ToLower(strings.TrimSpace(p.Source))
	if source == "" || source == "manual" || p.SoldCount <= 0 || math.IsNaN(p.Rating) || math.IsInf(p.Rating, 0) ||
		p.Rating <= 0 || p.Rating > 5 || !validDemandEvidenceURL(p.DemandEvidenceURL) {
		return false
	}
	if p.DemandEvidenceRating != p.Rating || p.DemandEvidenceSoldCount != p.SoldCount {
		return false
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(p.DemandEvidenceAt))
	if err != nil || capturedAt.IsZero() {
		return false
	}
	age := now.Sub(capturedAt)
	return age >= -5*time.Minute && age <= 30*24*time.Hour
}

// ReadyForAffiliateProduction requires explicit human approval and a complete
// offer. Missing provider sales or rating fields remain visible evidence gaps;
// they do not silently overrule the operator's review decision.
func (p Product) ReadyForAffiliateProduction() bool {
	return p.ReviewStatus == ReviewApproved && EvaluateCandidate(p, time.Now()).OfferReady
}

func validDemandEvidenceURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

// Matches reports whether p satisfies the hard filters of q.
func (p Product) Matches(q Query) bool {
	if q.MinCommission > 0 && p.CommissionRate < q.MinCommission {
		return false
	}
	if q.MinSold > 0 && (!p.HasDemandSignals() || p.SoldCount < q.MinSold) {
		return false
	}
	if q.MaxPrice > 0 && p.Price > q.MaxPrice {
		return false
	}
	return true
}

// Score ranks candidates. Commission dominates (60 pts), because Ninh's
// rule is "hoa hồng cao nhất"; sales velocity (25) and rating (15) break
// ties so we don't pick a 50%-commission product nobody buys.
func Score(p Product) float64 {
	comm := math.Min(p.CommissionRate, 0.5) / 0.5 * 60
	sold := 0.0
	rating := 0.0
	if p.HasDemandSignals() {
		sold = math.Min(math.Log10(float64(p.SoldCount)+1)/4, 1) * 25 // 10k sold ≈ max
		rating = math.Min(math.Max(p.Rating, 0), 5) / 5 * 15
	}
	return comm + sold + rating
}

// Rank sorts products best-first (stable, so equal scores keep discovery order).
func Rank(in []Product) []Product {
	out := make([]Product, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return Score(out[i]) > Score(out[j]) })
	return out
}

// Filter keeps only products matching q's hard filters.
func Filter(in []Product, q Query) []Product {
	var out []Product
	for _, p := range in {
		if p.Matches(q) {
			out = append(out, p)
		}
	}
	return out
}

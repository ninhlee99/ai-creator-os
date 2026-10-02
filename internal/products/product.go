package products

import (
	"math"
	"sort"
)

// Product is one affiliate candidate from a provider.
type Product struct {
	ID             int64
	Source         string // provider name, e.g. "tiktok_shop"
	SourceID       string // provider-side product id
	Title          string
	ImageURLs      []string // listing photos (ground truth for product lock)
	Price          float64
	Currency       string
	CommissionRate float64 // 0..1 — the king metric
	ShopName       string
	Rating         float64 // 0..5
	SoldCount      int64
	Category       string
	ProductURL     string
	Theme          string // theme it was discovered under
	FoundAt        string
	// Shelf state (R2-W1): "shelf"/"scaled" = curated onto the Kệ hàng
	// shelf; "" = discovery pool only. Lives in this same store so the
	// autopilot sees shelf-added products immediately.
	ShelfStatus string
	ShelfScore  float64
}

// Query describes what the autopilot is hunting.
type Query struct {
	Theme         string
	Keywords      []string
	MinCommission float64 // 0..1, e.g. 0.10 = 10%
	MinSold       int64
	MaxPrice      float64 // 0 = no cap
	Limit         int
}

// Matches reports whether p satisfies the hard filters of q.
func (p Product) Matches(q Query) bool {
	if q.MinCommission > 0 && p.CommissionRate < q.MinCommission {
		return false
	}
	if q.MinSold > 0 && p.SoldCount < q.MinSold {
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
	if p.SoldCount > 0 {
		sold = math.Min(math.Log10(float64(p.SoldCount)+1)/4, 1) * 25 // 10k sold ≈ max
	}
	rating := math.Min(math.Max(p.Rating, 0), 5) / 5 * 15
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

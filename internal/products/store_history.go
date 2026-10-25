package products

import (
	"encoding/json"
	"fmt"
)

type productSnapshotRecord struct {
	Title                   string  `json:"title"`
	Price                   float64 `json:"price"`
	PriceMax                float64 `json:"price_max"`
	OriginalPrice           float64 `json:"original_price"`
	Currency                string  `json:"currency"`
	CommissionRate          float64 `json:"commission_rate"`
	CommissionAmount        float64 `json:"commission_amount"`
	CommissionCurrency      string  `json:"commission_currency"`
	ShopName                string  `json:"shop_name"`
	Rating                  float64 `json:"rating"`
	SoldCount               int64   `json:"sold_count"`
	SoldCountKnown          bool    `json:"sold_count_known"`
	DemandEvidenceURL       string  `json:"demand_evidence_url"`
	DemandEvidenceAt        string  `json:"demand_evidence_at"`
	DemandEvidenceRating    float64 `json:"demand_evidence_rating"`
	DemandEvidenceSoldCount int64   `json:"demand_evidence_sold_count"`
	Category                string  `json:"category"`
}

func productSnapshotData(p Product) productSnapshotRecord {
	return productSnapshotRecord{
		Title: p.Title, Price: p.Price, PriceMax: p.PriceMax, OriginalPrice: p.OriginalPrice,
		Currency: p.Currency, CommissionRate: p.CommissionRate,
		CommissionAmount: p.CommissionAmount, CommissionCurrency: p.CommissionCurrency, ShopName: p.ShopName,
		Rating: p.Rating, SoldCount: p.SoldCount, SoldCountKnown: p.SoldCountKnown,
		DemandEvidenceURL: p.DemandEvidenceURL, DemandEvidenceAt: p.DemandEvidenceAt,
		DemandEvidenceRating: p.DemandEvidenceRating, DemandEvidenceSoldCount: p.DemandEvidenceSoldCount,
		Category: p.Category,
	}
}

type ProductSnapshot struct {
	CapturedAt string
	Product    Product
}

type ProductReviewEvent struct {
	Status     string
	Note       string
	ReviewedAt string
}

type ProductHistory struct {
	Snapshots []ProductSnapshot
	Reviews   []ProductReviewEvent
}

// History returns recent distinct offer observations and review decisions.
func (s *Store) History(id int64, limit int) (ProductHistory, error) {
	var history ProductHistory
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM products WHERE id=?`, id).Scan(&exists); err != nil {
		return history, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT captured_at,payload FROM product_snapshots WHERE product_id=? ORDER BY id DESC LIMIT ?`, id, limit)
	if err != nil {
		return history, err
	}
	for rows.Next() {
		var item ProductSnapshot
		var payload string
		if err := rows.Scan(&item.CapturedAt, &payload); err != nil {
			rows.Close()
			return history, err
		}
		var data productSnapshotRecord
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			rows.Close()
			return history, fmt.Errorf("products: decode snapshot: %w", err)
		}
		item.Product = Product{
			Title: data.Title, Price: data.Price, PriceMax: data.PriceMax, OriginalPrice: data.OriginalPrice,
			Currency: data.Currency, CommissionRate: data.CommissionRate,
			CommissionAmount: data.CommissionAmount, CommissionCurrency: data.CommissionCurrency, ShopName: data.ShopName,
			Rating: data.Rating, SoldCount: data.SoldCount, SoldCountKnown: data.SoldCountKnown,
			DemandEvidenceURL: data.DemandEvidenceURL, DemandEvidenceAt: data.DemandEvidenceAt,
			DemandEvidenceRating: data.DemandEvidenceRating, DemandEvidenceSoldCount: data.DemandEvidenceSoldCount,
			Category: data.Category,
		}
		history.Snapshots = append(history.Snapshots, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return history, err
	}
	if err := rows.Close(); err != nil {
		return history, err
	}
	rows, err = s.db.Query(`SELECT status,note,reviewed_at FROM product_review_history WHERE product_id=? ORDER BY id DESC LIMIT ?`, id, limit)
	if err != nil {
		return history, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProductReviewEvent
		if err := rows.Scan(&item.Status, &item.Note, &item.ReviewedAt); err != nil {
			return history, err
		}
		history.Reviews = append(history.Reviews, item)
	}
	return history, rows.Err()
}

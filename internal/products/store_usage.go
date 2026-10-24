package products

import (
	"fmt"
	"time"
)

func (s *Store) RecordUse(productID, accountID int64, jobID string) (bool, error) {
	res, err := s.db.Exec(
		`INSERT INTO product_usage(product_id,account_id,job_id,used_at) VALUES(?,?,?,?)
		ON CONFLICT(product_id, account_id) DO NOTHING`,
		productID, accountID, jobID, time.Now().Format("2006-01-02T15:04:05"))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReleaseUseClaim releases a reservation only when no render job was linked.
// Failed preflight must not permanently hide a product from later channel runs.
func (s *Store) ReleaseUseClaim(productID, accountID int64) error {
	_, err := s.db.Exec(`DELETE FROM product_usage
		WHERE product_id=? AND account_id=? AND COALESCE(job_id,'')=''`, productID, accountID)
	return err
}

// SetUsageJobID updates the job id on an existing usage claim (used when
// the product was claimed before its studio job existed).
func (s *Store) SetUsageJobID(productID, accountID int64, jobID string) error {
	_, err := s.db.Exec(
		`UPDATE product_usage SET job_id=? WHERE product_id=? AND account_id=?`,
		jobID, productID, accountID)
	return err
}

// SetShelf marks a product's shelf state ("shelf"/"scaled") with its
// curated score, or removes it from the shelf with an empty status.
// Shelf state lives in this same store so the autopilot (TopByTheme) sees
// shelf-curated products immediately.
func (s *Store) SetShelf(productID int64, status string, score float64) error {
	if status == "shelf" || status == "scaled" {
		_, err := s.db.Exec(`UPDATE products SET shelf_status=?, shelf_score=?,
			review_status='approved', review_note='Được chọn vào kệ', reviewed_at=? WHERE id=?`,
			status, score, time.Now().UTC().Format(time.RFC3339), productID)
		return err
	}
	if status != "" {
		return fmt.Errorf("products: invalid shelf status %q", status)
	}
	_, err := s.db.Exec(`UPDATE products SET shelf_status='', shelf_score=0,
		review_status='pending', review_note='', reviewed_at='' WHERE id=?`, productID)
	return err
}

// Shelf returns products curated onto the shelf (status shelf/scaled),
// curated score first.
func (s *Store) Shelf(limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT `+productCols+` FROM products
		WHERE shelf_status IN ('shelf','scaled') AND review_status='approved'
		ORDER BY shelf_score DESC, commission_rate DESC, id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// All returns up to limit products, best first by Score (API surface).

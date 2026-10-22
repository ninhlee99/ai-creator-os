package products

import (
	"strings"
)

func (s *Store) All(limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT ` + productCols + ` FROM products ORDER BY id`)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ranked := Rank(out)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

// GetBySource fetches one product by its provider coordinates.
func (s *Store) GetBySource(source, sourceID string) (Product, error) {
	p, err := scanProduct(s.db.QueryRow(
		`SELECT `+productCols+` FROM products WHERE source=? AND source_id=?`,
		source, sourceID))
	return p, err
}

// GetByID fetches a single product by its primary key.
func (s *Store) GetByID(id int64) (Product, error) {
	p, err := scanProduct(s.db.QueryRow(
		`SELECT `+productCols+` FROM products WHERE id=?`, id))
	return p, err
}

// SearchThemes lists distinct themes with product counts (for the UI).
func (s *Store) SearchThemes() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT theme FROM products WHERE theme != '' ORDER BY theme`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil && strings.TrimSpace(t) != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

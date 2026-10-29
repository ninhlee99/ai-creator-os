package products

import (
	"encoding/json"
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

// AddProductImages bổ sung thêm các hình ảnh mới vào sản phẩm đã có.
func (s *Store) AddProductImages(id int64, newImages []string) error {
	if len(newImages) == 0 {
		return nil
	}
	p, err := s.GetByID(id)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, img := range p.ImageURLs {
		seen[img] = true
	}
	for _, img := range newImages {
		img = strings.TrimSpace(img)
		if img != "" && !seen[img] {
			seen[img] = true
			p.ImageURLs = append(p.ImageURLs, img)
		}
	}
	imgs, err := json.Marshal(p.ImageURLs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE products SET image_urls = ? WHERE id = ?`, string(imgs), id)
	return err
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

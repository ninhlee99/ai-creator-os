package products

import (
	"strings"
)

func (s *Store) TopByTheme(theme string, minCommission float64, accountID int64, limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT `+productCols+` FROM products
		WHERE theme = ? AND commission_rate >= ?
		  AND id NOT IN (SELECT product_id FROM product_usage WHERE account_id = ?)`,
		theme, minCommission, accountID)
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
	ranked := Rank(out)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

// Search queries products matching a keyword and/or theme, with optional minCommission floor.
func (s *Store) Search(keyword, theme string, minCommission float64, limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 50
	}
	var where []string
	var args []any
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		where = append(where, "(LOWER(title) LIKE ? OR LOWER(category) LIKE ? OR LOWER(shop_name) LIKE ?)")
		args = append(args, like, like, like)
	}
	if theme = strings.TrimSpace(theme); theme != "" {
		where = append(where, "theme = ?")
		args = append(args, theme)
	}
	if minCommission > 0 {
		where = append(where, "commission_rate >= ?")
		args = append(args, minCommission)
	}
	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}
	q := `SELECT ` + productCols + ` FROM products ` + clause + ` ORDER BY commission_rate DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
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

// TopHighCommission returns products with high commission rate (>= 15% or highest available).
func (s *Store) TopHighCommission(limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT `+productCols+` FROM products WHERE commission_rate >= 0.15 ORDER BY commission_rate DESC, id DESC LIMIT ?`, limit)
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

// TopUnusedBySource trả về sản phẩm tốt nhất (chưa dùng cho accountID)
// của một nguồn (vd "accesstrade"), sort theo Score. Hunter Accesstrade
// dùng accountID 0 cho video trực tiếp — không va chạm usage theo account.
func (s *Store) TopUnusedBySource(source string, minCommission float64, accountID int64, limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT `+productCols+` FROM products
		WHERE source = ? AND commission_rate >= ?
		  AND id NOT IN (SELECT product_id FROM product_usage WHERE account_id = ?)`,
		source, minCommission, accountID)
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
	ranked := Rank(out)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, rows.Err()
}

// RecordUse marks a product as promoted for an account (by studio job id).
// Đợt Y: INSERT ... ON CONFLICT DO NOTHING — trả về claimed=true chỉ khi
// dòng được chèn thật (chống 2 chu kỳ cùng chọn 1 sản phẩm cho 1 account).

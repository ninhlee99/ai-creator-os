package products

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (s *Store) SetAffiliateLink(id int64, raw string) error {
	link := strings.TrimSpace(raw)
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("products: affiliate link phải là HTTPS hợp lệ")
	}
	res, err := s.db.Exec(`UPDATE products SET product_url=? WHERE id=?`, link, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ReviewProduct records an operator's decision. Approved offers enter the
// shelf; rejected offers remain searchable in the review history.
func (s *Store) ReviewProduct(id int64, status, note string) error {
	status = strings.TrimSpace(status)
	note = strings.TrimSpace(note)
	switch status {
	case ReviewPending:
		note = ""
	case ReviewApproved:
		if note == "" {
			note = "Duyệt thủ công"
		}
	case ReviewRejected:
		if note == "" {
			return fmt.Errorf("products: lý do loại sản phẩm là bắt buộc")
		}
	default:
		return fmt.Errorf("products: trạng thái duyệt không hợp lệ")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p, err := scanProduct(tx.QueryRow(`SELECT `+productCols+` FROM products WHERE id=?`, id))
	if err != nil {
		return err
	}
	shelfStatus, shelfScore := "", 0.0
	if status == ReviewApproved {
		shelfStatus, shelfScore = "shelf", Score(p)
	}
	reviewedAt := time.Now().UTC().Format(time.RFC3339)
	if status == ReviewPending {
		reviewedAt = ""
	}
	res, err := tx.Exec(`UPDATE products SET review_status=?, review_note=?, reviewed_at=?,
		shelf_status=?, shelf_score=? WHERE id=?`, status, note, reviewedAt, shelfStatus, shelfScore, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	payload, err := json.Marshal(productSnapshotData(p))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO product_review_history(product_id,status,note,reviewed_at,payload) VALUES(?,?,?,?,?)`,
		id, status, note, time.Now().UTC().Format(time.RFC3339Nano), string(payload)); err != nil {
		return err
	}
	return tx.Commit()
}

// ReviewList returns products in one explicit review state.
func (s *Store) ReviewList(status, keyword, theme string, limit int) ([]Product, error) {
	if status != ReviewPending && status != ReviewApproved && status != ReviewRejected {
		return nil, fmt.Errorf("products: trạng thái duyệt không hợp lệ")
	}
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + productCols + ` FROM products WHERE review_status=?`
	args := []any{status}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		query += ` AND (LOWER(title) LIKE ? OR LOWER(category) LIKE ? OR LOWER(shop_name) LIKE ?)`
		args = append(args, like, like, like)
	}
	if theme = strings.TrimSpace(theme); theme != "" {
		query += ` AND theme=?`
		args = append(args, theme)
	}
	query += ` ORDER BY commission_rate DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
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

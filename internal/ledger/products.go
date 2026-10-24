package ledger

import (
	"database/sql"
	"errors"
	"strings"
)

func (l *Ledger) UpsertProduct(platformPID string, fields map[string]any) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var id int64
	err := l.db.QueryRow(
		"SELECT id FROM products WHERE platform_pid = ?", platformPID).Scan(&id)
	switch {
	case err == nil:
		var sets []string
		var vals []any
		for k, v := range fields {
			if k == "platform_pid" {
				continue
			}
			sets = append(sets, k+" = ?")
			vals = append(vals, normalize(v))
		}
		vals = append(vals, platformPID)
		_, err := l.db.Exec(
			"UPDATE products SET "+strings.Join(sets, ", ")+
				", updated_at = datetime('now') WHERE platform_pid = ?",
			vals...,
		)
		if err != nil {
			return 0, err
		}
		return id, nil
	case errors.Is(err, sql.ErrNoRows):
		fields["platform_pid"] = platformPID
		return l.insertLocked("products", fields)
	default:
		return 0, err
	}
}

// SetProductStatus updates a product's status.
func (l *Ledger) SetProductStatus(productID int64, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE products SET status = ?, updated_at = datetime('now') WHERE id = ?",
		status, productID)
	return err
}

// ShelfProducts returns up to limit products with status shelf/scaled,
// best score first.
func (l *Ledger) ShelfProducts(limit int) ([]Product, error) {
	rows, err := l.db.Query(
		"SELECT id, platform_pid, title, category, price, commission_rate, "+
			"commission_value, seller_rating, score, status, created_at, updated_at "+
			"FROM products WHERE status IN ('shelf','scaled') "+
			"ORDER BY score DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

// AddProduct inserts a new product with status 'shelf', computing
// commission_value = round(price * commission_rate, 2).
func (l *Ledger) AddProduct(platformPID, title string, price, commissionRate float64, category string) (int64, error) {
	// Python: round(price * commission_rate, 2)
	cv := float64(int(price*commissionRate*100+0.5)) / 100
	return l.insert("products", map[string]any{
		"platform_pid":     platformPID,
		"title":            title,
		"category":         category,
		"price":            price,
		"commission_rate":  commissionRate,
		"commission_value": cv,
		"status":           "shelf",
	})
}

// GetProducts returns all products ordered by score DESC, id.
func (l *Ledger) GetProducts() ([]Product, error) {
	rows, err := l.db.Query(
		"SELECT id, platform_pid, title, category, price, commission_rate, " +
			"commission_value, seller_rating, score, status, created_at, updated_at " +
			"FROM products ORDER BY score DESC, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

func scanProducts(rows *sql.Rows) ([]Product, error) {
	var out []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.PlatformPID, &p.Title, &p.Category,
			&p.Price, &p.CommissionRate, &p.CommissionValue, &p.SellerRating,
			&p.Score, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- append-only money ----

// RecordOrder records an order. Idempotent: a duplicate platform_oid
// returns (0, nil) instead of an error.
func (l *Ledger) RecordOrder(platformOID string, fields map[string]any) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fields["platform_oid"] = platformOID
	id, err := l.insertLocked("orders", fields)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, nil // duplicate platform_oid: ignored, like Python
		}
		return 0, err
	}
	return id, nil
}

// RecordCommission records a commission settlement.
func (l *Ledger) RecordCommission(period string, amount float64, source string) (int64, error) {
	return l.insert("commissions", map[string]any{
		"period": period, "amount": amount, "source": source,
	})
}

// ---- decisions / usage ----

// Decide records a governance decision with JSON inputs (audit trail).
func (l *Ledger) Decide(agent, action string, target *string, reason string, inputs map[string]any) error {
	ij, err := toJSON(inputs)
	if err != nil {
		return err
	}
	_, err = l.insert("decisions", map[string]any{
		"agent": agent, "action": action, "target": target,
		"reason": reason, "inputs_json": ij,
	})
	return err
}

// LogUsage meters engine usage (free-tier caps).
func (l *Ledger) LogUsage(engine, provider string, units, costUSD float64) error {
	_, err := l.insert("api_usage", map[string]any{
		"engine": engine, "provider": provider,
		"units": units, "cost_usd": costUSD,
	})
	return err
}

// DailySpendUSD returns today's total API spend in USD.
func (l *Ledger) DailySpendUSD() (float64, error) {
	var s float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd),0) AS s FROM api_usage " +
			"WHERE date(created_at) = date('now')").Scan(&s)
	return s, err
}

// ---- analyst queries ----

// ProductStats aggregates orders and content views for a product.
func (l *Ledger) ProductStats(productID int64) (ProductStats, error) {
	var st ProductStats
	err := l.db.QueryRow(
		"SELECT COUNT(*) AS orders, COALESCE(SUM(amount),0) AS revenue, "+
			"COALESCE(SUM(commission),0) AS commission "+
			"FROM orders WHERE product_id = ?", productID).
		Scan(&st.Orders, &st.Revenue, &st.Commission)
	if err != nil {
		return st, err
	}
	err = l.db.QueryRow(
		"SELECT COALESCE(SUM(views),0) AS v FROM content_items WHERE product_id = ?",
		productID).Scan(&st.Views)
	return st, err
}

// ---- accounts (AI Creator Network) ----

// AddAccount registers a TikTok account. A duplicate username returns the
// existing id.

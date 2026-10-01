package products

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver
)

// Store persists discovered products and which account used which product
// (so autopilot doesn't promote the same product twice for one account).
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the product database.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("products: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("products: pragma %q: %w", p, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS products(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL, source_id TEXT NOT NULL,
		title TEXT NOT NULL, image_urls TEXT NOT NULL DEFAULT '[]',
		price REAL NOT NULL DEFAULT 0, currency TEXT NOT NULL DEFAULT '',
		commission_rate REAL NOT NULL DEFAULT 0,
		shop_name TEXT NOT NULL DEFAULT '', rating REAL NOT NULL DEFAULT 0,
		sold_count INTEGER NOT NULL DEFAULT 0, category TEXT NOT NULL DEFAULT '',
		product_url TEXT NOT NULL DEFAULT '', theme TEXT NOT NULL DEFAULT '',
		found_at TEXT NOT NULL,
		UNIQUE(source, source_id));
	CREATE INDEX IF NOT EXISTS idx_products_theme ON products(theme);
	CREATE TABLE IF NOT EXISTS product_usage(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL, account_id INTEGER NOT NULL,
		job_id TEXT NOT NULL DEFAULT '', used_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_usage_account ON product_usage(account_id);
	-- generic key/value settings (e.g. autopilot schedule) so the web UI —
	-- not env vars or CLI — is the control plane for operations.
	CREATE TABLE IF NOT EXISTS settings(
		key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');`)
	return err
}

// GetSetting reads a UI-managed setting. ok=false when unset.
func (s *Store) GetSetting(key string) (val string, ok bool) {
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&val)
	if err != nil {
		return "", false
	}
	return val, true
}

// SetSetting writes a UI-managed setting.
func (s *Store) SetSetting(key, val string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, val)
	return err
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Save inserts or refreshes a product (dedupe on source+source_id).
func (s *Store) Save(p Product) (int64, error) {
	imgs, _ := json.Marshal(p.ImageURLs)
	now := time.Now().Format("2006-01-02T15:04:05")
	res, err := s.db.Exec(`
		INSERT INTO products(source,source_id,title,image_urls,price,currency,
			commission_rate,shop_name,rating,sold_count,category,product_url,theme,found_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,source_id) DO UPDATE SET
			title=excluded.title, image_urls=excluded.image_urls, price=excluded.price,
			commission_rate=excluded.commission_rate, shop_name=excluded.shop_name,
			rating=excluded.rating, sold_count=excluded.sold_count,
			category=excluded.category, product_url=excluded.product_url,
			theme=excluded.theme, found_at=excluded.found_at`,
		p.Source, p.SourceID, p.Title, string(imgs), p.Price, p.Currency,
		p.CommissionRate, p.ShopName, p.Rating, p.SoldCount, p.Category,
		p.ProductURL, p.Theme, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		_ = s.db.QueryRow(`SELECT id FROM products WHERE source=? AND source_id=?`,
			p.Source, p.SourceID).Scan(&id)
	}
	return id, nil
}

func scanProduct(row interface {
	Scan(...any) error
}) (Product, error) {
	var p Product
	var imgs string
	err := row.Scan(&p.ID, &p.Source, &p.SourceID, &p.Title, &imgs, &p.Price,
		&p.Currency, &p.CommissionRate, &p.ShopName, &p.Rating, &p.SoldCount,
		&p.Category, &p.ProductURL, &p.Theme, &p.FoundAt)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(imgs), &p.ImageURLs)
	return p, nil
}

const productCols = `id,source,source_id,title,image_urls,price,currency,` +
	`commission_rate,shop_name,rating,sold_count,category,product_url,theme,found_at`

// TopByTheme returns the best unused-by-account products for a theme,
// ordered by score. Products already promoted for accountID are excluded.
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

// RecordUse marks a product as promoted for an account (by studio job id).
func (s *Store) RecordUse(productID, accountID int64, jobID string) error {
	_, err := s.db.Exec(
		`INSERT INTO product_usage(product_id,account_id,job_id,used_at) VALUES(?,?,?,?)`,
		productID, accountID, jobID, time.Now().Format("2006-01-02T15:04:05"))
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

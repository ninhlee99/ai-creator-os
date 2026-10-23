package products

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"strings"
	"time"
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
	if err := stampUserVersion(db, 1); err != nil {
		db.Close()
		return nil, fmt.Errorf("products: pragma user_version: %w", err)
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
		original_price REAL NOT NULL DEFAULT 0,
		data_updated_at TEXT NOT NULL DEFAULT '',
		commission_rate REAL NOT NULL DEFAULT 0,
		shop_name TEXT NOT NULL DEFAULT '', rating REAL NOT NULL DEFAULT 0,
		sold_count INTEGER NOT NULL DEFAULT 0,
		demand_evidence_url TEXT NOT NULL DEFAULT '', demand_evidence_at TEXT NOT NULL DEFAULT '',
		demand_evidence_rating REAL NOT NULL DEFAULT 0,
		demand_evidence_sold_count INTEGER NOT NULL DEFAULT 0,
		category TEXT NOT NULL DEFAULT '',
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
		key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
	CREATE TABLE IF NOT EXISTS bio_profiles(
		account_id INTEGER PRIMARY KEY,
		published INTEGER NOT NULL DEFAULT 0,
		description TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS bio_listings(
		account_id INTEGER NOT NULL,
		product_id INTEGER NOT NULL,
		published INTEGER NOT NULL DEFAULT 0,
		display_order INTEGER NOT NULL DEFAULT 0,
		voucher_code TEXT NOT NULL DEFAULT '',
		voucher_value REAL NOT NULL DEFAULT 0,
		voucher_ends_at TEXT NOT NULL DEFAULT '',
		voucher_terms TEXT NOT NULL DEFAULT '',
		product_details TEXT NOT NULL DEFAULT '',
		image_urls_override TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(account_id, product_id));
	CREATE INDEX IF NOT EXISTS idx_bio_listings_published ON bio_listings(account_id,published,display_order);
	CREATE TABLE IF NOT EXISTS bio_events(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		product_id INTEGER NOT NULL DEFAULT 0,
		event_type TEXT NOT NULL,
		local_day TEXT NOT NULL,
		created_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_bio_events_account_day ON bio_events(account_id,local_day,event_type);
	CREATE INDEX IF NOT EXISTS idx_bio_events_product_day ON bio_events(account_id,product_id,local_day,event_type);
	CREATE INDEX IF NOT EXISTS idx_bio_events_created_at ON bio_events(created_at);
	CREATE TABLE IF NOT EXISTS bio_event_dedupe(
		dedupe_key TEXT PRIMARY KEY,
		expires_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_bio_event_dedupe_expires ON bio_event_dedupe(expires_at);
	CREATE TABLE IF NOT EXISTS bio_tryon_jobs(
		id TEXT PRIMARY KEY,
		account_id INTEGER NOT NULL,
		buyer_google_account_id INTEGER NOT NULL DEFAULT 0,
		product_id INTEGER NOT NULL,
		ip_hash TEXT NOT NULL,
		device_hash TEXT NOT NULL,
		image_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		result_path TEXT NOT NULL DEFAULT '',
		error_code TEXT NOT NULL DEFAULT '',
		local_day TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		expires_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_bio_tryon_reuse ON bio_tryon_jobs(product_id,image_hash,status,expires_at);
	CREATE INDEX IF NOT EXISTS idx_bio_tryon_ip_product ON bio_tryon_jobs(ip_hash,product_id,created_at);
	CREATE INDEX IF NOT EXISTS idx_bio_tryon_device_product ON bio_tryon_jobs(device_hash,product_id,created_at);
	CREATE TABLE IF NOT EXISTS bio_google_accounts(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject_hash TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL DEFAULT '',
		display_name TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		last_login_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS bio_google_sessions(
		session_hash TEXT PRIMARY KEY,
		account_id INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_bio_google_sessions_expiry ON bio_google_sessions(expires_at);
	CREATE INDEX IF NOT EXISTS idx_bio_google_sessions_account ON bio_google_sessions(account_id);
	CREATE TABLE IF NOT EXISTS bio_tryon_account_product_usage(
		account_id INTEGER NOT NULL,
		product_id INTEGER NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(account_id,product_id));
	CREATE TABLE IF NOT EXISTS bio_tryon_account_daily_usage(
		account_id INTEGER NOT NULL,
		local_day TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(account_id,local_day));
	CREATE TABLE IF NOT EXISTS bio_tryon_product_usage(
		identity_type TEXT NOT NULL,
		identity_hash TEXT NOT NULL,
		product_id INTEGER NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(identity_type,identity_hash,product_id));
	CREATE TABLE IF NOT EXISTS bio_tryon_daily_usage(
		identity_type TEXT NOT NULL,
		identity_hash TEXT NOT NULL,
		local_day TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(identity_type,identity_hash,local_day));`)
	if err != nil {
		return err
	}
	if err := s.addColumn("bio_listings", "image_urls_override", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := s.addColumn("bio_listings", "product_details", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("bio_tryon_jobs", "buyer_google_account_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE bio_tryon_jobs SET status='expired'
		WHERE buyer_google_account_id=0 AND status IN ('queued','running','succeeded')`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_bio_tryon_idempotency`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_bio_tryon_google_idempotency
		ON bio_tryon_jobs(buyer_google_account_id,product_id,image_hash)
		WHERE status IN ('queued','running','succeeded')`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_bio_tryon_buyer_account
		ON bio_tryon_jobs(buyer_google_account_id,created_at)`); err != nil {
		return err
	}
	// R2-W1: shelf state moved into this store (it used to live in the
	// ledger products table, invisible to the autopilot). Columns are
	// added to installs created before this change.
	if err := s.addColumn("products", "shelf_status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("products", "shelf_score", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "click_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "original_price", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "data_updated_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	// Additive provenance fields. Existing metric values stay intact, but
	// source-less rows fail the production/public-claim gate until refreshed.
	if err := s.addColumn("products", "demand_evidence_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("products", "demand_evidence_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("products", "demand_evidence_rating", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "demand_evidence_sold_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Đợt Y: một sản phẩm chỉ được chọn một lần cho mỗi account — chống
	// race khi 2 chu kỳ (chạy tay + tick) cùng chọn 1 sản phẩm trước khi
	// RecordUse commit. Dedupe bản ghi cũ trước để tạo được UNIQUE index.
	if _, err := s.db.Exec(`DELETE FROM product_usage WHERE id NOT IN (
		SELECT MIN(id) FROM product_usage GROUP BY product_id, account_id)`); err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_product_account
		ON product_usage(product_id, account_id)`)
	return err
}

// addColumn appends a column unless it already exists (idempotent).
func (s *Store) addColumn(table, column, decl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + decl)
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

// RemoveSource deletes stale products from a source that is no longer trusted.
func (s *Store) RemoveSource(source string) (int64, error) {
	if strings.TrimSpace(source) == "" {
		return 0, fmt.Errorf("products: source is required")
	}
	res, err := s.db.Exec(`DELETE FROM products WHERE source=?`, source)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Count returns the total number of products in the store (autopilot
// status surface on /products).
func (s *Store) Count() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n)
	return n, err
}

// Save inserts or refreshes a product (dedupe on source+source_id).
func (s *Store) Save(p Product) (int64, error) {
	imgs, _ := json.Marshal(p.ImageURLs)
	now := time.Now().Format("2006-01-02T15:04:05")
	if p.DemandEvidenceURL != "" && p.DemandEvidenceAt != "" {
		p.DemandEvidenceRating = p.Rating
		p.DemandEvidenceSoldCount = p.SoldCount
	}
	res, err := s.db.Exec(`
		INSERT INTO products(source,source_id,title,image_urls,price,currency,original_price,data_updated_at,
			commission_rate,shop_name,rating,sold_count,demand_evidence_url,demand_evidence_at,
			demand_evidence_rating,demand_evidence_sold_count,category,product_url,theme,found_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,source_id) DO UPDATE SET
			title=excluded.title, image_urls=excluded.image_urls, price=excluded.price,
			original_price=excluded.original_price,
			data_updated_at=CASE WHEN excluded.data_updated_at<>'' THEN excluded.data_updated_at ELSE products.data_updated_at END,
			commission_rate=excluded.commission_rate, shop_name=excluded.shop_name,
			rating=excluded.rating, sold_count=excluded.sold_count,
			demand_evidence_url=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_url ELSE '' END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count THEN ''
				ELSE products.demand_evidence_url END,
			demand_evidence_at=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_at ELSE '' END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count THEN ''
				ELSE products.demand_evidence_at END,
			demand_evidence_rating=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_rating ELSE 0 END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count THEN 0
				ELSE products.demand_evidence_rating END,
			demand_evidence_sold_count=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_sold_count ELSE 0 END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count THEN 0
				ELSE products.demand_evidence_sold_count END,
			category=excluded.category, product_url=excluded.product_url,
			theme=excluded.theme, found_at=excluded.found_at`,
		p.Source, p.SourceID, p.Title, string(imgs), p.Price, p.Currency, p.OriginalPrice, p.DataUpdatedAt,
		p.CommissionRate, p.ShopName, p.Rating, p.SoldCount, p.DemandEvidenceURL, p.DemandEvidenceAt,
		p.DemandEvidenceRating, p.DemandEvidenceSoldCount, p.Category,
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
		&p.Currency, &p.OriginalPrice, &p.DataUpdatedAt, &p.CommissionRate, &p.ShopName, &p.Rating, &p.SoldCount,
		&p.DemandEvidenceURL, &p.DemandEvidenceAt, &p.DemandEvidenceRating, &p.DemandEvidenceSoldCount,
		&p.Category, &p.ProductURL, &p.Theme, &p.FoundAt,
		&p.ShelfStatus, &p.ShelfScore, &p.ClickCount)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(imgs), &p.ImageURLs)
	return p, nil
}

const productCols = `id,source,source_id,title,image_urls,price,currency,original_price,data_updated_at,` +
	`commission_rate,shop_name,rating,sold_count,demand_evidence_url,demand_evidence_at,` +
	`demand_evidence_rating,demand_evidence_sold_count,category,product_url,theme,found_at,` +
	`shelf_status,shelf_score,click_count`

// RecordClick increments the click counter for an affiliate product when
// a customer clicks through from the Bio Link storefront.
func (s *Store) RecordClick(id int64) error {
	_, err := s.db.Exec(`UPDATE products SET click_count = click_count + 1 WHERE id = ?`, id)
	return err
}

// TopByTheme returns the best unused-by-account products for a theme,
// ordered by score. Products already promoted for accountID are excluded.
// stampUserVersion records the schema generation for future migrations
// (R2-W7): a fresh database is marked with v, an existing stamp is never
// overwritten here.
func stampUserVersion(db *sql.DB, v int) error {
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		return err
	}
	if cur == 0 {
		_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
		return err
	}
	return nil
}

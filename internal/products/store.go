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
		commission_amount REAL NOT NULL DEFAULT 0,
		commission_currency TEXT NOT NULL DEFAULT '',
		shop_name TEXT NOT NULL DEFAULT '', rating REAL NOT NULL DEFAULT 0,
		sold_count INTEGER NOT NULL DEFAULT 0, sold_count_known INTEGER NOT NULL DEFAULT 0,
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
	CREATE TABLE IF NOT EXISTS product_snapshots(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL, captured_at TEXT NOT NULL,
		payload TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_product_snapshots_history
		ON product_snapshots(product_id,captured_at DESC,id DESC);
	CREATE TABLE IF NOT EXISTS product_review_history(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL, status TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '', reviewed_at TEXT NOT NULL,
		payload TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_product_review_history
		ON product_review_history(product_id,reviewed_at DESC,id DESC);
	-- generic key/value settings (e.g. autopilot schedule) so the web UI —
	-- not env vars or CLI — is the control plane for operations.
	CREATE TABLE IF NOT EXISTS settings(
		key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
`)
	if err != nil {
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
	if err := s.addColumn("products", "original_price", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "price_max", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "product_page_url", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("products", "data_updated_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	// Additive fields preserve existing products as unknown and remain readable
	// by older binaries, which ignore these extra columns during rollback.
	if err := s.addColumn("products", "commission_amount", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "commission_currency", "TEXT NOT NULL DEFAULT ''"); err != nil {
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
	if err := s.addColumn("products", "sold_count_known", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.addColumn("products", "review_status", "TEXT NOT NULL DEFAULT 'pending'"); err != nil {
		return err
	}
	if err := s.addColumn("products", "review_note", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumn("products", "reviewed_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	// Products already curated onto the shelf were explicitly selected before
	// review states existed. Preserve that decision; all other existing rows
	// remain pending review.
	if _, err := s.db.Exec(`UPDATE products SET review_status='approved'
		WHERE review_status='pending' AND shelf_status IN ('shelf','scaled')`); err != nil {
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
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO products(source,source_id,title,image_urls,price,price_max,currency,original_price,data_updated_at,
			commission_rate,commission_amount,commission_currency,shop_name,rating,sold_count,sold_count_known,demand_evidence_url,demand_evidence_at,
			demand_evidence_rating,demand_evidence_sold_count,category,product_url,product_page_url,theme,found_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,source_id) DO UPDATE SET
			review_status=CASE WHEN
				excluded.title<>products.title OR excluded.image_urls<>products.image_urls OR
				excluded.price<>products.price OR excluded.price_max<>products.price_max OR
				excluded.commission_rate<>products.commission_rate OR
				excluded.commission_amount<>products.commission_amount OR excluded.commission_currency<>products.commission_currency OR excluded.category<>products.category OR
				excluded.product_page_url<>products.product_page_url
				THEN 'pending' ELSE products.review_status END,
			review_note=CASE WHEN
				excluded.title<>products.title OR excluded.image_urls<>products.image_urls OR
				excluded.price<>products.price OR excluded.price_max<>products.price_max OR
				excluded.commission_rate<>products.commission_rate OR
				excluded.commission_amount<>products.commission_amount OR excluded.commission_currency<>products.commission_currency OR excluded.category<>products.category OR
				excluded.product_page_url<>products.product_page_url
				THEN 'Thông tin offer đã đổi; cần duyệt lại.' ELSE products.review_note END,
			reviewed_at=CASE WHEN
				excluded.title<>products.title OR excluded.image_urls<>products.image_urls OR
				excluded.price<>products.price OR excluded.price_max<>products.price_max OR
				excluded.commission_rate<>products.commission_rate OR
				excluded.commission_amount<>products.commission_amount OR excluded.commission_currency<>products.commission_currency OR excluded.category<>products.category OR
				excluded.product_page_url<>products.product_page_url
				THEN '' ELSE products.reviewed_at END,
			shelf_status=CASE WHEN
				excluded.title<>products.title OR excluded.image_urls<>products.image_urls OR
				excluded.price<>products.price OR excluded.price_max<>products.price_max OR
				excluded.commission_rate<>products.commission_rate OR
				excluded.commission_amount<>products.commission_amount OR excluded.commission_currency<>products.commission_currency OR excluded.category<>products.category OR
				excluded.product_page_url<>products.product_page_url
				THEN '' ELSE products.shelf_status END,
			shelf_score=CASE WHEN
				excluded.title<>products.title OR excluded.image_urls<>products.image_urls OR
				excluded.price<>products.price OR excluded.price_max<>products.price_max OR
				excluded.commission_rate<>products.commission_rate OR
				excluded.commission_amount<>products.commission_amount OR excluded.commission_currency<>products.commission_currency OR excluded.category<>products.category OR
				excluded.product_page_url<>products.product_page_url
				THEN 0 ELSE products.shelf_score END,
			title=excluded.title, image_urls=excluded.image_urls, price=excluded.price,
			price_max=excluded.price_max, original_price=excluded.original_price,
			data_updated_at=excluded.data_updated_at,
			commission_rate=excluded.commission_rate, commission_amount=excluded.commission_amount,
			commission_currency=excluded.commission_currency, shop_name=excluded.shop_name,
			rating=excluded.rating, sold_count=excluded.sold_count, sold_count_known=excluded.sold_count_known,
			demand_evidence_url=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_url ELSE '' END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count OR excluded.sold_count_known<>products.sold_count_known THEN ''
				ELSE products.demand_evidence_url END,
			demand_evidence_at=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_at ELSE '' END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count OR excluded.sold_count_known<>products.sold_count_known THEN ''
				ELSE products.demand_evidence_at END,
			demand_evidence_rating=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_rating ELSE 0 END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count OR excluded.sold_count_known<>products.sold_count_known THEN 0
				ELSE products.demand_evidence_rating END,
			demand_evidence_sold_count=CASE
				WHEN excluded.demand_evidence_url<>'' OR excluded.demand_evidence_at<>'' THEN
					CASE WHEN excluded.demand_evidence_url<>'' AND excluded.demand_evidence_at<>'' THEN excluded.demand_evidence_sold_count ELSE 0 END
				WHEN excluded.rating<>products.rating OR excluded.sold_count<>products.sold_count OR excluded.sold_count_known<>products.sold_count_known THEN 0
				ELSE products.demand_evidence_sold_count END,
			category=excluded.category,
			product_url=CASE WHEN excluded.product_page_url<>products.product_page_url THEN excluded.product_url
				WHEN excluded.product_url<>'' THEN excluded.product_url ELSE products.product_url END,
			product_page_url=CASE WHEN excluded.product_page_url<>'' THEN excluded.product_page_url ELSE products.product_page_url END,
			theme=excluded.theme`,
		p.Source, p.SourceID, p.Title, string(imgs), p.Price, p.PriceMax, p.Currency, p.OriginalPrice, p.DataUpdatedAt,
		p.CommissionRate, p.CommissionAmount, p.CommissionCurrency, p.ShopName, p.Rating, p.SoldCount, p.SoldCountKnown, p.DemandEvidenceURL, p.DemandEvidenceAt,
		p.DemandEvidenceRating, p.DemandEvidenceSoldCount, p.Category,
		p.ProductURL, p.ProductPageURL, p.Theme, now)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM products WHERE source=? AND source_id=?`, p.Source, p.SourceID).Scan(&id); err != nil {
		return 0, err
	}
	current, err := scanProduct(tx.QueryRow(`SELECT `+productCols+` FROM products WHERE id=?`, id))
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(productSnapshotData(current))
	if err != nil {
		return 0, err
	}
	var latest string
	err = tx.QueryRow(`SELECT payload FROM product_snapshots WHERE product_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&latest)
	if err == sql.ErrNoRows || (err == nil && latest != string(payload)) {
		if _, err := tx.Exec(`INSERT INTO product_snapshots(product_id,captured_at,payload) VALUES(?,?,?)`, id, time.Now().UTC().Format(time.RFC3339Nano), string(payload)); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// PruneOldSnapshots xóa các bản ghi product_snapshots cũ hơn keepDays nhưng luôn giữ lại bản mới nhất của mỗi sản phẩm.
func (s *Store) PruneOldSnapshots(keepDays int) (int64, error) {
	if keepDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -keepDays).Format(time.RFC3339Nano)
	res, err := s.db.Exec(`DELETE FROM product_snapshots
		WHERE captured_at < ? AND id NOT IN (
			SELECT MAX(id) FROM product_snapshots GROUP BY product_id
		)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune product_snapshots: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
		_, _ = s.db.Exec(`PRAGMA optimize`)
	}
	return n, nil
}

func scanProduct(row interface {
	Scan(...any) error
}) (Product, error) {
	var p Product
	var imgs string
	err := row.Scan(&p.ID, &p.Source, &p.SourceID, &p.Title, &imgs, &p.Price, &p.PriceMax,
		&p.Currency, &p.OriginalPrice, &p.DataUpdatedAt, &p.CommissionRate, &p.CommissionAmount, &p.CommissionCurrency,
		&p.ShopName, &p.Rating, &p.SoldCount, &p.SoldCountKnown,
		&p.DemandEvidenceURL, &p.DemandEvidenceAt, &p.DemandEvidenceRating, &p.DemandEvidenceSoldCount,
		&p.Category, &p.ProductURL, &p.ProductPageURL, &p.Theme, &p.FoundAt,
		&p.ShelfStatus, &p.ShelfScore, &p.ReviewStatus, &p.ReviewNote, &p.ReviewedAt)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(imgs), &p.ImageURLs)
	return p, nil
}

const productCols = `id,source,source_id,title,image_urls,price,price_max,currency,original_price,data_updated_at,` +
	`commission_rate,commission_amount,commission_currency,shop_name,rating,sold_count,sold_count_known,demand_evidence_url,demand_evidence_at,` +
	`demand_evidence_rating,demand_evidence_sold_count,category,product_url,product_page_url,theme,found_at,` +
	`shelf_status,shelf_score,review_status,review_note,reviewed_at`

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

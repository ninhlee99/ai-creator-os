package accesstrade

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver
)

// Store lưu cache chiến dịch + link đã tạo (SQLite WAL, một DB một nhiệm vụ).
type Store struct {
	db *sql.DB
}

// schemaVersion hiện tại; migration qua PRAGMA user_version.
const schemaVersion = 1

// NewStore mở (hoặc tạo) accesstrade.db trong dataDir.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("accesstrade: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("accesstrade: pragma %q: %w", p, err)
		}
	}
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		db.Close()
		return nil, fmt.Errorf("accesstrade: pragma user_version: %w", err)
	}
	if cur < schemaVersion {
		if err := migrate(db); err != nil {
			db.Close()
			return nil, err
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			db.Close()
			return nil, fmt.Errorf("accesstrade: stamp user_version: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS at_campaigns(
		campaign_id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		approval TEXT NOT NULL DEFAULT '',
		commission_policy TEXT NOT NULL DEFAULT '',
		commission_rate REAL NOT NULL DEFAULT 0,
		cookie_duration TEXT NOT NULL DEFAULT '',
		synced_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS at_links(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_url TEXT NOT NULL,
		campaign_id TEXT NOT NULL DEFAULT '',
		account_ref TEXT NOT NULL DEFAULT '',
		tracking_link TEXT NOT NULL,
		short_link TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		UNIQUE(product_url, campaign_id, account_ref));
	CREATE INDEX IF NOT EXISTS idx_at_links_campaign ON at_links(campaign_id);`)
	if err != nil {
		return fmt.Errorf("accesstrade: migrate: %w", err)
	}
	return nil
}

// Close đóng DB.
func (s *Store) Close() error { return s.db.Close() }

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// UpsertCampaigns lưu đè cache chiến dịch theo campaign_id.
func (s *Store) UpsertCampaigns(cs []Campaign) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("accesstrade: upsert campaigns: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO at_campaigns
		(campaign_id, name, approval, commission_policy, commission_rate, cookie_duration, synced_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(campaign_id) DO UPDATE SET
			name=excluded.name, approval=excluded.approval,
			commission_policy=excluded.commission_policy,
			commission_rate=excluded.commission_rate,
			cookie_duration=excluded.cookie_duration,
			synced_at=excluded.synced_at`)
	if err != nil {
		return fmt.Errorf("accesstrade: upsert campaigns: %w", err)
	}
	defer stmt.Close()
	for _, c := range cs {
		if _, err := stmt.Exec(c.ID, c.Name, c.Approval, c.CommissionPolicy,
			c.CommissionRate, c.CookieDuration, nowUTC()); err != nil {
			return fmt.Errorf("accesstrade: upsert campaigns: %w", err)
		}
	}
	return tx.Commit()
}

// CachedCampaigns đọc cache chiến dịch (mới nhất trước).
func (s *Store) CachedCampaigns() ([]Campaign, error) {
	rows, err := s.db.Query(`SELECT campaign_id, name, approval, commission_policy,
		commission_rate, cookie_duration FROM at_campaigns ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("accesstrade: đọc cache campaigns: %w", err)
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.Approval, &c.CommissionPolicy,
			&c.CommissionRate, &c.CookieDuration); err != nil {
			return nil, fmt.Errorf("accesstrade: đọc cache campaigns: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveLink lưu link đã tạo; trùng (product_url, campaign_id, account_ref)
// thì cập nhật link mới (upsert).
func (s *Store) SaveLink(productURL, campaignID, accountRef, trackingLink, shortLink string) error {
	_, err := s.db.Exec(`INSERT INTO at_links
		(product_url, campaign_id, account_ref, tracking_link, short_link, created_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(product_url, campaign_id, account_ref) DO UPDATE SET
			tracking_link=excluded.tracking_link, short_link=excluded.short_link,
			created_at=excluded.created_at`,
		productURL, campaignID, accountRef, trackingLink, shortLink, nowUTC())
	if err != nil {
		return fmt.Errorf("accesstrade: lưu link: %w", err)
	}
	return nil
}

// SavedLink là một link đã tạo trong at_links.
type SavedLink struct {
	ID          int64
	ProductURL  string
	CampaignID  string
	Campaign    string // tên chiến dịch (join, có thể trống)
	AccountRef  string
	TrackingURL string
	ShortURL    string
	CreatedAt   string
}

// ListLinks liệt kê link đã tạo (mới nhất trước, tối đa limit; 0 = tất cả).
func (s *Store) ListLinks(limit int) ([]SavedLink, error) {
	q := `SELECT l.id, l.product_url, l.campaign_id,
		COALESCE(c.name,''), l.account_ref, l.tracking_link, l.short_link, l.created_at
		FROM at_links l LEFT JOIN at_campaigns c ON c.campaign_id = l.campaign_id
		ORDER BY l.id DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("accesstrade: liệt kê link: %w", err)
	}
	defer rows.Close()
	var out []SavedLink
	for rows.Next() {
		var l SavedLink
		if err := rows.Scan(&l.ID, &l.ProductURL, &l.CampaignID, &l.Campaign,
			&l.AccountRef, &l.TrackingURL, &l.ShortURL, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("accesstrade: liệt kê link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

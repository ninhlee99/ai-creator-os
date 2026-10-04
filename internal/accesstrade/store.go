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
// v2 (Đợt C): thêm at_orders (đối soát đơn) + at_sync_state (watermark).
const schemaVersion = 2

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
	CREATE INDEX IF NOT EXISTS idx_at_links_campaign ON at_links(campaign_id);
	-- Đợt C: đối soát đơn hàng. upsert theo order_id; đơn pending được
	-- UPDATE tại chỗ (commission lúc hold thường = 0, sau mới điền số).
	CREATE TABLE IF NOT EXISTS at_orders(
		order_id TEXT PRIMARY KEY,
		campaign_id TEXT NOT NULL DEFAULT '',
		campaign_name TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 0,
		pub_commission REAL NOT NULL DEFAULT 0,
		utm_source TEXT NOT NULL DEFAULT '',
		utm_medium TEXT NOT NULL DEFAULT '',
		sub1 TEXT NOT NULL DEFAULT '',
		sub2 TEXT NOT NULL DEFAULT '',
		sub3 TEXT NOT NULL DEFAULT '',
		sub4 TEXT NOT NULL DEFAULT '',
		ordered_at TEXT NOT NULL DEFAULT '',
		raw_json TEXT NOT NULL DEFAULT '',
		synced_at TEXT NOT NULL,
		updated_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_at_orders_status ON at_orders(status);
	CREATE INDEX IF NOT EXISTS idx_at_orders_ordered ON at_orders(ordered_at);
	-- Watermark sync (since/until đã quét) cho order sync 30 phút/lần.
	CREATE TABLE IF NOT EXISTS at_sync_state(
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '');`)
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

// ------------------------------------------------------------------ orders

// SavedOrder là một đơn hàng đã sync trong at_orders.
type SavedOrder struct {
	OrderID       string
	CampaignID    string
	CampaignName  string
	Status        int
	PubCommission float64
	UTMSource     string
	UTMMedium     string
	Sub1          string
	Sub2          string
	OrderedAt     string
	SyncedAt      string
	UpdatedAt     string
}

// StatusLabel nhãn tiếng Việt cho trạng thái đơn.
func (o SavedOrder) StatusLabel() string {
	return Order{Status: o.Status}.StatusLabel()
}

// CommissionLabel hiển thị hoa hồng trung thực (pending chưa có số → "—").
func (o SavedOrder) CommissionLabel() string {
	return Order{Status: o.Status, PubCommission: o.PubCommission}.CommissionLabel()
}

// MaskedID che giữa order_id khi hiển thị (vd "AT12••••9X7Q").
func (o SavedOrder) MaskedID() string {
	id := o.OrderID
	if len(id) <= 8 {
		return id
	}
	return id[:4] + "••••" + id[len(id)-4:]
}

// UpsertOrders lưu đơn từ order-list; đơn đã có được UPDATE tại chỗ
// (status + commission mới nhất) — chống "đóng băng" hoa hồng ở 0đ khi
// đơn còn pending. Trả về (số mới, số cập nhật).
func (s *Store) UpsertOrders(os []Order) (added, updated int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("accesstrade: upsert orders: %w", err)
	}
	defer tx.Rollback()
	var exists int
	for _, o := range os {
		if o.OrderID == "" {
			continue
		}
		err := tx.QueryRow(`SELECT COUNT(*) FROM at_orders WHERE order_id=?`,
			o.OrderID).Scan(&exists)
		if err != nil {
			return 0, 0, fmt.Errorf("accesstrade: upsert orders: %w", err)
		}
		_, err = tx.Exec(`INSERT INTO at_orders
			(order_id, campaign_id, campaign_name, status, pub_commission,
			 utm_source, utm_medium, sub1, sub2, sub3, sub4,
			 ordered_at, raw_json, synced_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(order_id) DO UPDATE SET
				campaign_id=excluded.campaign_id, campaign_name=excluded.campaign_name,
				status=excluded.status, pub_commission=excluded.pub_commission,
				utm_source=excluded.utm_source, utm_medium=excluded.utm_medium,
				sub1=excluded.sub1, sub2=excluded.sub2, sub3=excluded.sub3,
				sub4=excluded.sub4, ordered_at=excluded.ordered_at,
				raw_json=excluded.raw_json, synced_at=excluded.synced_at,
				updated_at=excluded.updated_at`,
			o.OrderID, o.CampaignID, o.CampaignName, o.Status, o.PubCommission,
			o.UTMSource, o.UTMMedium, o.Sub1, o.Sub2, o.Sub3, o.Sub4,
			o.OrderedAt, string(o.Raw), nowUTC(), nowUTC())
		if err != nil {
			return 0, 0, fmt.Errorf("accesstrade: upsert orders: %w", err)
		}
		if exists > 0 {
			updated++
		} else {
			added++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("accesstrade: upsert orders: %w", err)
	}
	return added, updated, nil
}

// ListOrders liệt kê đơn đã sync (mới nhất trước, limit 0 = tất cả).
func (s *Store) ListOrders(limit int) ([]SavedOrder, error) {
	q := `SELECT order_id, campaign_id, campaign_name, status, pub_commission,
		utm_source, utm_medium, sub1, sub2, ordered_at, synced_at, updated_at
		FROM at_orders ORDER BY ordered_at DESC, updated_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("accesstrade: liệt kê đơn: %w", err)
	}
	defer rows.Close()
	var out []SavedOrder
	for rows.Next() {
		var o SavedOrder
		if err := rows.Scan(&o.OrderID, &o.CampaignID, &o.CampaignName,
			&o.Status, &o.PubCommission, &o.UTMSource, &o.UTMMedium,
			&o.Sub1, &o.Sub2, &o.OrderedAt, &o.SyncedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("accesstrade: liệt kê đơn: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OrderStats là số liệu đối soát trung thực: đã duyệt và chờ duyệt
// HIỆN RIÊNG — không cộng pending vào "đã nhận".
type OrderStats struct {
	ApprovedCount int64
	ApprovedTotal float64
	PendingCount  int64
	PendingTotal  float64
	RejectedCount int64
}

// GetOrderStats tổng hợp đối soát từ at_orders.
func (s *Store) GetOrderStats() (OrderStats, error) {
	var st OrderStats
	rows, err := s.db.Query(`SELECT status, COUNT(*), COALESCE(SUM(pub_commission),0)
		FROM at_orders GROUP BY status`)
	if err != nil {
		return st, fmt.Errorf("accesstrade: đối soát: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status int
		var n int64
		var total float64
		if err := rows.Scan(&status, &n, &total); err != nil {
			return st, fmt.Errorf("accesstrade: đối soát: %w", err)
		}
		switch status {
		case OrderApproved:
			st.ApprovedCount, st.ApprovedTotal = n, total
		case OrderRejected:
			st.RejectedCount = n
		default:
			st.PendingCount, st.PendingTotal = n, total
		}
	}
	return st, rows.Err()
}

// GetSyncState đọc watermark sync (ok=false khi chưa có).
func (s *Store) GetSyncState(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM at_sync_state WHERE key=?`,
		key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// SetSyncState ghi watermark sync.
func (s *Store) SetSyncState(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO at_sync_state(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("accesstrade: lưu sync state: %w", err)
	}
	return nil
}

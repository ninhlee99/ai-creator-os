package products

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const defaultBioAccountSetting = "bio.default_account_id"
const bioEventPrunedDaySetting = "bio.events_pruned_utc_day"

var (
	ErrBioProfileNotFound = errors.New("bio profile not found")
	ErrBioProductNotFound = errors.New("bio product not found")
)

type BioProfile struct {
	AccountID   int64
	Published   bool
	Description string
	UpdatedAt   string
}

type BioListing struct {
	Product        Product
	ImageURLs      []string
	Published      bool
	DisplayOrder   int
	VoucherCode    string
	VoucherValue   float64
	VoucherEndsAt  string
	VoucherTerms   string
	ProductDetails string
}

type BioDailyMetric struct {
	Day            string
	ProfileViews   int64
	ProductViews   int64
	OutboundClicks int64
	VoucherCopies  int64
	TryOnStarts    int64
	TryOnSuccesses int64
	TryOnFailures  int64
	QuotaDenials   int64
}

type BioProductMetric struct {
	ProductID      int64
	Title          string
	Views          int64
	OutboundClicks int64
	VoucherCopies  int64
	TryOnStarts    int64
	TryOnSuccesses int64
	TryOnFailures  int64
	QuotaDenials   int64
}

var bioEventTypes = map[string]bool{
	"profile_view": true, "product_view": true, "outbound_click": true,
	"voucher_copy": true, "tryon_start": true, "tryon_success": true,
	"tryon_failure": true, "tryon_quota_denied": true,
}

func (s *Store) GetBioProfile(accountID int64) (BioProfile, error) {
	var p BioProfile
	var published int
	err := s.db.QueryRow(`SELECT account_id,published,description,updated_at
		FROM bio_profiles WHERE account_id=?`, accountID).Scan(
		&p.AccountID, &published, &p.Description, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BioProfile{}, ErrBioProfileNotFound
		}
		return BioProfile{}, err
	}
	p.Published = published == 1
	return p, nil
}

func (s *Store) SetBioProfile(accountID int64, published bool, description string, makeDefault bool) error {
	if accountID <= 0 {
		return fmt.Errorf("bio account ID must be positive")
	}
	if makeDefault && !published {
		return fmt.Errorf("default bio profile must be published")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO bio_profiles(account_id,published,description,updated_at)
		VALUES(?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET
		published=excluded.published, description=excluded.description,
		updated_at=excluded.updated_at`, accountID, boolInt(published), strings.TrimSpace(description), now)
	if err != nil {
		return err
	}
	var current string
	err = tx.QueryRow(`SELECT value FROM settings WHERE key=?`, defaultBioAccountSetting).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	switch {
	case published && makeDefault:
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, defaultBioAccountSetting, strconv.FormatInt(accountID, 10))
	case current == strconv.FormatInt(accountID, 10) && (!published || !makeDefault):
		_, err = tx.Exec(`DELETE FROM settings WHERE key=?`, defaultBioAccountSetting)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DefaultBioAccountID() (int64, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, defaultBioAccountSetting).Scan(&raw); err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrBioProfileNotFound
	}
	profile, err := s.GetBioProfile(id)
	if err != nil || !profile.Published {
		return 0, ErrBioProfileNotFound
	}
	return id, nil
}

func (s *Store) SetBioListing(accountID, productID int64, published bool, displayOrder int,
	voucherCode string, voucherValue float64, voucherEndsAt, voucherTerms string) error {
	return s.SetBioListingWithImages(accountID, productID, published, displayOrder, nil,
		voucherCode, voucherValue, voucherEndsAt, voucherTerms)
}

// SetBioListingWithImages saves channel-specific product image URLs and
// private upload markers, separate from provider catalog data.
func (s *Store) SetBioListingWithImages(accountID, productID int64, published bool, displayOrder int,
	imageURLs []string, voucherCode string, voucherValue float64, voucherEndsAt, voucherTerms string) error {
	return s.setBioListingWithImages(accountID, productID, published, displayOrder, imageURLs,
		voucherCode, voucherValue, voucherEndsAt, voucherTerms, "", false)
}

func (s *Store) SetBioListingWithProductDetails(accountID, productID int64, published bool, displayOrder int,
	imageURLs []string, voucherCode string, voucherValue float64, voucherEndsAt, voucherTerms,
	productDetails string) error {
	return s.setBioListingWithImages(accountID, productID, published, displayOrder, imageURLs,
		voucherCode, voucherValue, voucherEndsAt, voucherTerms, productDetails, true)
}

func (s *Store) setBioListingWithImages(accountID, productID int64, published bool, displayOrder int,
	imageURLs []string, voucherCode string, voucherValue float64, voucherEndsAt, voucherTerms,
	productDetails string, updateProductDetails bool) error {
	if accountID <= 0 || productID <= 0 || displayOrder < 0 || voucherValue < 0 {
		return fmt.Errorf("invalid bio listing values")
	}
	if len(imageURLs) > 8 {
		return fmt.Errorf("Bio listing supports at most 8 product images")
	}
	productDetails = strings.TrimSpace(productDetails)
	if updateProductDetails && utf8.RuneCountInString(productDetails) > 1500 {
		return fmt.Errorf("Bio product details exceed 1500 characters")
	}
	if imageURLs == nil {
		imageURLs = []string{}
	}
	imagesJSON, err := json.Marshal(imageURLs)
	if err != nil {
		return err
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM products WHERE id=?`, productID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBioProductNotFound
		}
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`INSERT INTO bio_listings(account_id,product_id,published,display_order,
		voucher_code,voucher_value,voucher_ends_at,voucher_terms,product_details,image_urls_override,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(account_id,product_id) DO UPDATE SET
		published=excluded.published, display_order=excluded.display_order,
		voucher_code=excluded.voucher_code, voucher_value=excluded.voucher_value,
		voucher_ends_at=excluded.voucher_ends_at, voucher_terms=excluded.voucher_terms,
		product_details=CASE WHEN ?=1 THEN excluded.product_details ELSE bio_listings.product_details END,
		image_urls_override=excluded.image_urls_override,
		updated_at=excluded.updated_at`, accountID, productID, boolInt(published), displayOrder,
		strings.TrimSpace(voucherCode), voucherValue, strings.TrimSpace(voucherEndsAt),
		strings.TrimSpace(voucherTerms), productDetails, string(imagesJSON), now, now, boolInt(updateProductDetails))
	return err
}

func (s *Store) BioListing(accountID, productID int64, publishedOnly bool) (BioListing, error) {
	return s.scanBioListing(s.db.QueryRow(s.bioListingQuery(publishedOnly), accountID, productID))
}

func (s *Store) HasBioProductImage(marker string) (bool, error) {
	rows, err := s.db.Query(`SELECT image_urls_override FROM bio_listings WHERE image_urls_override IS NOT NULL`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		var images []string
		if err := json.Unmarshal([]byte(raw), &images); err != nil {
			return false, err
		}
		for _, image := range images {
			if image == marker {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

func (s *Store) ListBioListings(accountID int64, publishedOnly bool, limit int) ([]BioListing, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT ` + qualifiedProductCols("p") + `,
		bl.published,bl.display_order,bl.voucher_code,bl.voucher_value,bl.voucher_ends_at,bl.voucher_terms,
		bl.product_details,bl.image_urls_override
		FROM bio_listings bl JOIN products p ON p.id=bl.product_id
		WHERE bl.account_id=?`
	if publishedOnly {
		query += ` AND bl.published=1`
	}
	query += ` ORDER BY bl.display_order, p.shelf_score DESC, p.id DESC LIMIT ?`
	rows, err := s.db.Query(query, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BioListing
	for rows.Next() {
		item, err := s.scanBioListing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) bioListingQuery(publishedOnly bool) string {
	query := `SELECT ` + qualifiedProductCols("p") + `,
		bl.published,bl.display_order,bl.voucher_code,bl.voucher_value,bl.voucher_ends_at,bl.voucher_terms,
		bl.product_details,bl.image_urls_override
		FROM bio_listings bl JOIN products p ON p.id=bl.product_id
		WHERE bl.account_id=? AND bl.product_id=?`
	if publishedOnly {
		query += ` AND bl.published=1`
	}
	return query
}

func (s *Store) scanBioListing(row interface{ Scan(...any) error }) (BioListing, error) {
	var item BioListing
	var p Product
	var imgs string
	var overrideImages string
	var published int
	err := row.Scan(&p.ID, &p.Source, &p.SourceID, &p.Title, &imgs, &p.Price,
		&p.Currency, &p.OriginalPrice, &p.DataUpdatedAt, &p.CommissionRate, &p.ShopName,
		&p.Rating, &p.SoldCount, &p.DemandEvidenceURL, &p.DemandEvidenceAt,
		&p.DemandEvidenceRating, &p.DemandEvidenceSoldCount,
		&p.Category, &p.ProductURL, &p.Theme, &p.FoundAt,
		&p.ShelfStatus, &p.ShelfScore, &p.ClickCount,
		&published, &item.DisplayOrder, &item.VoucherCode, &item.VoucherValue,
		&item.VoucherEndsAt, &item.VoucherTerms, &item.ProductDetails, &overrideImages)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BioListing{}, ErrBioProductNotFound
		}
		return BioListing{}, err
	}
	_ = json.Unmarshal([]byte(imgs), &p.ImageURLs)
	_ = json.Unmarshal([]byte(overrideImages), &item.ImageURLs)
	item.Product = p
	item.Published = published == 1
	return item, nil
}

func qualifiedProductCols(alias string) string {
	cols := strings.Split(productCols, ",")
	for i := range cols {
		cols[i] = alias + "." + strings.TrimSpace(cols[i])
	}
	return strings.Join(cols, ",")
}

func (s *Store) RecordBioEvent(accountID, productID int64, eventType string, at time.Time) error {
	if accountID <= 0 || !bioEventTypes[eventType] || productID < 0 {
		return fmt.Errorf("invalid bio event")
	}
	if at.IsZero() {
		at = time.Now()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := recordBioEventTx(tx, accountID, productID, eventType, at); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordBioEventOnce persists a short-lived, route-scoped HMAC dedupe key and
// its event atomically. It prevents restarts and concurrent requests from
// inflating analytics while keeping raw client identities out of the store.
func (s *Store) RecordBioEventOnce(dedupeKey string, accountID, productID int64, eventType string, at time.Time, window time.Duration) (bool, error) {
	decoded, decodeErr := hex.DecodeString(dedupeKey)
	if decodeErr != nil || len(decoded) != 32 || accountID <= 0 || !bioEventTypes[eventType] || productID < 0 || window <= 0 {
		return false, fmt.Errorf("invalid bio event dedupe request")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	now := formatBioTryOnTime(at)
	expiresAt := formatBioTryOnTime(at.Add(window))
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bio_event_dedupe WHERE expires_at<=?`, now); err != nil {
		return false, err
	}
	result, err := tx.Exec(`INSERT INTO bio_event_dedupe(dedupe_key,expires_at) VALUES(?,?)
		ON CONFLICT(dedupe_key) DO UPDATE SET expires_at=excluded.expires_at
		WHERE bio_event_dedupe.expires_at<=?`, dedupeKey, expiresAt, now)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := recordBioEventTx(tx, accountID, productID, eventType, at); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// CleanupExpiredBioEventDedupe removes short-lived HMACs after their window.
func (s *Store) CleanupExpiredBioEventDedupe(at time.Time) (int, error) {
	if at.IsZero() {
		at = time.Now()
	}
	result, err := s.db.Exec(`DELETE FROM bio_event_dedupe WHERE expires_at<=?`, formatBioTryOnTime(at.UTC()))
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func recordBioEventTx(tx *sql.Tx, accountID, productID int64, eventType string, at time.Time) error {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		loc = time.FixedZone("ICT", 7*60*60)
	}
	local := at.In(loc)
	if _, err := tx.Exec(`INSERT INTO bio_events(account_id,product_id,event_type,local_day,created_at)
		VALUES(?,?,?,?,?)`, accountID, productID, eventType, local.Format("2006-01-02"), at.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	utcDay := at.UTC().Format("2006-01-02")
	var prunedDay string
	pruneErr := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, bioEventPrunedDaySetting).Scan(&prunedDay)
	if pruneErr != nil && !errors.Is(pruneErr, sql.ErrNoRows) {
		return pruneErr
	}
	if prunedDay < utcDay {
		cutoff := at.UTC().AddDate(0, 0, -400).Format(time.RFC3339)
		if _, err := tx.Exec(`DELETE FROM bio_events WHERE created_at<?`, cutoff); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, bioEventPrunedDaySetting, utcDay); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BioDailyMetrics(accountID int64, fromDay string) ([]BioDailyMetric, error) {
	rows, err := s.db.Query(`SELECT local_day,
		SUM(CASE WHEN event_type='profile_view' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='product_view' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='outbound_click' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='voucher_copy' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='tryon_start' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='tryon_success' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='tryon_failure' THEN 1 ELSE 0 END),
		SUM(CASE WHEN event_type='tryon_quota_denied' THEN 1 ELSE 0 END)
		FROM bio_events WHERE account_id=? AND local_day>=?
		GROUP BY local_day ORDER BY local_day`, accountID, fromDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BioDailyMetric
	for rows.Next() {
		var m BioDailyMetric
		if err := rows.Scan(&m.Day, &m.ProfileViews, &m.ProductViews, &m.OutboundClicks,
			&m.VoucherCopies, &m.TryOnStarts, &m.TryOnSuccesses, &m.TryOnFailures, &m.QuotaDenials); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) BioProductMetrics(accountID int64, fromDay string, limit int) ([]BioProductMetric, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT p.id,p.title,
		SUM(CASE WHEN e.event_type='product_view' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='outbound_click' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='voucher_copy' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='tryon_start' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='tryon_success' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='tryon_failure' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.event_type='tryon_quota_denied' THEN 1 ELSE 0 END)
		FROM bio_listings bl JOIN products p ON p.id=bl.product_id
		LEFT JOIN bio_events e ON e.account_id=bl.account_id AND e.product_id=bl.product_id AND e.local_day>=?
		WHERE bl.account_id=? AND bl.published=1
		GROUP BY p.id,p.title ORDER BY 4 DESC,3 DESC,p.title LIMIT ?`, fromDay, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BioProductMetric
	for rows.Next() {
		var m BioProductMetric
		if err := rows.Scan(&m.ProductID, &m.Title, &m.Views, &m.OutboundClicks,
			&m.VoucherCopies, &m.TryOnStarts, &m.TryOnSuccesses, &m.TryOnFailures,
			&m.QuotaDenials); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

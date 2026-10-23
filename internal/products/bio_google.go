package products

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrBioGoogleAccountNotFound = errors.New("Bio Google account not found or disabled")

type BioGoogleAccount struct {
	ID          int64
	Email       string
	DisplayName string
	Active      bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

type BioGoogleAccountUsage struct {
	BioGoogleAccount
	RequestsToday int64
	Products      []BioGoogleProductUsage
}

type BioGoogleProductUsage struct {
	ProductID    int64
	ProductTitle string
	RequestCount int64
}

// UpsertBioGoogleAccount stores a one-way hash of Google's stable subject and
// updates only the user-provided profile claims on subsequent sign-ins.
func (s *Store) UpsertBioGoogleAccount(subjectHash, email, displayName string, at time.Time) (BioGoogleAccount, error) {
	if !validTryOnValue(subjectHash, 32, 128) {
		return BioGoogleAccount{}, fmt.Errorf("invalid Bio Google subject")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	email = strings.TrimSpace(strings.ToLower(email))
	displayName = strings.TrimSpace(displayName)
	if len(email) > 254 {
		return BioGoogleAccount{}, fmt.Errorf("Bio Google email is too long")
	}
	if len(displayName) > 160 {
		displayName = displayName[:160]
	}
	stamp := formatBioTryOnTime(at)
	_, err := s.db.Exec(`INSERT INTO bio_google_accounts(subject_hash,email,display_name,active,created_at,last_login_at)
		VALUES(?,?,?,1,?,?) ON CONFLICT(subject_hash) DO UPDATE SET
		email=excluded.email,display_name=excluded.display_name,last_login_at=excluded.last_login_at`,
		subjectHash, email, displayName, stamp, stamp)
	if err != nil {
		return BioGoogleAccount{}, err
	}
	return s.bioGoogleAccountBySubject(subjectHash)
}

func (s *Store) bioGoogleAccountBySubject(subjectHash string) (BioGoogleAccount, error) {
	var account BioGoogleAccount
	var active int
	var createdAt, lastLoginAt string
	err := s.db.QueryRow(`SELECT id,email,display_name,active,created_at,last_login_at
		FROM bio_google_accounts WHERE subject_hash=?`, subjectHash).Scan(
		&account.ID, &account.Email, &account.DisplayName, &active, &createdAt, &lastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BioGoogleAccount{}, ErrBioGoogleAccountNotFound
	}
	if err != nil {
		return BioGoogleAccount{}, err
	}
	account.Active = active == 1
	account.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	account.LastLoginAt, _ = time.Parse(time.RFC3339Nano, lastLoginAt)
	return account, nil
}

func (s *Store) CreateBioGoogleSession(sessionHash string, accountID int64, now, expiresAt time.Time) error {
	if !validTryOnValue(sessionHash, 32, 128) || accountID <= 0 {
		return fmt.Errorf("invalid Bio Google session")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !expiresAt.After(now) {
		return fmt.Errorf("Bio Google session expiry must be in the future")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM bio_google_sessions WHERE expires_at<=?`, formatBioTryOnTime(now.UTC())); err != nil {
		return err
	}
	var active int
	err = tx.QueryRow(`SELECT active FROM bio_google_accounts WHERE id=?`, accountID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) || err == nil && active != 1 {
		return ErrBioGoogleAccountNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO bio_google_sessions(session_hash,account_id,created_at,expires_at)
		VALUES(?,?,?,?)`, sessionHash, accountID, formatBioTryOnTime(now.UTC()), formatBioTryOnTime(expiresAt.UTC())); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) BioGoogleAccountForSession(sessionHash string, now time.Time) (BioGoogleAccount, error) {
	if !validTryOnValue(sessionHash, 32, 128) {
		return BioGoogleAccount{}, ErrBioGoogleAccountNotFound
	}
	if now.IsZero() {
		now = time.Now()
	}
	var account BioGoogleAccount
	var active int
	var createdAt, lastLoginAt string
	err := s.db.QueryRow(`SELECT a.id,a.email,a.display_name,a.active,a.created_at,a.last_login_at
		FROM bio_google_sessions s JOIN bio_google_accounts a ON a.id=s.account_id
		WHERE s.session_hash=? AND s.expires_at>?`, sessionHash, formatBioTryOnTime(now.UTC())).Scan(
		&account.ID, &account.Email, &account.DisplayName, &active, &createdAt, &lastLoginAt)
	if errors.Is(err, sql.ErrNoRows) || err == nil && active != 1 {
		return BioGoogleAccount{}, ErrBioGoogleAccountNotFound
	}
	if err != nil {
		return BioGoogleAccount{}, err
	}
	account.Active = true
	account.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	account.LastLoginAt, _ = time.Parse(time.RFC3339Nano, lastLoginAt)
	return account, nil
}

func (s *Store) RevokeBioGoogleSession(sessionHash string) error {
	if !validTryOnValue(sessionHash, 32, 128) {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM bio_google_sessions WHERE session_hash=?`, sessionHash)
	return err
}

func (s *Store) SetBioGoogleAccountActive(accountID int64, active bool) error {
	if accountID <= 0 {
		return ErrBioGoogleAccountNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE bio_google_accounts SET active=? WHERE id=?`, boolInt(active), accountID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrBioGoogleAccountNotFound
	}
	if !active {
		if _, err := tx.Exec(`DELETE FROM bio_google_sessions WHERE account_id=?`, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListBioGoogleAccountUsage(localDay string, limit int) ([]BioGoogleAccountUsage, error) {
	if _, err := time.Parse("2006-01-02", localDay); err != nil {
		return nil, fmt.Errorf("invalid Bio usage day")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(`WITH top_accounts AS (
		SELECT a.id,a.email,a.display_name,a.active,a.created_at,a.last_login_at,
			COALESCE(d.request_count,0) AS requests_today
		FROM bio_google_accounts a
		LEFT JOIN bio_tryon_account_daily_usage d ON d.account_id=a.id AND d.local_day=?
		ORDER BY requests_today DESC,a.last_login_at DESC,a.id LIMIT ?
	), ranked_products AS (
		SELECT u.account_id,u.product_id,p.title,u.request_count,
			ROW_NUMBER() OVER(PARTITION BY u.account_id ORDER BY u.request_count DESC,p.title) AS rank
		FROM bio_tryon_account_product_usage u JOIN products p ON p.id=u.product_id
		JOIN top_accounts a ON a.id=u.account_id
	)
	SELECT a.id,a.email,a.display_name,a.active,a.created_at,a.last_login_at,a.requests_today,
		u.product_id,u.title,u.request_count
	FROM top_accounts a
	LEFT JOIN ranked_products u ON u.account_id=a.id AND u.rank<=20
	ORDER BY a.requests_today DESC,a.last_login_at DESC,a.id,u.request_count DESC`, localDay, limit)
	if err != nil {
		return nil, err
	}
	var accounts []BioGoogleAccountUsage
	accountIndex := make(map[int64]int)
	for rows.Next() {
		var item BioGoogleAccountUsage
		var active int
		var createdAt, lastLoginAt string
		var productID sql.NullInt64
		var productTitle sql.NullString
		var productCount sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &active,
			&createdAt, &lastLoginAt, &item.RequestsToday, &productID, &productTitle, &productCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		index, exists := accountIndex[item.ID]
		if !exists {
			item.Active = active == 1
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
			item.LastLoginAt, _ = time.Parse(time.RFC3339Nano, lastLoginAt)
			accounts = append(accounts, item)
			index = len(accounts) - 1
			accountIndex[item.ID] = index
		}
		if productID.Valid {
			accounts[index].Products = append(accounts[index].Products, BioGoogleProductUsage{
				ProductID: productID.Int64, ProductTitle: productTitle.String, RequestCount: productCount.Int64,
			})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return accounts, nil
}

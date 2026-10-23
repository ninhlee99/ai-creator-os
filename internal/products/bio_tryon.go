package products

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	BioTryOnProductLimit      = 3
	BioTryOnDailyLimit        = 15
	BioTryOnRetention         = 24 * time.Hour
	BioTryOnQueueTimeout      = 15 * time.Minute
	BioTryOnProcessingTimeout = 2 * time.Hour
	BioTryOnFailureRetention  = 15 * time.Minute
)

var ErrBioTryOnJobNotFound = errors.New("bio try-on job not found")

type BioTryOnRequest struct {
	ID                   string
	AccountID            int64 // published creator/channel account
	BuyerGoogleAccountID int64
	ProductID            int64
	ImageHash            string
	Now                  time.Time
	// Deprecated compatibility fields; try-on quota and ownership use Google account.
	IPHash     string
	DeviceHash string
}

type BioTryOnJob struct {
	ID                   string
	AccountID            int64
	BuyerGoogleAccountID int64
	ProductID            int64
	Status               string
	ResultPath           string
	ErrorCode            string
	CreatedAt            time.Time
	ExpiresAt            time.Time
}

type BioTryOnReservation struct {
	Job         BioTryOnJob
	Reused      bool
	QuotaReason string
}

const bioTryOnTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

// ReserveBioTryOn atomically checks Google-account idempotency and increments
// per-account quotas before creating a generation job.
func (s *Store) ReserveBioTryOn(request BioTryOnRequest) (BioTryOnReservation, error) {
	if !validTryOnValue(request.ID, 24, 128) || request.AccountID <= 0 ||
		request.BuyerGoogleAccountID <= 0 || request.ProductID <= 0 || request.ImageHash == "" {
		return BioTryOnReservation{}, fmt.Errorf("invalid bio try-on request")
	}
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	now := request.Now.UTC()
	localDay := bioLocalDay(now)
	tx, err := s.db.Begin()
	if err != nil {
		return BioTryOnReservation{}, err
	}
	defer tx.Rollback()
	var buyerActive int
	err = tx.QueryRow(`SELECT active FROM bio_google_accounts WHERE id=?`, request.BuyerGoogleAccountID).Scan(&buyerActive)
	if errors.Is(err, sql.ErrNoRows) || err == nil && buyerActive != 1 {
		return BioTryOnReservation{}, ErrBioGoogleAccountNotFound
	}
	if err != nil {
		return BioTryOnReservation{}, err
	}
	cutoffDay := now.AddDate(0, 0, -400).In(bioTimeLocation()).Format("2006-01-02")
	if _, err := tx.Exec(`DELETE FROM bio_tryon_daily_usage WHERE local_day<?`, cutoffDay); err != nil {
		return BioTryOnReservation{}, err
	}
	if _, err := tx.Exec(`DELETE FROM bio_tryon_account_daily_usage WHERE local_day<?`, cutoffDay); err != nil {
		return BioTryOnReservation{}, err
	}
	if _, err := tx.Exec(`UPDATE bio_tryon_jobs SET status='expired',updated_at=?
		WHERE expires_at<=? AND status IN ('queued','running','succeeded')`,
		formatBioTryOnTime(now), formatBioTryOnTime(now)); err != nil {
		return BioTryOnReservation{}, err
	}

	var published int
	var profilePublished int
	var source string
	if err := tx.QueryRow(`SELECT bl.published,p.source,bp.published FROM bio_listings bl
		JOIN products p ON p.id=bl.product_id
		JOIN bio_profiles bp ON bp.account_id=bl.account_id
		WHERE bl.account_id=? AND bl.product_id=?`, request.AccountID, request.ProductID).Scan(&published, &source, &profilePublished); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BioTryOnReservation{}, ErrBioProductNotFound
		}
		return BioTryOnReservation{}, err
	}
	if published != 1 || profilePublished != 1 || source == "market_verified" {
		return BioTryOnReservation{}, ErrBioProductNotFound
	}

	// Reuse only within the same Google account, product and normalized image.
	var existing BioTryOnJob
	var createdAt, expiresAt string
	err = tx.QueryRow(`SELECT id,account_id,buyer_google_account_id,product_id,status,result_path,error_code,created_at,expires_at
		FROM bio_tryon_jobs WHERE buyer_google_account_id=? AND product_id=? AND image_hash=?
		AND status IN ('queued','running','succeeded') AND expires_at>?
		ORDER BY created_at DESC LIMIT 1`, request.BuyerGoogleAccountID, request.ProductID,
		request.ImageHash, formatBioTryOnTime(now)).Scan(
		&existing.ID, &existing.AccountID, &existing.BuyerGoogleAccountID, &existing.ProductID, &existing.Status,
		&existing.ResultPath, &existing.ErrorCode, &createdAt, &expiresAt)
	if err == nil {
		existing.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		existing.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
		if err := tx.Commit(); err != nil {
			return BioTryOnReservation{}, err
		}
		return BioTryOnReservation{Job: existing, Reused: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return BioTryOnReservation{}, err
	}

	limits := []struct {
		productLimit bool
		reason       string
	}{{true, "account_product"}, {false, "account_daily"}}
	for _, limit := range limits {
		var result sql.Result
		if limit.productLimit {
			result, err = tx.Exec(`INSERT INTO bio_tryon_account_product_usage(account_id,product_id,request_count)
				VALUES(?,?,1) ON CONFLICT(account_id,product_id)
				DO UPDATE SET request_count=request_count+1 WHERE request_count<?`,
				request.BuyerGoogleAccountID, request.ProductID, BioTryOnProductLimit)
		} else {
			result, err = tx.Exec(`INSERT INTO bio_tryon_account_daily_usage(account_id,local_day,request_count)
				VALUES(?,?,1) ON CONFLICT(account_id,local_day)
				DO UPDATE SET request_count=request_count+1 WHERE request_count<?`,
				request.BuyerGoogleAccountID, localDay, BioTryOnDailyLimit)
		}
		if err != nil {
			return BioTryOnReservation{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return BioTryOnReservation{}, err
		}
		if affected != 1 {
			return BioTryOnReservation{QuotaReason: limit.reason}, nil
		}
	}

	expires := now.Add(BioTryOnQueueTimeout)
	_, err = tx.Exec(`INSERT INTO bio_tryon_jobs(id,account_id,buyer_google_account_id,product_id,ip_hash,device_hash,image_hash,
		status,local_day,created_at,updated_at,expires_at) VALUES(?,?,?,?,?,? ,?,'queued',?,?,?,?)`,
		request.ID, request.AccountID, request.BuyerGoogleAccountID, request.ProductID, "", "",
		request.ImageHash, localDay, formatBioTryOnTime(now), formatBioTryOnTime(now),
		formatBioTryOnTime(expires))
	if err != nil {
		// The unique active-job index closes the duplicate-submit race even if
		// another process reaches the insert between the lookup and reservation.
		var duplicate BioTryOnJob
		var duplicateCreated, duplicateExpires string
		lookupErr := tx.QueryRow(`SELECT id,account_id,buyer_google_account_id,product_id,status,result_path,error_code,created_at,expires_at
			FROM bio_tryon_jobs WHERE buyer_google_account_id=? AND product_id=? AND image_hash=?
			AND status IN ('queued','running','succeeded') AND expires_at>?`,
			request.BuyerGoogleAccountID, request.ProductID, request.ImageHash, formatBioTryOnTime(now)).Scan(
			&duplicate.ID, &duplicate.AccountID, &duplicate.BuyerGoogleAccountID, &duplicate.ProductID, &duplicate.Status,
			&duplicate.ResultPath, &duplicate.ErrorCode, &duplicateCreated, &duplicateExpires)
		if lookupErr == nil {
			duplicate.CreatedAt, _ = time.Parse(time.RFC3339Nano, duplicateCreated)
			duplicate.ExpiresAt, _ = time.Parse(time.RFC3339Nano, duplicateExpires)
			_ = tx.Rollback()
			return BioTryOnReservation{Job: duplicate, Reused: true}, nil
		}
		return BioTryOnReservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return BioTryOnReservation{}, err
	}
	return BioTryOnReservation{Job: BioTryOnJob{
		ID: request.ID, AccountID: request.AccountID, BuyerGoogleAccountID: request.BuyerGoogleAccountID,
		ProductID: request.ProductID,
		Status:    "queued", CreatedAt: now, ExpiresAt: expires,
	}}, nil
}

// GetBioTryOnJob authorizes lookup by the random job ID and Google account.
func (s *Store) GetBioTryOnJob(id string, buyerGoogleAccountID int64, now time.Time) (BioTryOnJob, error) {
	if !validTryOnValue(id, 24, 128) || buyerGoogleAccountID <= 0 {
		return BioTryOnJob{}, ErrBioTryOnJobNotFound
	}
	if now.IsZero() {
		now = time.Now()
	}
	var job BioTryOnJob
	var createdAt, expiresAt string
	err := s.db.QueryRow(`SELECT id,account_id,buyer_google_account_id,product_id,status,result_path,error_code,created_at,expires_at
		FROM bio_tryon_jobs WHERE id=? AND expires_at>? AND buyer_google_account_id=?`,
		id, formatBioTryOnTime(now.UTC()), buyerGoogleAccountID).Scan(
		&job.ID, &job.AccountID, &job.BuyerGoogleAccountID, &job.ProductID, &job.Status, &job.ResultPath, &job.ErrorCode,
		&createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BioTryOnJob{}, ErrBioTryOnJobNotFound
	}
	if err != nil {
		return BioTryOnJob{}, err
	}
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	job.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	return job, nil
}

// ClaimNextBioTryOnJob atomically moves the oldest live queued job to running.
// A single UPDATE keeps separate worker processes from rendering the same job.
func (s *Store) ClaimNextBioTryOnJob(now time.Time) (BioTryOnJob, bool, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	var job BioTryOnJob
	var createdAt, expiresAt string
	err := s.db.QueryRow(`UPDATE bio_tryon_jobs SET status='running',updated_at=?,expires_at=?
		WHERE id=(SELECT id FROM bio_tryon_jobs WHERE status='queued' AND expires_at>?
			ORDER BY created_at,id LIMIT 1) AND status='queued'
		RETURNING id,account_id,buyer_google_account_id,product_id,status,result_path,error_code,created_at,expires_at`,
		formatBioTryOnTime(now), formatBioTryOnTime(now.Add(BioTryOnProcessingTimeout)),
		formatBioTryOnTime(now)).Scan(
		&job.ID, &job.AccountID, &job.BuyerGoogleAccountID, &job.ProductID, &job.Status, &job.ResultPath,
		&job.ErrorCode, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BioTryOnJob{}, false, nil
	}
	if err != nil {
		return BioTryOnJob{}, false, err
	}
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	job.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	return job, true, nil
}

func (s *Store) SetBioTryOnJobState(id, status, resultPath, errorCode string, at time.Time) error {
	if !validTryOnValue(id, 24, 128) {
		return ErrBioTryOnJobNotFound
	}
	switch status {
	case "running":
	case "succeeded":
		if strings.TrimSpace(resultPath) == "" {
			return fmt.Errorf("try-on result path is required")
		}
	case "failed":
		if strings.TrimSpace(errorCode) == "" {
			return fmt.Errorf("try-on error code is required")
		}
	default:
		return fmt.Errorf("invalid bio try-on job state")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	expiresAt := ""
	switch status {
	case "running":
		expiresAt = formatBioTryOnTime(at.Add(BioTryOnProcessingTimeout))
	case "succeeded":
		expiresAt = formatBioTryOnTime(at.Add(BioTryOnRetention))
	case "failed":
		expiresAt = formatBioTryOnTime(at.Add(BioTryOnFailureRetention))
	}
	result, err := s.db.Exec(`UPDATE bio_tryon_jobs SET status=?,result_path=?,error_code=?,updated_at=?,
		expires_at=CASE WHEN ?<>'' THEN ? ELSE expires_at END
		WHERE id=? AND status IN ('queued','running') AND expires_at>?`, status, resultPath, errorCode,
		formatBioTryOnTime(at), expiresAt, expiresAt, id, formatBioTryOnTime(at))
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrBioTryOnJobNotFound
	}
	return nil
}

func validTryOnValue(value string, minLen, maxLen int) bool {
	if len(value) < minLen || len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func bioLocalDay(at time.Time) string {
	return at.In(bioTimeLocation()).Format("2006-01-02")
}

func bioTimeLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("ICT", 7*60*60)
	}
	return loc
}

func formatBioTryOnTime(at time.Time) string {
	return at.UTC().Format(bioTryOnTimestampLayout)
}

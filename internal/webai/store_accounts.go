package webai

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AddAccount adds a new account to the platform keyring pool.
func (s *Store) AddAccount(acc Account) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc.Email = normalizeAccountEmail(acc.Email)

	if acc.ID == "" {
		acc.ID = fmt.Sprintf("acc_%s_%d", acc.Platform, time.Now().UnixNano())
	}
	if acc.Status == "" || acc.Status == "ok" {
		if strings.TrimSpace(acc.SessionKey) != "" || strings.TrimSpace(acc.Cookie) != "" {
			acc.Status = StatusConnected
		} else {
			acc.Status = StatusDisconnected
		}
	}
	if acc.Tier == "" {
		acc.Tier = "unknown"
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if acc.Email != "" {
		var existingID, existingKey, existingCookie string
		err := s.db.QueryRow(`
			SELECT id, session_key, cookie FROM webai_accounts WHERE platform = ? AND email = ?
		`, acc.Platform, acc.Email).Scan(&existingID, &existingKey, &existingCookie)
		if err == nil {
			if acc.SessionKey == "" {
				acc.SessionKey = existingKey
			}
			if acc.Cookie == "" {
				acc.Cookie = existingCookie
			}
			_, err = s.db.Exec(`
				UPDATE webai_accounts
				SET account_name = ?, email = ?, session_key = ?, cookie = ?, status = ?, tier = ?, tier_confirmed = ?, is_active = 1, last_checked = ?, updated_at = ?
				WHERE id = ?
			`, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, now, existingID)
			if err != nil {
				return "", fmt.Errorf("webai: update account: %w", err)
			}
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("webai: find account by email: %w", err)
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, status, tier, tier_confirmed, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, 0, 1, ?, ?, ?, ?)
	`, acc.ID, acc.Platform, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, now, now, now)
	if err != nil {
		return "", fmt.Errorf("webai: add account: %w", err)
	}
	return acc.ID, nil
}

// UpdateAccount updates existing account credentials or properties.
func (s *Store) UpdateAccount(acc Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc.Email = normalizeAccountEmail(acc.Email)

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`
		UPDATE webai_accounts
		SET account_name = ?, email = ?, session_key = ?, cookie = ?, status = ?, tier = ?, tier_confirmed = ?, updated_at = ?
		WHERE id = ?
	`, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, acc.ID)
	return err
}

// UpsertAccountSession cập nhật phiên làm việc nếu đã có tài khoản tương ứng hoặc thêm tài khoản mới vào nhóm.
func (s *Store) UpsertAccountSession(platform, sessionKey, cookie, accountName, email, tier string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if tier == "" {
		tier = "unknown"
	}
	email = normalizeAccountEmail(email)
	if accountName == "" {
		if email != "" {
			accountName = fmt.Sprintf("%s (%s)", strings.ToUpper(platform), email)
		} else {
			accountName = fmt.Sprintf("Auto Session (%s)", time.Now().Format("02/01 15:04"))
		}
	}

	var existingID, existingCookie, existingKey string

	// 1. Nếu có email: tìm chính xác tài khoản cùng platform và email để cập nhật
	if email != "" {
		err := s.db.QueryRow(`
			SELECT id, cookie, session_key FROM webai_accounts
			WHERE platform = ? AND email = ?
			ORDER BY is_active DESC, updated_at DESC LIMIT 1
		`, platform, email).Scan(&existingID, &existingCookie, &existingKey)
		if err == nil && existingID != "" {
			newKey := sessionKey
			if newKey == "" {
				newKey = existingKey
			}
			newCookie := cookie
			if newCookie == "" {
				newCookie = existingCookie
			}
			_, uerr := s.db.Exec(`
				UPDATE webai_accounts
				SET session_key = ?, cookie = ?, account_name = ?, email = ?, status = 'ok', tier = ?, tier_confirmed = 0, is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newKey, newCookie, accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 2. Nếu có sessionKey (và không phải URL gateway chung): tìm tài khoản có cùng session_key
	if sessionKey != "" && !strings.HasPrefix(sessionKey, "http://127.0.0.1") && !strings.HasPrefix(sessionKey, "http://localhost") {
		err := s.db.QueryRow(`
			SELECT id, cookie, session_key FROM webai_accounts
			WHERE platform = ? AND session_key = ?
			ORDER BY is_active DESC, updated_at DESC LIMIT 1
		`, platform, sessionKey).Scan(&existingID, &existingCookie, &existingKey)
		if err == nil && existingID != "" {
			newCookie := cookie
			if newCookie == "" {
				newCookie = existingCookie
			}
			_, uerr := s.db.Exec(`
				UPDATE webai_accounts
				SET cookie = ?, account_name = ?, email = COALESCE(NULLIF(?, ''), email), status = 'ok', tier = ?, tier_confirmed = 0, is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newCookie, accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 3. Không trùng với tài khoản nào -> THÊM MỚI một tài khoản riêng biệt vào pool
	newID := fmt.Sprintf("acc_%s_%d", platform, time.Now().UnixNano())
	_, ierr := s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, status, tier, tier_confirmed, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'ok', ?, 0, '', 0, 0, 1, ?, ?, ?, ?)
	`, newID, platform, accountName, email, sessionKey, cookie, tier, now, now, now, now)
	return newID, ierr
}

// ConfirmFreeAccount records operator confirmation that account is on a free plan.
func (s *Store) ConfirmFreeAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`UPDATE webai_accounts SET tier = 'free', tier_confirmed = 1, updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("webai: confirm free account: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("webai: check free account confirmation: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("webai: account %q not found", id)
	}
	return nil
}

// UpdateSessionCookieRefresh replaces cookies only for the matching account
// identity. Callers must have verified that the refreshed session completed a
// provider request successfully.
func (s *Store) UpdateSessionCookieRefresh(platform, email, cookie string) error {
	platform = strings.TrimSpace(platform)
	email = normalizeAccountEmail(email)
	cookie = strings.TrimSpace(cookie)
	if platform == "" || email == "" || cookie == "" {
		return fmt.Errorf("webai: platform, email, and refreshed cookie are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`
		UPDATE webai_accounts
		SET cookie = ?, status = 'ok', last_checked = ?, updated_at = ?
		WHERE platform = ? AND email = ?
	`, cookie, now, now, platform, email)
	if err != nil {
		return fmt.Errorf("webai: refresh account cookie: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("webai: verify refreshed account cookie: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("webai: account %s / %s not found", platform, email)
	}
	return nil
}

// MarkSessionExpired removes a provider-rejected session from account rotation.
func (s *Store) MarkSessionExpired(platform, email string) error {
	platform = strings.TrimSpace(platform)
	email = normalizeAccountEmail(email)
	if platform == "" || email == "" {
		return fmt.Errorf("webai: platform and email are required to mark an expired session")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`
		UPDATE webai_accounts
		SET status = 'expired', is_active = 0, last_checked = ?, updated_at = ?
		WHERE platform = ? AND email = ?
	`, now, now, platform, email)
	if err != nil {
		return fmt.Errorf("webai: mark expired session: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("webai: verify expired session: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("webai: account %s / %s not found", platform, email)
	}
	return nil
}

func normalizeAccountEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// DeleteAccount removes an account from the pool.
func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM webai_accounts WHERE id = ?`, id)
	return err
}

// GetAccount retrieves an account by ID or by platform.
// If no account exists for a platform, returns a disconnected default struct without error.
func (s *Store) GetAccount(idOrPlatform string) (*Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var acc Account
	var cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt string
	var isActive int
	err := s.db.QueryRow(`
		SELECT id, platform, account_name, email, session_key, cookie, status, tier, tier_confirmed,
		       cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at
		FROM webai_accounts
		WHERE id = ? OR platform = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, is_active DESC, updated_at DESC LIMIT 1`,
		idOrPlatform, idOrPlatform, idOrPlatform).Scan(
		&acc.ID, &acc.Platform, &acc.AccountName, &acc.Email, &acc.SessionKey, &acc.Cookie,
		&acc.Status, &acc.Tier, &acc.TierConfirmed, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
		&isActive, &lastUsed, &lastChecked, &createdAt, &updatedAt)
	if err != nil {
		return &Account{
			Platform: idOrPlatform,
			Status:   StatusDisconnected,
		}, nil
	}
	acc.IsActive = isActive == 1
	acc.fillDerived(cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt)
	return &acc, nil
}

// GetAccounts retrieves all accounts configured for a platform.
func (s *Store) GetAccounts(platform string) ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// First auto-clear expired cooldowns
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, _ = s.db.Exec(`
		UPDATE webai_accounts
		SET status = 'ok', cooldown_until = ''
		WHERE status = 'cooldown' AND cooldown_until != '' AND cooldown_until <= ?
	`, nowStr)

	rows, err := s.db.Query(`
		SELECT id, platform, account_name, email, session_key, cookie, status, tier, tier_confirmed,
		       cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at
		FROM webai_accounts
		WHERE platform = ?
		ORDER BY requests ASC, created_at ASC, id ASC`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Account
	for rows.Next() {
		var acc Account
		var cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt string
		var isActive int
		if err := rows.Scan(
			&acc.ID, &acc.Platform, &acc.AccountName, &acc.Email, &acc.SessionKey, &acc.Cookie,
			&acc.Status, &acc.Tier, &acc.TierConfirmed, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
			&isActive, &lastUsed, &lastChecked, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		acc.IsActive = isActive == 1
		acc.fillDerived(cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt)
		list = append(list, acc)
	}
	return list, nil
}

// GetAllAccounts retrieves all accounts grouped by platform.
func (s *Store) GetAllAccounts() (map[string][]Account, error) {
	res := make(map[string][]Account)
	for _, p := range SupportedPlatforms() {
		res[p.ID] = []Account{}
	}
	for _, p := range SupportedPlatforms() {
		accs, err := s.GetAccounts(p.ID)
		if err == nil {
			res[p.ID] = accs
		}
	}
	return res, nil
}

// SaveAccount maintains compatibility with legacy callers.
func (s *Store) SaveAccount(acc Account) error {
	_, err := s.AddAccount(acc)
	return err
}

// DisconnectAccount maintains compatibility with legacy callers.
func (s *Store) DisconnectAccount(platform string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM webai_accounts WHERE platform = ?`, platform)
	return err
}

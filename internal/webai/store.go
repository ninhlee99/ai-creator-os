package webai

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists Web AI platform credentials and session states with multi-account rotation.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewStore opens or creates the Web AI database.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("webai: open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("webai: pragma %q: %w", p, err)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if legacy schema exists (where platform was PRIMARY KEY)
	var hasIDCol bool
	rows, err := s.db.Query("PRAGMA table_info(webai_accounts)")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dfltValue any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
				if name == "id" {
					hasIDCol = true
				}
			}
		}
	}

	if !hasIDCol {
		// Need migration: create new table structure
		_, _ = s.db.Exec(`DROP TABLE IF EXISTS webai_accounts_v2;`)
		_, err := s.db.Exec(`
		CREATE TABLE webai_accounts_v2 (
			id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			account_name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			session_key TEXT NOT NULL DEFAULT '',
			cookie TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			tier TEXT NOT NULL DEFAULT 'free',
			cooldown_until TEXT NOT NULL DEFAULT '',
			requests INTEGER NOT NULL DEFAULT 0,
			rate_limit_hits INTEGER NOT NULL DEFAULT 0,
			is_active INTEGER NOT NULL DEFAULT 0,
			last_used TEXT NOT NULL DEFAULT (datetime('now')),
			last_checked TEXT NOT NULL DEFAULT (datetime('now')),
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		`)
		if err != nil {
			return fmt.Errorf("webai: create v2 table: %w", err)
		}

		// Copy legacy data if legacy table exists
		var tableExists int
		_ = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='webai_accounts'`).Scan(&tableExists)
		if tableExists > 0 {
			_, _ = s.db.Exec(`
				INSERT INTO webai_accounts_v2 (id, platform, account_name, email, session_key, cookie, status, tier, is_active, last_checked, created_at, updated_at)
				SELECT 'acc_' || platform || '_' || hex(randomblob(4)), platform, account_name, email, session_key, cookie,
				       CASE WHEN status = 'connected' THEN 'ok' ELSE 'disconnected' END,
				       tier, 1, last_checked, created_at, updated_at
				FROM webai_accounts;
			`)
			_, _ = s.db.Exec(`DROP TABLE webai_accounts;`)
		}
		_, _ = s.db.Exec(`ALTER TABLE webai_accounts_v2 RENAME TO webai_accounts;`)
	}

	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_webai_accounts_platform ON webai_accounts(platform);`)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// AddAccount adds a new account to the platform keyring pool.
func (s *Store) AddAccount(acc Account) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		acc.Tier = "free"
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, status, tier, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 0, 0, 1, ?, ?, ?, ?)
	`, acc.ID, acc.Platform, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, acc.Status, acc.Tier, now, now, now, now)
	if err != nil {
		return "", fmt.Errorf("webai: add account: %w", err)
	}
	return acc.ID, nil
}

// UpdateAccount updates existing account credentials or properties.
func (s *Store) UpdateAccount(acc Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`
		UPDATE webai_accounts
		SET account_name = ?, email = ?, session_key = ?, cookie = ?, status = ?, tier = ?, updated_at = ?
		WHERE id = ?
	`, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, acc.Status, acc.Tier, now, acc.ID)
	return err
}

// UpsertAccountSession cập nhật phiên làm việc nếu đã có tài khoản tương ứng hoặc thêm tài khoản mới vào nhóm.
func (s *Store) UpsertAccountSession(platform, sessionKey, cookie, accountName, email, tier string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if tier == "" {
		tier = "free"
	}
	email = strings.TrimSpace(email)
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
				SET session_key = ?, cookie = ?, account_name = ?, status = 'ok', tier = ?, is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newKey, newCookie, accountName, tier, now, now, existingID)
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
				SET cookie = ?, account_name = ?, email = COALESCE(NULLIF(?, ''), email), status = 'ok', tier = ?, is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newCookie, accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 3. Không trùng với tài khoản nào -> THÊM MỚI một tài khoản riêng biệt vào pool
	newID := fmt.Sprintf("acc_%s_%d", platform, time.Now().UnixNano())
	_, ierr := s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, status, tier, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'ok', ?, '', 0, 0, 1, ?, ?, ?, ?)
	`, newID, platform, accountName, email, sessionKey, cookie, tier, now, now, now, now)
	return newID, ierr
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
		SELECT id, platform, account_name, email, session_key, cookie, status, tier,
		       cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at
		FROM webai_accounts
		WHERE id = ? OR platform = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, is_active DESC, updated_at DESC LIMIT 1`,
		idOrPlatform, idOrPlatform, idOrPlatform).Scan(
		&acc.ID, &acc.Platform, &acc.AccountName, &acc.Email, &acc.SessionKey, &acc.Cookie,
		&acc.Status, &acc.Tier, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
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
		SELECT id, platform, account_name, email, session_key, cookie, status, tier,
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
			&acc.Status, &acc.Tier, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
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

// PickActiveAccount selects the next available account for the platform (round-robin / least used).
// If an account is currently in cooldown, it automatically checks the next account.
func (s *Store) PickActiveAccount(platform string) (*Account, error) {
	accs, err := s.GetAccounts(platform)
	if err != nil {
		return nil, err
	}
	if len(accs) == 0 {
		return nil, fmt.Errorf("chưa có tài khoản %s nào — hãy bấm '+ Thêm tài khoản web' trong AI Web Hub", platform)
	}

	now := time.Now().UTC()
	var shortestCool *Account
	var shortestRemain int64 = 9999999

	for i := range accs {
		acc := &accs[i]
		if !acc.IsActive || acc.Status == "invalid" || acc.Status == "expired" || acc.Status == "disconnected" {
			continue
		}
		if acc.Status == "cooldown" && !acc.CooldownUntil.IsZero() && acc.CooldownUntil.After(now) {
			remain := int64(acc.CooldownUntil.Sub(now).Seconds())
			if remain < shortestRemain {
				shortestRemain = remain
				shortestCool = acc
			}
			continue
		}
		// Found an eligible OK account: record usage and return
		s.mu.Lock()
		nowStr := now.Format("2006-01-02 15:04:05")
		_, _ = s.db.Exec(`
			UPDATE webai_accounts
			SET requests = requests + 1, last_used = ?, last_checked = ?, is_active = 1
			WHERE id = ?
		`, nowStr, nowStr, acc.ID)
		s.mu.Unlock()

		acc.Requests++
		acc.Status = "ok"
		return acc, nil
	}

	if shortestCool != nil {
		mins := (shortestRemain + 59) / 60
		return nil, fmt.Errorf("toàn bộ %d tài khoản %s đang bị giới hạn (rate-limited) — tài khoản tiếp theo (%s) sẽ mở lại sau %d phút",
			len(accs), platform, shortestCool.Email, mins)
	}

	return nil, fmt.Errorf("không có tài khoản %s nào hợp lệ", platform)
}

// MarkRateLimited puts an account into temporary cooldown and triggers auto-rotation.
func (s *Store) MarkRateLimited(id string, cooldownDuration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	coolUntil := time.Now().UTC().Add(cooldownDuration).Format("2006-01-02 15:04:05")
	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`
		UPDATE webai_accounts
		SET status = 'cooldown', cooldown_until = ?, rate_limit_hits = rate_limit_hits + 1, updated_at = ?
		WHERE id = ?
	`, coolUntil, nowStr, id)
	return err
}

// ResetCooldown manually clears the cooldown status for an account.
func (s *Store) ResetCooldown(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowStr := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`
		UPDATE webai_accounts
		SET status = 'ok', cooldown_until = '', updated_at = ?
		WHERE id = ?
	`, nowStr, id)
	return err
}

// CountConnected returns the total number of ready/connected web accounts across all platforms.
func (s *Store) CountConnected() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM webai_accounts WHERE status IN ('ok', 'connected')`).Scan(&count)
	return count
}

// CountByPlatform returns counts of ready, cooldown, and total accounts for a platform.
func (s *Store) CountByPlatform(platform string) (ready, cooldown, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT status, cooldown_until FROM webai_accounts WHERE platform = ?`, platform)
	if err != nil {
		return 0, 0, 0
	}
	defer rows.Close()

	now := time.Now().UTC()
	for rows.Next() {
		var st, cool string
		if err := rows.Scan(&st, &cool); err == nil {
			total++
			if st == "cooldown" {
				if t, perr := time.Parse("2006-01-02 15:04:05", cool); perr == nil && t.After(now) {
					cooldown++
				} else {
					ready++
				}
			} else if st == "ok" || st == "connected" {
				ready++
			}
		}
	}
	return ready, cooldown, total
}

// Helper to fill derived fields like CooldownRemainingSec and CooldownLabel
func (acc *Account) fillDerived(cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt string) {
	acc.LastUsed, _ = time.Parse("2006-01-02 15:04:05", lastUsed)
	acc.LastChecked, _ = time.Parse("2006-01-02 15:04:05", lastChecked)
	acc.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	acc.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)

	if cooldownUntil != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", cooldownUntil); err == nil {
			acc.CooldownUntil = t
			now := time.Now().UTC()
			if t.After(now) {
				acc.CooldownRemainingSec = int64(t.Sub(now).Seconds())
				mins := (acc.CooldownRemainingSec + 59) / 60
				acc.CooldownLabel = fmt.Sprintf("Nghỉ còn %dp", mins)
				acc.Status = "cooldown"
			} else {
				acc.Status = "ok"
				acc.CooldownRemainingSec = 0
			}
		}
	}
	if acc.RateLimitHits > 0 {
		acc.LastRateLimitLabel = fmt.Sprintf("Bị rate-limit %d lần", acc.RateLimitHits)
	}
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

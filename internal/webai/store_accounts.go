package webai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AddAccount adds a new account to the platform keyring pool.
func (s *Store) AddAccount(acc Account) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	platform, err := normalizeAccountPlatform(acc.Platform)
	if err != nil {
		return "", err
	}
	acc.Platform = platform
	acc.Email = normalizeAccountEmail(acc.Email)
	acc.SessionKey = strings.TrimSpace(acc.SessionKey)
	acc.Cookie = strings.TrimSpace(acc.Cookie)
	acc.CookieJar = normalizeCookieJar(acc.CookieJar)
	if acc.Cookie == "" && len(acc.CookieJar) > 0 {
		acc.Cookie = cookieHeaderFromSnapshot(acc.CookieJar)
	}

	if acc.ID == "" {
		acc.ID = fmt.Sprintf("acc_%s_%d", acc.Platform, time.Now().UnixNano())
	}
	if acc.Tier == "" {
		acc.Tier = "unknown"
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if acc.Email != "" {
		var existingID, existingKey, existingCookie, existingCookieJar string
		err := s.db.QueryRow(`
			SELECT id, session_key, cookie, cookie_jar FROM webai_accounts WHERE platform = ? AND email = ?
		`, acc.Platform, acc.Email).Scan(&existingID, &existingKey, &existingCookie, &existingCookieJar)
		if err == nil {
			if acc.SessionKey == "" {
				acc.SessionKey = existingKey
			}
			if acc.Cookie == "" {
				acc.Cookie = existingCookie
			}
			if len(acc.CookieJar) == 0 {
				acc.CookieJar = decodeCookieJar(existingCookieJar)
			}
			if len(acc.CookieJar) == 0 {
				acc.CookieJar = mergeCookieJarHeader(nil, acc.Cookie, acc.Platform)
			} else {
				acc.CookieJar = mergeCookieHeaderIntoSnapshot(acc.CookieJar, acc.Cookie, acc.Platform)
			}
			normalizeAccountStatus(&acc)
			_, err = s.db.Exec(`
				UPDATE webai_accounts
				SET account_name = ?, email = ?, session_key = ?, cookie = ?, cookie_jar = ?, status = ?, tier = ?, tier_confirmed = ?, is_active = 1, last_checked = ?, updated_at = ?
				WHERE id = ?
			`, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, encodeCookieJar(acc.CookieJar), acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, now, existingID)
			if err != nil {
				return "", fmt.Errorf("webai: update account: %w", err)
			}
			return existingID, nil
		}
		if err != sql.ErrNoRows {
			return "", fmt.Errorf("webai: find account by email: %w", err)
		}
	}

	if len(acc.CookieJar) == 0 && acc.Cookie != "" {
		acc.CookieJar = mergeCookieJarHeader(nil, acc.Cookie, acc.Platform)
	}
	normalizeAccountStatus(&acc)
	_, err = s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, cookie_jar, status, tier, tier_confirmed, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, 0, 1, ?, ?, ?, ?)
	`, acc.ID, acc.Platform, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, encodeCookieJar(acc.CookieJar), acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, now, now, now)
	if err != nil {
		return "", fmt.Errorf("webai: add account: %w", err)
	}
	return acc.ID, nil
}

func encodeCookieJar(jar []cdpCookie) string {
	if len(jar) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(jar)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeCookieJar(raw string) []cdpCookie {
	var jar []cdpCookie
	if json.Unmarshal([]byte(raw), &jar) != nil {
		return nil
	}
	return normalizeCookieJar(jar)
}

func decodeAccountCookieJar(raw, cookie, platform string) []BrowserCookie {
	jar := decodeCookieJar(raw)
	if len(jar) == 0 && strings.TrimSpace(cookie) != "" {
		return mergeCookieJarHeader(nil, cookie, platform)
	}
	return jar
}

func normalizeCookieJar(jar []cdpCookie) []cdpCookie {
	if len(jar) == 0 {
		return nil
	}
	out := make([]cdpCookie, 0, len(jar))
	for _, cookie := range jar {
		if strings.TrimSpace(cookie.Name) == "" || strings.TrimSpace(cookie.Domain) == "" {
			continue
		}
		if !cookie.Session && cookie.Expires > 0 && cookie.Expires <= float64(time.Now().Unix()) {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		if len(cookie.PartitionKey) > 0 {
			cookie.Partitioned = true
		}
		out = append(out, cookie)
	}
	return out
}

func cookieDefaultDomain(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformChatGPT:
		return ".chatgpt.com"
	case PlatformClaude:
		return ".claude.ai"
	case PlatformGemini:
		return ".google.com"
	case PlatformGrok:
		return ".grok.com"
	case PlatformGoogleFlow:
		return ".google.com"
	case PlatformMuseAI:
		return ".muse.ai"
	default:
		return ""
	}
}

func mergeCookieJarHeader(jar []cdpCookie, header, platform string) []cdpCookie {
	jar = normalizeCookieJar(jar)
	defaultDomain := cookieDefaultDomain(platform)
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		updated := false
		for i := range jar {
			if jar[i].Name == name {
				jar[i].Value = value
				updated = true
				break
			}
		}
		if !updated && defaultDomain != "" {
			jar = append(jar, cdpCookie{Name: name, Value: value, Domain: defaultDomain, Path: "/", Secure: true, Session: true})
		}
	}
	return jar
}

func normalizeAccountPlatform(raw string) (string, error) {
	platform := strings.ToLower(strings.TrimSpace(raw))
	switch platform {
	case "muse", "muse_ai":
		platform = PlatformMuseAI
	case "google_flow", "google-flow":
		platform = PlatformGoogleFlow
	}
	for _, supported := range SupportedPlatforms() {
		if platform == supported.ID {
			return platform, nil
		}
	}
	return "", fmt.Errorf("webai: unsupported platform %q", strings.TrimSpace(raw))
}

func normalizeAccountStatus(acc *Account) {
	if acc == nil {
		return
	}
	if !accountHasProviderCredentials(*acc) {
		acc.Status = StatusDisconnected
		return
	}
	if acc.Status == "" || acc.Status == "ok" {
		acc.Status = StatusConnected
	}
}

func accountHasProviderCredentials(acc Account) bool {
	if acc.Platform == PlatformMuseAI || acc.Platform == PlatformGoogleFlow {
		if strings.TrimSpace(acc.Cookie) != "" {
			return true
		}
		if strings.TrimSpace(acc.SessionKey) != "" {
			return true
		}
		for _, cookie := range acc.CookieJar {
			if strings.TrimSpace(cookie.Name) != "" && cookie.Value != "" {
				return true
			}
		}
		return false
	}
	return strings.TrimSpace(acc.SessionKey) != "" || strings.TrimSpace(acc.Cookie) != ""
}

// UpdateAccount updates existing account credentials or properties.
func (s *Store) UpdateAccount(acc Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	acc.Email = normalizeAccountEmail(acc.Email)
	acc.CookieJar = normalizeCookieJar(acc.CookieJar)
	if len(acc.CookieJar) == 0 {
		var rawCookieJar string
		if err := s.db.QueryRow(`SELECT cookie_jar FROM webai_accounts WHERE id = ?`, acc.ID).Scan(&rawCookieJar); err == nil {
			acc.CookieJar = decodeAccountCookieJar(rawCookieJar, acc.Cookie, acc.Platform)
		}
	} else {
		acc.CookieJar = mergeCookieHeaderIntoSnapshot(acc.CookieJar, acc.Cookie, acc.Platform)
	}
	if !accountHasProviderCredentials(acc) {
		acc.Status = StatusDisconnected
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Exec(`
		UPDATE webai_accounts
		SET account_name = ?, email = ?, session_key = ?, cookie = ?, cookie_jar = ?, status = ?, tier = ?, tier_confirmed = ?, updated_at = ?
		WHERE id = ?
	`, acc.AccountName, acc.Email, acc.SessionKey, acc.Cookie, encodeCookieJar(acc.CookieJar), acc.Status, acc.Tier, boolInt(acc.TierConfirmed), now, acc.ID)
	return err
}

// UpdateAccountProbeStatus persists only probe-owned fields. Credential
// rotation can happen concurrently, so account probes must not write a stale
// Account snapshot back over a newer cookie.
func (s *Store) UpdateAccountProbeStatus(id, status string, preserveCooldown bool) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("webai: account ID is required")
	}
	switch status {
	case "ok", StatusConnected, StatusExpired, StatusUnverified:
	default:
		return "", fmt.Errorf("webai: invalid probe status %q", status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var currentStatus string
	if err := s.db.QueryRow(`SELECT status FROM webai_accounts WHERE id = ?`, id).Scan(&currentStatus); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("webai: account %q not found", id)
		}
		return "", fmt.Errorf("webai: read account probe status: %w", err)
	}
	if preserveCooldown && currentStatus == "cooldown" {
		status = currentStatus
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`UPDATE webai_accounts SET status = ?, last_checked = ?, updated_at = ? WHERE id = ?`, status, now, now, id)
	if err != nil {
		return "", fmt.Errorf("webai: update account probe status: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return "", fmt.Errorf("webai: verify account probe status: %w", err)
	} else if count == 0 {
		return "", fmt.Errorf("webai: account %q not found", id)
	}
	return status, nil
}

// UpsertAccountSession cập nhật phiên làm việc nếu đã có tài khoản tương ứng hoặc thêm tài khoản mới vào nhóm.
func (s *Store) UpsertAccountSession(platform, sessionKey, cookie, accountName, email, tier string) (string, error) {
	return s.UpsertAccountSessionWithCookieJar(platform, sessionKey, cookie, nil, accountName, email, tier)
}

func (s *Store) UpsertAccountSessionWithCookieJar(platform, sessionKey, cookie string, cookieJar []cdpCookie, accountName, email, tier string) (string, error) {
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

	cookieJar = normalizeCookieJar(cookieJar)
	var existingID, existingCookie, existingKey, existingCookieJar string

	// 0. TikTok: Chỉ duy trì duy nhất 1 tài khoản TikTok trong hệ thống (Single TikTok Account)
	if platform == PlatformTikTok {
		err := s.db.QueryRow(`
			SELECT id, cookie, session_key, cookie_jar FROM webai_accounts
			WHERE platform = ?
			ORDER BY is_active DESC, updated_at DESC LIMIT 1
		`, platform).Scan(&existingID, &existingCookie, &existingKey, &existingCookieJar)
		if err == nil && existingID != "" {
			newKey := sessionKey
			if newKey == "" {
				newKey = existingKey
			}
			newCookie := cookie
			if newCookie == "" {
				newCookie = existingCookie
			}
			newCookieJar := cookieJar
			if len(newCookieJar) == 0 {
				newCookieJar = decodeCookieJar(existingCookieJar)
			}
			if len(newCookieJar) == 0 {
				newCookieJar = mergeCookieJarHeader(nil, newCookie, platform)
			} else {
				newCookieJar = mergeCookieHeaderIntoSnapshot(newCookieJar, newCookie, platform)
			}
			_, uerr := s.db.Exec(`
				UPDATE webai_accounts
				SET session_key = ?, cookie = ?, cookie_jar = ?, account_name = ?, email = ?, status = 'ok',
				    tier = ?, tier_confirmed = 0,
				    is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newKey, newCookie, encodeCookieJar(newCookieJar), accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 1. Nếu có email: tìm chính xác tài khoản cùng platform và email để cập nhật
	if email != "" {
		err := s.db.QueryRow(`
			SELECT id, cookie, session_key, cookie_jar FROM webai_accounts
			WHERE platform = ? AND email = ?
			ORDER BY is_active DESC, updated_at DESC LIMIT 1
		`, platform, email).Scan(&existingID, &existingCookie, &existingKey, &existingCookieJar)
		if err == nil && existingID != "" {
			newKey := sessionKey
			if newKey == "" {
				newKey = existingKey
			}
			newCookie := cookie
			if newCookie == "" {
				newCookie = existingCookie
			}
			newCookieJar := cookieJar
			if len(newCookieJar) == 0 {
				newCookieJar = decodeCookieJar(existingCookieJar)
			}
			if len(newCookieJar) == 0 {
				newCookieJar = mergeCookieJarHeader(nil, newCookie, platform)
			} else {
				newCookieJar = mergeCookieHeaderIntoSnapshot(newCookieJar, newCookie, platform)
			}
			_, uerr := s.db.Exec(`
				UPDATE webai_accounts
				SET session_key = ?, cookie = ?, cookie_jar = ?, account_name = ?, email = ?, status = 'ok',
				    tier = ?, tier_confirmed = 0,
				    is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newKey, newCookie, encodeCookieJar(newCookieJar), accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 2. Nếu có sessionKey (và không phải URL gateway chung): tìm tài khoản có cùng session_key
	if sessionKey != "" && !strings.HasPrefix(sessionKey, "http://127.0.0.1") && !strings.HasPrefix(sessionKey, "http://localhost") {
		err := s.db.QueryRow(`
			SELECT id, cookie, session_key, cookie_jar FROM webai_accounts
			WHERE platform = ? AND session_key = ?
			ORDER BY is_active DESC, updated_at DESC LIMIT 1
		`, platform, sessionKey).Scan(&existingID, &existingCookie, &existingKey, &existingCookieJar)
		if err == nil && existingID != "" {
			newCookie := cookie
			if newCookie == "" {
				newCookie = existingCookie
			}
			newCookieJar := cookieJar
			if len(newCookieJar) == 0 {
				newCookieJar = decodeCookieJar(existingCookieJar)
			}
			if len(newCookieJar) == 0 {
				newCookieJar = mergeCookieJarHeader(nil, newCookie, platform)
			} else {
				newCookieJar = mergeCookieHeaderIntoSnapshot(newCookieJar, newCookie, platform)
			}
			_, uerr := s.db.Exec(`
				UPDATE webai_accounts
				SET cookie = ?, cookie_jar = ?, account_name = ?, email = COALESCE(NULLIF(?, ''), email), status = 'ok',
				    tier = ?, tier_confirmed = 0,
				    is_active = 1, updated_at = ?, last_checked = ?
				WHERE id = ?
			`, newCookie, encodeCookieJar(newCookieJar), accountName, email, tier, now, now, existingID)
			return existingID, uerr
		}
	}

	// 3. Không trùng với tài khoản nào -> THÊM MỚI một tài khoản riêng biệt vào pool
	if len(cookieJar) == 0 {
		cookieJar = mergeCookieJarHeader(nil, cookie, platform)
	} else {
		cookieJar = mergeCookieHeaderIntoSnapshot(cookieJar, cookie, platform)
	}
	newID := fmt.Sprintf("acc_%s_%d", platform, time.Now().UnixNano())
	_, ierr := s.db.Exec(`
		INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, cookie_jar, status, tier, tier_confirmed, cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'ok', ?, 0, '', 0, 0, 1, ?, ?, ?, ?)
	`, newID, platform, accountName, email, sessionKey, cookie, encodeCookieJar(cookieJar), tier, now, now, now, now)
	return newID, ierr
}

// UpdateAccountCookieRefresh persists a cookie snapshot by stable account ID.
// It deliberately leaves account status untouched: a response may be blocked
// by Cloudflare while still rotating cookies.
func (s *Store) UpdateAccountCookieRefresh(id, cookie string) error {
	id = strings.TrimSpace(id)
	cookie = strings.TrimSpace(cookie)
	if id == "" {
		return fmt.Errorf("webai: account ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var platform, sessionKey, previousCookie, rawCookieJar string
	if err := s.db.QueryRow(`SELECT platform, session_key, cookie, cookie_jar FROM webai_accounts WHERE id = ?`, id).Scan(&platform, &sessionKey, &previousCookie, &rawCookieJar); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("webai: account %q not found", id)
		}
		return fmt.Errorf("webai: read account before cookie refresh: %w", err)
	}
	refreshedSessionKey := refreshedCookieSessionKey(sessionKey, previousCookie, cookie)
	refreshedCookieJar := mergeCookieJarHeader(decodeCookieJar(rawCookieJar), cookie, platform)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`UPDATE webai_accounts SET session_key = ?, cookie = ?, cookie_jar = ?, updated_at = ? WHERE id = ?`, refreshedSessionKey, cookie, encodeCookieJar(refreshedCookieJar), now, id)
	if err != nil {
		return fmt.Errorf("webai: refresh account cookie: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("webai: verify refreshed account cookie: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("webai: account %q not found", id)
	}
	return nil
}

func (s *Store) UpdateAccountCookieSnapshot(id, cookie string, cookieJar []BrowserCookie) error {
	id = strings.TrimSpace(id)
	cookie = strings.TrimSpace(cookie)
	if id == "" {
		return fmt.Errorf("webai: account ID is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sessionKey, previousCookie string
	if err := s.db.QueryRow(`SELECT session_key, cookie FROM webai_accounts WHERE id = ?`, id).Scan(&sessionKey, &previousCookie); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("webai: account %q not found", id)
		}
		return fmt.Errorf("webai: read account before cookie snapshot: %w", err)
	}
	refreshedSessionKey := refreshedCookieSessionKey(sessionKey, previousCookie, cookie)
	result, err := s.db.Exec(`UPDATE webai_accounts SET session_key = ?, cookie = ?, cookie_jar = ?, updated_at = ? WHERE id = ?`, refreshedSessionKey, cookie, encodeCookieJar(normalizeCookieJar(cookieJar)), time.Now().UTC().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return fmt.Errorf("webai: persist account cookie snapshot: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("webai: verify refreshed cookie snapshot: %w", err)
	} else if count == 0 {
		return fmt.Errorf("webai: account %q not found", id)
	}
	return nil
}

func refreshedCookieSessionKey(sessionKey, previousCookie, refreshedCookie string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || refreshedCookie == "" {
		return ""
	}
	for _, pair := range strings.Split(previousCookie, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || name == "" || value != sessionKey {
			continue
		}
		for _, refreshedPair := range strings.Split(refreshedCookie, ";") {
			refreshedName, refreshedValue, refreshedOK := strings.Cut(strings.TrimSpace(refreshedPair), "=")
			if refreshedOK && refreshedName == name && refreshedValue != "" {
				return refreshedValue
			}
		}
		return ""
	}
	return sessionKey
}

// UpdateSessionCookieRefresh persists cookies for matching legacy callers.
// It never changes status because cookie rotation alone does not prove session
// validity.
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
		SET cookie = ?, updated_at = ?
		WHERE platform = ? AND email = ?
	`, cookie, now, platform, email)
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
	var rawCookieJar string
	var isActive int
	err := s.db.QueryRow(`
		SELECT id, platform, account_name, email, session_key, cookie, cookie_jar, status, tier, tier_confirmed,
		       cooldown_until, requests, rate_limit_hits, is_active, last_used, last_checked, created_at, updated_at
		FROM webai_accounts
		WHERE id = ? OR platform = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, is_active DESC, updated_at DESC LIMIT 1`,
		idOrPlatform, idOrPlatform, idOrPlatform).Scan(
		&acc.ID, &acc.Platform, &acc.AccountName, &acc.Email, &acc.SessionKey, &acc.Cookie, &rawCookieJar,
		&acc.Status, &acc.Tier, &acc.TierConfirmed, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
		&isActive, &lastUsed, &lastChecked, &createdAt, &updatedAt)
	if err != nil {
		return &Account{
			Platform: idOrPlatform,
			Status:   StatusDisconnected,
		}, nil
	}
	acc.CookieJar = decodeAccountCookieJar(rawCookieJar, acc.Cookie, acc.Platform)
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
	if _, err := s.db.Exec(`
		UPDATE webai_accounts
		SET status = 'ok', cooldown_until = ''
		WHERE status = 'cooldown' AND cooldown_until != '' AND cooldown_until <= ?
	`, nowStr); err != nil {
		return nil, fmt.Errorf("webai: clear expired cooldowns: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT id, platform, account_name, email, session_key, cookie, cookie_jar, status, tier, tier_confirmed,
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
		var rawCookieJar string
		var isActive int
		if err := rows.Scan(
			&acc.ID, &acc.Platform, &acc.AccountName, &acc.Email, &acc.SessionKey, &acc.Cookie, &rawCookieJar,
			&acc.Status, &acc.Tier, &acc.TierConfirmed, &cooldownUntil, &acc.Requests, &acc.RateLimitHits,
			&isActive, &lastUsed, &lastChecked, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		acc.CookieJar = decodeAccountCookieJar(rawCookieJar, acc.Cookie, acc.Platform)
		acc.IsActive = isActive == 1
		acc.fillDerived(cooldownUntil, lastUsed, lastChecked, createdAt, updatedAt)
		list = append(list, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
		if err != nil {
			return nil, fmt.Errorf("webai: list %s accounts: %w", p.ID, err)
		}
		res[p.ID] = accs
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

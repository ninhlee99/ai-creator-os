package webai

import (
	"fmt"
	"time"
)

// PickActiveAccount selects the next available account for the platform (round-robin / least used).
// If an account is currently in cooldown, it automatically checks the next account.
func (s *Store) PickActiveAccount(platform string) (*Account, error) {
	return s.PickActiveAccountExcluding(platform, nil)
}

// PickActiveAccountExcluding selects an available account not already tried by
// the current request.
func (s *Store) PickActiveAccountExcluding(platform string, excluded map[string]struct{}) (*Account, error) {
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
	eligibleCount := 0
	credentiallessCount := 0

	for i := range accs {
		acc := &accs[i]
		if !acc.IsActive {
			continue
		}
		if _, tried := excluded[acc.ID]; tried {
			continue
		}
		if !accountHasProviderCredentials(*acc) {
			credentiallessCount++
			continue
		}
		eligibleCount++
		if acc.Status == "cooldown" && !acc.CooldownUntil.IsZero() && acc.CooldownUntil.After(now) {
			remain := int64(acc.CooldownUntil.Sub(now).Seconds())
			if remain < shortestRemain {
				shortestRemain = remain
				shortestCool = acc
			}
			continue
		}
		if acc.Status != "ok" && acc.Status != "connected" {
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
		return nil, fmt.Errorf("không còn tài khoản %s khả dụng; tài khoản tiếp theo (%s) có thể dùng sau %d phút",
			platform, shortestCool.Email, mins)
	}

	if eligibleCount == 0 {
		if credentiallessCount > 0 {
			return nil, fmt.Errorf("không còn tài khoản %s có cookie hoặc khóa phiên để dùng", platform)
		}
		return nil, fmt.Errorf("không còn tài khoản %s đang bật để dùng; hãy kết nối hoặc bật một tài khoản", platform)
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

// CountConnected counts active accounts with saved provider credentials and a
// connected/ok status. It does not probe provider access.
func (s *Store) CountConnected() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	rows, err := s.db.Query(`SELECT platform, status, session_key, cookie, cookie_jar FROM webai_accounts WHERE is_active = 1`)
	if err != nil {
		return 0
	}
	defer rows.Close()
	for rows.Next() {
		var platform, status, sessionKey, cookie, cookieJar string
		if err := rows.Scan(&platform, &status, &sessionKey, &cookie, &cookieJar); err == nil &&
			(platform == PlatformMuseAI || platform == PlatformGoogleFlow) &&
			(status == "ok" || status == StatusConnected) &&
			accountHasProviderCredentials(Account{Platform: platform, SessionKey: sessionKey, Cookie: cookie, CookieJar: decodeAccountCookieJar(cookieJar, cookie, platform)}) {
			count++
		}
	}
	if rows.Err() != nil {
		return 0
	}
	return count
}

// CountByPlatform returns counts of ready, cooldown, and total accounts for a platform.
func (s *Store) CountByPlatform(platform string) (ready, cooldown, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT status, cooldown_until, is_active, session_key, cookie, cookie_jar FROM webai_accounts WHERE platform = ?`, platform)
	if err != nil {
		return 0, 0, 0
	}
	defer rows.Close()

	now := time.Now().UTC()
	for rows.Next() {
		var st, cool, sessionKey, cookie, cookieJar string
		var isActive int
		if err := rows.Scan(&st, &cool, &isActive, &sessionKey, &cookie, &cookieJar); err == nil {
			total++
			if isActive != 1 || !accountHasProviderCredentials(Account{Platform: platform, SessionKey: sessionKey, Cookie: cookie, CookieJar: decodeAccountCookieJar(cookieJar, cookie, platform)}) {
				continue
			}
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
	if !accountHasProviderCredentials(*acc) {
		acc.Status = StatusDisconnected
	}
}

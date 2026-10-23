package webai

import (
	"database/sql"
	"fmt"
)

func (s *Store) migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("webai: begin account migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Check if legacy schema exists (where platform was PRIMARY KEY)
	var hasIDCol bool
	rows, err := tx.Query("PRAGMA table_info(webai_accounts)")
	if err != nil {
		return fmt.Errorf("webai: inspect account schema: %w", err)
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("webai: scan account schema: %w", err)
		}
		if name == "id" {
			hasIDCol = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("webai: inspect account schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("webai: close account schema: %w", err)
	}

	if !hasIDCol {
		var legacyExists, v2Exists int
		if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='webai_accounts'`).Scan(&legacyExists); err != nil {
			return fmt.Errorf("webai: inspect legacy account table: %w", err)
		}
		if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='webai_accounts_v2'`).Scan(&v2Exists); err != nil {
			return fmt.Errorf("webai: inspect staged account table: %w", err)
		}
		if v2Exists == 0 {
			_, err = tx.Exec(`
		CREATE TABLE webai_accounts_v2 (
			id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			account_name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			session_key TEXT NOT NULL DEFAULT '',
			cookie TEXT NOT NULL DEFAULT '',
			cookie_jar TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'ok',
			tier TEXT NOT NULL DEFAULT 'unknown',
			tier_confirmed INTEGER NOT NULL DEFAULT 0,
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
		}

		// Preserve staged credentials from older interrupted migrations, then
		// merge the legacy table. Transaction rollback keeps both originals intact
		// if any copy, drop, or rename fails.
		if legacyExists > 0 {
			_, err := tx.Exec(`
				INSERT INTO webai_accounts_v2 (id, platform, account_name, email, session_key, cookie, status, tier, is_active, last_checked, created_at, updated_at)
				SELECT 'acc_' || platform || '_' || hex(randomblob(4)), platform, account_name, email, session_key, cookie,
				       CASE WHEN status = 'connected' THEN 'ok' ELSE 'disconnected' END,
				       tier, 1, last_checked, created_at, updated_at
				FROM webai_accounts;
			`)
			if err != nil {
				return fmt.Errorf("webai: copy legacy account credentials: %w", err)
			}
			if _, err := tx.Exec(`DROP TABLE webai_accounts;`); err != nil {
				return fmt.Errorf("webai: drop migrated legacy accounts: %w", err)
			}
		}
		if _, err := tx.Exec(`ALTER TABLE webai_accounts_v2 RENAME TO webai_accounts;`); err != nil {
			return fmt.Errorf("webai: activate migrated account table: %w", err)
		}
	}

	hasTierConfirmed, err := accountTableHasColumn(tx, "tier_confirmed")
	if err != nil {
		return fmt.Errorf("webai: inspect account tier confirmation: %w", err)
	}
	if !hasTierConfirmed {
		if _, err := tx.Exec(`ALTER TABLE webai_accounts ADD COLUMN tier_confirmed INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("webai: add account tier confirmation: %w", err)
		}
	}
	hasCookieJar, err := accountTableHasColumn(tx, "cookie_jar")
	if err != nil {
		return fmt.Errorf("webai: inspect account cookie metadata: %w", err)
	}
	if !hasCookieJar {
		if _, err := tx.Exec(`ALTER TABLE webai_accounts ADD COLUMN cookie_jar TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("webai: add account cookie metadata: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE webai_accounts SET email = lower(trim(email)) WHERE email != '';`); err != nil {
		return fmt.Errorf("webai: normalize account emails: %w", err)
	}
	// Keep the newest identity row, but carry forward its most recent non-empty
	// credentials and active state before deleting duplicate rows. Older store
	// versions could save the same email more than once with different cookie
	// fields, so deleting duplicates directly can discard the only usable cookie.
	if _, err := tx.Exec(`
		UPDATE webai_accounts AS canonical
		SET cookie = CASE WHEN trim(canonical.cookie) != '' THEN canonical.cookie ELSE COALESCE((
			SELECT candidate.cookie FROM webai_accounts AS candidate
			WHERE candidate.platform = canonical.platform AND candidate.email = canonical.email
			  AND trim(candidate.cookie) != ''
			ORDER BY candidate.updated_at DESC, candidate.id DESC LIMIT 1
		), '') END,
		cookie_jar = CASE WHEN trim(canonical.cookie_jar) NOT IN ('', '[]') THEN canonical.cookie_jar ELSE COALESCE((
			SELECT candidate.cookie_jar FROM webai_accounts AS candidate
			WHERE candidate.platform = canonical.platform AND candidate.email = canonical.email
			  AND trim(candidate.cookie_jar) NOT IN ('', '[]')
			ORDER BY candidate.updated_at DESC, candidate.id DESC LIMIT 1
		), '[]') END,
		session_key = CASE WHEN trim(canonical.session_key) != '' THEN canonical.session_key ELSE COALESCE((
			SELECT candidate.session_key FROM webai_accounts AS candidate
			WHERE candidate.platform = canonical.platform AND candidate.email = canonical.email
			  AND trim(candidate.session_key) != ''
			ORDER BY candidate.updated_at DESC, candidate.id DESC LIMIT 1
		), '') END,
		is_active = MAX(canonical.is_active, COALESCE((
			SELECT MAX(candidate.is_active) FROM webai_accounts AS candidate
			WHERE candidate.platform = canonical.platform AND candidate.email = canonical.email
		), 0))
		WHERE canonical.email != '' AND NOT EXISTS (
			SELECT 1 FROM webai_accounts AS newer
			WHERE newer.platform = canonical.platform
			  AND newer.email = canonical.email
			  AND (newer.updated_at > canonical.updated_at
			       OR (newer.updated_at = canonical.updated_at AND newer.id > canonical.id))
		)
	`); err != nil {
		return fmt.Errorf("webai: merge duplicate account credentials: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM webai_accounts
		WHERE email != '' AND EXISTS (
			SELECT 1 FROM webai_accounts AS newer
			WHERE newer.platform = webai_accounts.platform
			  AND newer.email = webai_accounts.email
			  AND (newer.updated_at > webai_accounts.updated_at
			       OR (newer.updated_at = webai_accounts.updated_at AND newer.id > webai_accounts.id))
		)
	`); err != nil {
		return fmt.Errorf("webai: remove duplicate account identities: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_webai_accounts_platform ON webai_accounts(platform);`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_webai_accounts_platform_email ON webai_accounts(platform, email) WHERE email != '';`); err != nil {
		return err
	}
	// Older account tests labeled blocked HTTP 403 probes as unknown, even when
	// the browser had already saved a usable cookie. Keep those credentials
	// connected; only HTTP 401 should mark a saved session expired.
	if _, err := tx.Exec(`
		UPDATE webai_accounts SET status = 'connected'
		WHERE status = 'unknown' AND is_active = 1
		  AND (trim(session_key) != '' OR trim(cookie) != '')
	`); err != nil {
		return fmt.Errorf("webai: restore saved sessions blocked by provider probes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("webai: commit account migration: %w", err)
	}
	return nil
}

func accountTableHasColumn(tx *sql.Tx, target string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(webai_accounts)")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == target {
			return true, nil
		}
	}
	return false, rows.Err()
}

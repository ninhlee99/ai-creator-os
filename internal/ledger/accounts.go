package ledger

import (
	"database/sql"
	"fmt"
	"strings"
)

func (l *Ledger) AddAccount(username, nicheHint, rtmpKeyRef string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var count int64
	if err := l.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count); err != nil {
		return 0, err
	}
	id, err := l.insertLocked("accounts", map[string]any{
		"username":     username,
		"niche_hint":   nicheHint,
		"rtmp_key_ref": rtmpKeyRef,
		"rest_weekday": count % 7,
	})
	if err != nil {
		if isUniqueViolation(err) {
			var existing int64
			if qerr := l.db.QueryRow(
				"SELECT id FROM accounts WHERE username = ?", username,
			).Scan(&existing); qerr != nil {
				return 0, qerr
			}
			return existing, nil
		}
		return 0, err
	}
	return id, nil
}

// GetAccount returns one account by id (sql.ErrNoRows if missing).
func (l *Ledger) GetAccount(accountID int64) (*Account, error) {
	return scanAccount(l.db.QueryRow(
		"SELECT id, username, status, persona, niche, niche_hint, topics_json, "+
			"youtube_channel, youtube_content_types, followers, followers_source, followers_verified_at, rtmp_key_ref, "+
			"rest_weekday, gift_usd, created_at, updated_at "+
			"FROM accounts WHERE id = ?", accountID))
}

// ListAccounts lists accounts, optionally filtered by status.
func (l *Ledger) ListAccounts(statuses []string) ([]Account, error) {
	var rows *sql.Rows
	var err error
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		args := make([]any, len(statuses))
		for i, s := range statuses {
			ph[i] = "?"
			args[i] = s
		}
		rows, err = l.db.Query(
			"SELECT id, username, status, persona, niche, niche_hint, topics_json, "+
				"youtube_channel, youtube_content_types, followers, followers_source, followers_verified_at, rtmp_key_ref, "+
				"rest_weekday, gift_usd, created_at, updated_at "+
				"FROM accounts WHERE status IN ("+strings.Join(ph, ",")+") ORDER BY id",
			args...)
	} else {
		rows, err = l.db.Query(
			"SELECT id, username, status, persona, niche, niche_hint, topics_json, " +
				"youtube_channel, youtube_content_types, followers, followers_source, followers_verified_at, rtmp_key_ref, " +
				"rest_weekday, gift_usd, created_at, updated_at " +
				"FROM accounts ORDER BY id")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Username, &a.Status, &a.Persona, &a.Niche,
			&a.NicheHint, &a.TopicsJSON, &a.YoutubeChannel, &a.YoutubeContentTypes,
			&a.Followers, &a.FollowersSource, &a.FollowersVerifiedAt,
			&a.RtmpKeyRef, &a.RestWeekday, &a.GiftUSD,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAccountStatus updates an account's status plus any extra fields.
func (l *Ledger) SetAccountStatus(accountID int64, status string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	fields["status"] = status
	sets := make([]string, 0, len(fields))
	vals := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		sets = append(sets, k+" = ?")
		vals = append(vals, normalize(v))
	}
	vals = append(vals, accountID)
	_, err := l.db.Exec(
		"UPDATE accounts SET "+strings.Join(sets, ", ")+
			", updated_at = datetime('now') WHERE id = ?", vals...)
	return err
}

// SetFollowers records a TikTok follower count verified against the account's
// TikTok profile. Legacy counts remain untrusted until a person verifies them.
func (l *Ledger) SetFollowers(accountID int64, followers int64) error {
	if followers < 0 {
		return fmt.Errorf("TikTok followers cannot be negative")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE accounts SET followers = ?, followers_source = 'tiktok_manual', "+
			"followers_verified_at = datetime('now'), updated_at = datetime('now') WHERE id = ?",
		followers, accountID)
	return err
}

// ---- characters (avatar identities) ----

// Character is one AI persona's visual identity. It mirrors
// internal/engines/avatar.Character (the web package must not import
// internal/engines); the adapter in cmd/aicos converts between them.

package ledger

import (
	"database/sql"
)

type Character struct {
	ID             int64
	Name           string
	ReferenceImage string
	Seed           int64
	IdentityLock   string
	VoicePreset    string
	Notes          string
	CreatedAt      string
}

const characterColumns = "id, name, reference_image, seed, identity_lock, voice_preset, notes, created_at"

func scanCharacter(row *sql.Row) (*Character, error) {
	var c Character
	if err := row.Scan(&c.ID, &c.Name, &c.ReferenceImage, &c.Seed,
		&c.IdentityLock, &c.VoicePreset, &c.Notes, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func scanCharacterRows(rows *sql.Rows) ([]Character, error) {
	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(&c.ID, &c.Name, &c.ReferenceImage, &c.Seed,
			&c.IdentityLock, &c.VoicePreset, &c.Notes, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCharacter stores a new avatar identity. The identity lock must
// already be computed by the caller (sha256 of image bytes + seed).
func (l *Ledger) CreateCharacter(name, referenceImage string, seed int64, identityLock, voicePreset, notes string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.insertLocked("characters", map[string]any{
		"name":            name,
		"reference_image": referenceImage,
		"seed":            seed,
		"identity_lock":   identityLock,
		"voice_preset":    voicePreset,
		"notes":           notes,
	})
}

// GetCharacter returns one character by id (sql.ErrNoRows if missing).
func (l *Ledger) GetCharacter(id int64) (*Character, error) {
	return scanCharacter(l.db.QueryRow(
		"SELECT "+characterColumns+" FROM characters WHERE id = ?", id))
}

// ListCharacters returns all characters in creation order.
func (l *Ledger) ListCharacters() ([]Character, error) {
	rows, err := l.db.Query("SELECT " + characterColumns + " FROM characters ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCharacterRows(rows)
}

// UpdateCharacter updates a character's editable fields.
func (l *Ledger) UpdateCharacter(id int64, name string, seed int64, identityLock, voicePreset, notes string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE characters SET name = ?, seed = ?, identity_lock = ?, "+
			"voice_preset = ?, notes = ? WHERE id = ?",
		name, seed, identityLock, voicePreset, notes, id)
	return err
}

// DeleteCharacter removes a character. The reference image file on disk is
// left alone (the caller may clean it up).
func (l *Ledger) DeleteCharacter(id int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

// ---- gifts (append-only, provider evidence) ----

// RecordGift records gift revenue and accumulates accounts.gift_usd.

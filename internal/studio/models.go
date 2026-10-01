package studio

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ModelPhoto is one user-uploaded model photo for an account. Ninh uploads
// these; they are the identity-lock ground truth. The system NEVER invents
// or web-searches model photos.
type ModelPhoto struct {
	ID        int64
	AccountID int64
	Path      string
	CreatedAt string
}

// ModelStore is the per-account model photo library, backed by the studio DB.
type ModelStore struct {
	db   *sql.DB
	root string // workRoot/models/<accountID>/
}

// NewModelStore creates the store (tables are created lazily).
func NewModelStore(db *sql.DB, workRoot string) (*ModelStore, error) {
	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS model_photos(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		path TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_model_photos_account ON model_photos(account_id);`); err != nil {
		return nil, fmt.Errorf("model store migrate: %w", err)
	}
	return &ModelStore{db: db, root: filepath.Join(workRoot, "models")}, nil
}

// Add copies src into the account's library and records it.
func (m *ModelStore) Add(accountID int64, src string) (ModelPhoto, error) {
	var mp ModelPhoto
	dir := filepath.Join(m.root, fmt.Sprint(accountID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return mp, err
	}
	dst := filepath.Join(dir, fmt.Sprintf("%d%s", time.Now().UnixNano(), filepath.Ext(src)))
	data, err := os.ReadFile(src)
	if err != nil {
		return mp, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return mp, err
	}
	now := time.Now().Format("2006-01-02T15:04:05")
	res, err := m.db.Exec(
		`INSERT INTO model_photos(account_id, path, created_at) VALUES(?,?,?)`,
		accountID, dst, now)
	if err != nil {
		_ = os.Remove(dst)
		return mp, err
	}
	id, _ := res.LastInsertId()
	return ModelPhoto{ID: id, AccountID: accountID, Path: dst, CreatedAt: now}, nil
}

// List returns the account's model photos, oldest first.
func (m *ModelStore) List(accountID int64) ([]ModelPhoto, error) {
	rows, err := m.db.Query(
		`SELECT id, account_id, path, created_at FROM model_photos WHERE account_id = ? ORDER BY id`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelPhoto
	for rows.Next() {
		var mp ModelPhoto
		if err := rows.Scan(&mp.ID, &mp.AccountID, &mp.Path, &mp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, mp)
	}
	return out, nil
}

// Delete removes a photo (file + row).
func (m *ModelStore) Delete(id int64) error {
	var path string
	if err := m.db.QueryRow(`SELECT path FROM model_photos WHERE id = ?`, id).Scan(&path); err != nil {
		return err
	}
	if _, err := m.db.Exec(`DELETE FROM model_photos WHERE id = ?`, id); err != nil {
		return err
	}
	_ = os.Remove(path)
	return nil
}

// HasPhotos reports whether the account has at least one model photo.
func (m *ModelStore) HasPhotos(accountID int64) (bool, error) {
	var n int
	if err := m.db.QueryRow(
		`SELECT COUNT(*) FROM model_photos WHERE account_id = ?`, accountID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

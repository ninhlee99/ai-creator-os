package studio

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// resolvePath repairs legacy relative model paths created when the app ran
// from repository root. The launch agent runs from data/; current writes use
// absolute paths, while older rows start with data/output/studio/models/.
func (m *ModelStore) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
			return abs
		}
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	const legacyPrefix = "data/output/studio/models/"
	if strings.HasPrefix(normalized, legacyPrefix) {
		relative := strings.TrimPrefix(normalized, legacyPrefix)
		candidate := filepath.Join(m.root, filepath.FromSlash(relative))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
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
		mp.Path = m.resolvePath(mp.Path)
		out = append(out, mp)
	}
	return out, nil
}

// ListAll returns all model photos across all accounts, newest first.
func (m *ModelStore) ListAll() ([]ModelPhoto, error) {
	rows, err := m.db.Query(
		`SELECT id, account_id, path, created_at FROM model_photos ORDER BY id DESC`)
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
		mp.Path = m.resolvePath(mp.Path)
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
	path = m.resolvePath(path)
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

// queryAccountModelPhotos retrieves only model photos owned by the selected account.
func (s *Studio) queryAccountModelPhotos(accountID int64) []string {
	if s.db == nil || accountID <= 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT path FROM model_photos WHERE account_id = ? ORDER BY id`, accountID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			p = s.resolveModelPhotoPath(p)
			if fileExists(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// ModelPhotosForAccount returns only identity references owned by that channel.
func (s *Studio) ModelPhotosForAccount(accountID int64) []string {
	return s.queryAccountModelPhotos(accountID)
}

// uploadedModelPhotos reads only user-uploaded library images. Automation that
// promises to use a user's model must not fall back to bundled sample assets.
func (s *Studio) uploadedModelPhotos(accountID int64) []string {
	if s.db == nil || accountID <= 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT path FROM model_photos WHERE account_id = ? ORDER BY id LIMIT 8`, accountID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var photos []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			path = s.resolveModelPhotoPath(path)
			if fileExists(path) {
				photos = append(photos, path)
			}
		}
	}
	return photos
}

func (s *Studio) resolveModelPhotoPath(path string) string {
	if s == nil || strings.TrimSpace(path) == "" {
		return ""
	}
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
			return abs
		}
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	const legacyOutputPrefix = "data/output/"
	if strings.HasPrefix(normalized, legacyOutputPrefix) {
		relative := strings.TrimPrefix(normalized, legacyOutputPrefix)
		candidate := filepath.Join(s.outDir, filepath.FromSlash(relative))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	const legacyDataPrefix = "data/"
	if strings.HasPrefix(normalized, legacyDataPrefix) {
		relative := strings.TrimPrefix(normalized, legacyDataPrefix)
		candidate := filepath.Join(filepath.Dir(s.outDir), filepath.FromSlash(relative))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return path
}

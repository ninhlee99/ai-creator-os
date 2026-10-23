package products

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const BioTryOnResultsDir = "bio-tryon-results"

type expiredBioTryOnResult struct {
	id   string
	path string
}

// CleanupExpiredBioTryOnResults deletes expired uploads/results from the
// private directory, then removes expired job rows and their identity hashes.
// It never follows or removes paths outside that directory.
func (s *Store) CleanupExpiredBioTryOnResults(resultDir string, now time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("bio try-on store is unavailable")
	}
	if strings.TrimSpace(resultDir) == "" {
		return 0, fmt.Errorf("bio try-on result directory is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	cutoff := formatBioTryOnTime(now)
	root, err := filepath.Abs(resultDir)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return 0, fmt.Errorf("create bio try-on result directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return 0, fmt.Errorf("bio try-on result path is not a private directory")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return 0, fmt.Errorf("secure bio try-on result directory: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	cutoffDay := bioLocalDay(now.AddDate(0, 0, -400))
	if _, err := tx.Exec(`DELETE FROM bio_tryon_daily_usage WHERE local_day<?`, cutoffDay); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM bio_tryon_account_daily_usage WHERE local_day<?`, cutoffDay); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE bio_tryon_jobs SET status='expired',updated_at=?
		WHERE expires_at<=? AND status IN ('queued','running','succeeded')`, cutoff, cutoff); err != nil {
		return 0, err
	}
	rows, err := tx.Query(`SELECT id,result_path FROM bio_tryon_jobs
		WHERE expires_at<=? ORDER BY expires_at LIMIT 100`, cutoff)
	if err != nil {
		return 0, err
	}
	var expired []expiredBioTryOnResult
	for rows.Next() {
		var item expiredBioTryOnResult
		if err := rows.Scan(&item.id, &item.path); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	cleaned := 0
	var firstErr error
	for _, item := range expired {
		if item.path != "" {
			if err := removeExpiredBioTryOnResult(root, item.id, item.path); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		if err := removeExpiredBioTryOnInput(root, item.id); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result, err := s.db.Exec(`DELETE FROM bio_tryon_jobs
			WHERE id=? AND result_path=? AND expires_at<=?`, item.id, item.path, cutoff)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if n, err := result.RowsAffected(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			cleaned += int(n)
		}
	}

	orphans, err := s.removeOrphanBioTryOnResults(root, now.Add(-BioTryOnProcessingTimeout-BioTryOnQueueTimeout))
	cleaned += orphans
	if err != nil && firstErr == nil {
		firstErr = err
	}
	return cleaned, firstErr
}

// removeOrphanBioTryOnResults sweeps files left by a crash between rendering
// and saving result_path. The age grace exceeds the maximum queued/running job
// lifetime; active job IDs and recent files are retained.
func (s *Store) removeOrphanBioTryOnResults(root string, olderThan time.Time) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, fmt.Errorf("read Bio try-on result directory: %w", err)
	}
	removed := 0
	var firstErr error
	for _, entry := range entries {
		name := entry.Name()
		id := ""
		switch {
		case strings.HasSuffix(name, ".render.png.raw.png"):
			id = strings.TrimSuffix(name, ".render.png.raw.png")
		case strings.HasSuffix(name, ".reference.jpg"):
			id = strings.TrimSuffix(name, ".reference.jpg")
		case strings.HasSuffix(name, ".input.jpg"):
			id = strings.TrimSuffix(name, ".input.jpg")
		case strings.HasSuffix(name, ".render.png"):
			id = strings.TrimSuffix(name, ".render.png")
		default:
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
				continue
			}
			id = strings.TrimSuffix(name, ext)
		}
		if !validTryOnValue(id, 24, 128) {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("inspect orphan Bio try-on result %q: %w", name, err)
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || !info.ModTime().Before(olderThan) {
			continue
		}
		var exists int
		err = s.db.QueryRow(`SELECT 1 FROM bio_tryon_jobs WHERE id=?`, id).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			if firstErr == nil {
				firstErr = fmt.Errorf("check orphan Bio try-on result %q: %w", name, err)
			}
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = fmt.Errorf("remove orphan Bio try-on result %q: %w", name, err)
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}

func removeExpiredBioTryOnInput(root, id string) error {
	if !validTryOnValue(id, 24, 128) {
		return fmt.Errorf("unsafe expired bio try-on input for job %q", id)
	}
	path := filepath.Join(root, id+".input.jpg")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect expired bio try-on input for job %q: %w", id, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("expired bio try-on input for job %q is not a regular file", id)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove expired bio try-on input for job %q: %w", id, err)
	}
	return nil
}

func removeExpiredBioTryOnResult(root, id, resultPath string) error {
	if !validTryOnValue(id, 24, 128) {
		return fmt.Errorf("unsafe expired bio try-on result for job %q", id)
	}
	path, err := filepath.Abs(resultPath)
	if err != nil {
		return fmt.Errorf("resolve expired bio try-on result for job %q: %w", id, err)
	}
	if filepath.Dir(path) != root {
		return fmt.Errorf("expired bio try-on result for job %q is outside result directory", id)
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, id+".") {
		return fmt.Errorf("expired bio try-on result for job %q has unexpected filename", id)
	}
	switch filepath.Ext(base) {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		return fmt.Errorf("expired bio try-on result for job %q has unsupported file type", id)
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect expired bio try-on result for job %q: %w", id, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("expired bio try-on result for job %q is not a regular file", id)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove expired bio try-on result for job %q: %w", id, err)
	}
	return nil
}

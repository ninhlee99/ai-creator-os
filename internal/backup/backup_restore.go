package backup

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var restoreStageMu sync.Mutex

func Stage(dataDir string, zr *zip.Reader) error {
	restoreStageMu.Lock()
	defer restoreStageMu.Unlock()

	if PendingRestore(dataDir) {
		return fmt.Errorf("đã có bản khôi phục chờ áp dụng; khởi động lại trước khi stage bản khác")
	}
	if err := Validate(zr); err != nil {
		return err
	}
	stage := filepath.Join(dataDir, stagingDir)
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	staged := false
	defer func() {
		if !staged {
			_ = os.RemoveAll(stage)
		}
	}()
	var totalWritten uint64
	for _, f := range zr.File {
		rel, err := validatedBackupEntryName(f.Name)
		if err != nil {
			return fmt.Errorf("entry không hợp lệ trong bản sao lưu: %q", f.Name)
		}
		dst := filepath.Join(stage, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			rc.Close()
			return err
		}
		remaining := maxBackupUncompressedBytes - totalWritten
		written, copyErr := io.Copy(out, io.LimitReader(rc, int64(remaining)+1))
		rcErr := rc.Close()
		outErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if rcErr != nil {
			return rcErr
		}
		if outErr != nil {
			return outErr
		}
		if uint64(written) > remaining || uint64(written) != f.UncompressedSize64 {
			return fmt.Errorf("entry %q có kích thước giải nén vượt giới hạn hoặc không khớp khai báo", f.Name)
		}
		totalWritten += uint64(written)
	}
	// Kiểm tra toàn vẹn mọi DB trong staging TRƯỚC khi đặt cờ —
	// bản sao lưu chứa DB hỏng thì từ chối stage, xóa staging, dữ liệu
	// hiện tại nguyên vẹn (không có RESTORE_PENDING).
	for _, f := range zr.File {
		rel := filepath.Clean(f.Name)
		if strings.Contains(rel, "..") || filepath.Dir(rel) != "." {
			continue
		}
		if !strings.HasSuffix(rel, ".db") {
			continue
		}
		if err := IntegrityCheck(filepath.Join(stage, rel)); err != nil {
			_ = os.RemoveAll(stage)
			return fmt.Errorf("%s trong bản sao lưu hỏng (%v) — không stage", rel, err)
		}
	}
	if err := rewriteStagedStudioAssetPaths(stage); err != nil {
		return fmt.Errorf("không chuẩn hóa được đường dẫn asset Studio: %w", err)
	}
	flag := filepath.Join(dataDir, RestorePending)
	if err := os.WriteFile(flag, []byte(time.Now().UTC().Format(time.RFC3339)), 0o600); err != nil {
		return err
	}
	staged = true
	return nil
}

func rewriteStagedStudioAssetPaths(stage string) error {
	dbPath := filepath.Join(stage, "studio.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath+"?cache=shared")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	type photoPath struct {
		id        int64
		accountID int64
		path      string
	}
	var paths []photoPath
	if exists, err := stagedTableExists(db, "model_photos"); err != nil {
		return err
	} else if exists {
		rows, err := db.Query(`SELECT id, account_id, path FROM model_photos`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var item photoPath
			if err := rows.Scan(&item.id, &item.accountID, &item.path); err != nil {
				rows.Close()
				return err
			}
			paths = append(paths, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}

	type jsonUpdate struct {
		table, column, id, value string
	}
	var jsonUpdates []jsonUpdate
	targetDataDir, err := filepath.Abs(filepath.Dir(stage))
	if err != nil {
		return err
	}
	for _, table := range []struct{ name, column string }{
		{"studio_jobs", "params"},
		{"studio_affiliate_plans", "payload"},
		{"studio_workflow_graphs", "graph_json"},
	} {
		exists, err := stagedTableExists(db, table.name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		rows, err := db.Query(`SELECT id, ` + table.column + ` FROM ` + table.name)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, raw string
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			rewritten, changed, err := rewriteStudioAssetJSON(raw, stage, targetDataDir)
			if err != nil {
				rows.Close()
				return fmt.Errorf("đọc %s.%s/%s: %w", table.name, table.column, id, err)
			}
			if changed {
				jsonUpdates = append(jsonUpdates, jsonUpdate{table.name, table.column, id, rewritten})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	if len(paths) == 0 && len(jsonUpdates) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range paths {
		if item.accountID <= 0 {
			continue
		}
		filename := path.Base(strings.ReplaceAll(item.path, `\`, `/`))
		if !validModelPhotoName(filename) {
			continue
		}
		asset := filepath.Join(stage, "output", "studio", "models", strconv.FormatInt(item.accountID, 10), filename)
		if info, err := os.Stat(asset); err != nil || info.IsDir() {
			continue
		}
		portable := filepath.ToSlash(filepath.Join("models", strconv.FormatInt(item.accountID, 10), filename))
		if _, err := tx.Exec(`UPDATE model_photos SET path=? WHERE id=?`, portable, item.id); err != nil {
			return err
		}
	}
	for _, item := range jsonUpdates {
		query := `UPDATE ` + item.table + ` SET ` + item.column + `=? WHERE id=?`
		if _, err := tx.Exec(query, item.value, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func stagedTableExists(db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func rewriteStudioAssetJSON(raw, stage, targetDataDir string) (string, bool, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", false, err
	}
	changed, err := rewriteStudioAssetValues(value, stage, targetDataDir)
	if err != nil || !changed {
		return raw, changed, err
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return "", false, err
	}
	return string(rewritten), true, nil
}

func rewriteStudioAssetValues(value any, stage, targetDataDir string) (bool, error) {
	switch item := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range item {
			if raw, ok := child.(string); ok {
				rewritten, pathChanged := stagedStudioAssetPath(raw, stage, targetDataDir)
				if pathChanged {
					item[key] = rewritten
					changed = true
				}
				continue
			}
			childChanged, err := rewriteStudioAssetValues(child, stage, targetDataDir)
			if err != nil {
				return false, err
			}
			changed = changed || childChanged
		}
		return changed, nil
	case []any:
		changed := false
		for i, child := range item {
			if raw, ok := child.(string); ok {
				rewritten, pathChanged := stagedStudioAssetPath(raw, stage, targetDataDir)
				if pathChanged {
					item[i] = rewritten
					changed = true
				}
				continue
			}
			childChanged, err := rewriteStudioAssetValues(child, stage, targetDataDir)
			if err != nil {
				return false, err
			}
			changed = changed || childChanged
		}
		return changed, nil
	default:
		return false, nil
	}
}

func stagedStudioAssetPath(value, stage, targetDataDir string) (string, bool) {
	if strings.Contains(value, "://") {
		return value, false
	}
	normalized := strings.ReplaceAll(value, `\`, `/`)
	windowsAbs := len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/' &&
		((normalized[0] >= 'a' && normalized[0] <= 'z') || (normalized[0] >= 'A' && normalized[0] <= 'Z'))
	pathLike := strings.HasPrefix(normalized, "/") || windowsAbs ||
		strings.HasPrefix(normalized, "data/") || strings.HasPrefix(normalized, "output/") ||
		strings.HasPrefix(normalized, "models/") || strings.HasPrefix(normalized, "uploads/")
	if !pathLike {
		return value, false
	}
	var rel string
	for _, root := range []string{"output/studio/models", "output/studio/uploads"} {
		marker := root + "/"
		if strings.HasPrefix(normalized, "models/") && root == "output/studio/models" {
			rel = root + "/" + strings.TrimPrefix(normalized, "models/")
			break
		}
		if strings.HasPrefix(normalized, "uploads/") && root == "output/studio/uploads" {
			rel = root + "/" + strings.TrimPrefix(normalized, "uploads/")
			break
		}
		if index := strings.LastIndex(normalized, marker); index >= 0 {
			rel = normalized[index:]
			break
		}
	}
	if rel == "" {
		return value, false
	}
	if _, err := validatedBackupEntryName(rel); err != nil {
		return value, false
	}
	asset := filepath.Join(stage, filepath.FromSlash(rel))
	if info, err := os.Stat(asset); err != nil || !info.Mode().IsRegular() {
		return value, false
	}
	return filepath.Join(targetDataDir, filepath.FromSlash(rel)), true
}

// PendingRestore báo có bản khôi phục đang chờ áp dụng không.
func PendingRestore(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, RestorePending))
	return err == nil
}

// ApplyStaged đổi các file trong thư mục chờ vào vị trí thật rồi xoá cờ.
// Chỉ gọi lúc app khởi động (chưa mở DB nào). Không có gì chờ thì no-op.
func ApplyStaged(dataDir string) error {
	if !PendingRestore(dataDir) {
		return nil
	}
	stage := filepath.Join(dataDir, stagingDir)
	flag := filepath.Join(dataDir, RestorePending)
	if _, err := os.Stat(stage); os.IsNotExist(err) {
		// A previous startup may have removed staging after applying every
		// file, then stopped before clearing this flag.
		if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("xóa cờ khôi phục: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("đọc thư mục staging: %w", err)
	}
	// Xoá -wal/-shm cũ trước khi đổi db mới vào: tránh WAL của bản cũ
	// bị SQLite đọc nhầm cho database vừa khôi phục.
	for _, db := range snapshotDBs {
		if _, err := os.Stat(filepath.Join(stage, db)); err == nil {
			for _, suffix := range []string{"-wal", "-shm"} {
				if err := removeIfExists(filepath.Join(dataDir, db+suffix)); err != nil {
					return fmt.Errorf("xóa trạng thái SQLite cũ %s: %w", db+suffix, err)
				}
			}
		}
	}
	err := filepath.Walk(stage, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		// Ghi đè nguyên tử: rename trong cùng filesystem.
		if err := os.Rename(path, dst); err != nil {
			return fmt.Errorf("áp dụng %s: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Empty staging means a prior startup moved every file but exited before
	// removing the pending flag. Stage writes the flag only after validation.
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("dọn thư mục staging sau khôi phục: %w", err)
	}
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("xóa cờ khôi phục: %w", err)
	}
	return nil
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

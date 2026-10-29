package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

// ManifestName là file nhận diện "đây là bản sao lưu của aicos".
const ManifestName = "aicos-backup.json"

// RestorePending là cờ file trong data dir: khôi phục đã stage xong,
// chờ restart app để áp dụng.
const RestorePending = "RESTORE_PENDING"

const stagingDir = "restore-staging"

const maxBackupArchiveEntries = 100_000

var errBackupArchiveTooLarge = errors.New("file sao lưu vượt giới hạn 512 MiB")

// snapshotDBs là các database được đưa vào ảnh chụp bằng SQLite online backup.
// webai.db deliberately excluded: it contains provider cookies and bearer
// sessions. Until encrypted credential storage and key recovery are wired,
// copying it into a plaintext ZIP would leak account access.
var snapshotDBs = []string{"ledger.db", "studio.db", "products.db", "accesstrade.db"}

// snapshotFiles là các file đơn lẻ được đưa vào ảnh chụp.
var snapshotFiles = []string{"content_jobs.json"}

// snapshotTreeDirs contains user-owned inputs referenced by studio.db and web uploads.
// Render outputs and run caches remain excluded.
var snapshotTreeDirs = []string{"output/studio/models", "output/studio/uploads", "uploads"}

// sqliteMagic là 16 byte đầu của mọi file SQLite hợp lệ.
var sqliteMagic = []byte("SQLite format 3\x00")

// Create ghi ảnh chụp của dataDir vào w.
func Create(dataDir string, w io.Writer) error {
	zw := zip.NewWriter(&archiveLimitWriter{writer: w, limit: MaxBackupArchiveBytes})
	added := 0
	manifest := fmt.Sprintf(`{"app":"aicos","created":%q}`, time.Now().UTC().Format(time.RFC3339))
	if uint64(len(manifest)) >= maxBackupUncompressedBytes {
		return fmt.Errorf("manifest vượt giới hạn giải nén 512 MiB")
	}
	maxContentBytes := maxBackupUncompressedBytes - uint64(len(manifest))
	var totalContentBytes uint64
	addFile := func(rel, full string) error {
		fi, err := os.Stat(full)
		if os.IsNotExist(err) {
			return nil // thiếu thì bỏ qua, không fail cả bản sao lưu
		}
		if err != nil {
			return fmt.Errorf("đọc thông tin %s: %w", rel, err)
		}
		if fi.IsDir() {
			return nil
		}
		if added >= maxBackupArchiveEntries-1 {
			return fmt.Errorf("bản sao lưu vượt giới hạn %d entry", maxBackupArchiveEntries)
		}
		if !fi.Mode().IsRegular() || fi.Size() < 0 {
			return fmt.Errorf("%s không phải file thường", rel)
		}
		remaining := maxContentBytes - totalContentBytes
		if uint64(fi.Size()) > remaining {
			return fmt.Errorf("bản sao lưu vượt giới hạn giải nén 512 MiB tại %s", rel)
		}
		src, err := os.Open(full)
		if err != nil {
			return fmt.Errorf("đọc %s: %w", rel, err)
		}
		fh := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
		fh.SetModTime(fi.ModTime())
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			src.Close()
			return err
		}
		written, copyErr := io.Copy(fw, io.LimitReader(src, int64(remaining)+1))
		closeErr := src.Close()
		if copyErr != nil {
			return fmt.Errorf("đọc %s: %w", rel, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("đóng %s: %w", rel, closeErr)
		}
		if uint64(written) > remaining || uint64(written) != uint64(fi.Size()) {
			return fmt.Errorf("%s thay đổi trong lúc tạo bản sao lưu", rel)
		}
		totalContentBytes += uint64(written)
		added++
		return nil
	}
	add := func(rel string) error {
		return addFile(rel, filepath.Join(dataDir, rel))
	}
	// Copy inputs first. ModelStore adds a file before its DB row and deletes
	// the DB row before the file, so this order avoids snapshotting a live row
	// whose input disappears before the asset walk.
	for _, d := range snapshotTreeDirs {
		full := filepath.Join(dataDir, d)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("đọc thư mục %s: %w", d, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s không phải thư mục", d)
		}
		if err := filepath.WalkDir(full, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current == full || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			rel, err := filepath.Rel(dataDir, current)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if _, err := validatedBackupEntryName(rel); err != nil {
				return nil // ignore unrelated files in these input directories
			}
			return add(rel)
		}); err != nil {
			return fmt.Errorf("đọc asset trong %s: %w", d, err)
		}
	}
	for _, db := range snapshotDBs {
		source := filepath.Join(dataDir, db)
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("đọc thông tin %s: %w", db, err)
		}
		snapshot, err := snapshotSQLiteDatabase(source)
		if err != nil {
			return fmt.Errorf("tạo snapshot %s: %w", db, err)
		}
		if db == "ledger.db" {
			if err := redactLedgerSecrets(snapshot); err != nil {
				_ = os.Remove(snapshot)
				return fmt.Errorf("loại secret khỏi ledger backup: %w", err)
			}
		}
		if err := addFile(db, snapshot); err != nil {
			_ = os.Remove(snapshot)
			return err
		}
		if err := os.Remove(snapshot); err != nil {
			return fmt.Errorf("dọn snapshot %s: %w", db, err)
		}
	}
	for _, f := range snapshotFiles {
		if err := add(f); err != nil {
			return err
		}
	}
	fw, err := zw.Create(ManifestName)
	if err != nil {
		return err
	}
	if _, err := fw.Write([]byte(manifest)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if added == 0 {
		return fmt.Errorf("thư mục dữ liệu trống — không có gì để sao lưu")
	}
	return nil
}

type archiveLimitWriter struct {
	writer  io.Writer
	limit   uint64
	written uint64
}

func (w *archiveLimitWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if uint64(len(p)) <= remaining {
		n, err := w.writer.Write(p)
		w.written += uint64(n)
		if err == nil && n != len(p) {
			err = io.ErrShortWrite
		}
		return n, err
	}
	if remaining == 0 {
		return 0, errBackupArchiveTooLarge
	}
	allowed := int(remaining)
	n, err := w.writer.Write(p[:allowed])
	w.written += uint64(n)
	if err != nil {
		return n, err
	}
	if n != allowed {
		return n, io.ErrShortWrite
	}
	return n, errBackupArchiveTooLarge
}

// snapshotSQLiteDatabase uses SQLite's online backup API so a running writer
// cannot leave the archive with DB pages from one moment and WAL pages from
// another. Returned file is a self-contained database without sidecar files.
func snapshotSQLiteDatabase(sourcePath string) (_ string, retErr error) {
	tmp, err := os.CreateTemp("", "aicos-db-snapshot-*.db")
	if err != nil {
		return "", fmt.Errorf("tạo file tạm: %w", err)
	}
	dstPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("đóng file tạm: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(dstPath)
		}
	}()

	db, err := sql.Open("sqlite", sourcePath+"?cache=shared")
	if err != nil {
		return "", fmt.Errorf("mở DB nguồn: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		return "", fmt.Errorf("kết nối DB nguồn: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "PRAGMA busy_timeout=5000"); err != nil {
		return "", fmt.Errorf("đặt busy_timeout: %w", err)
	}
	err = conn.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("SQLite driver không hỗ trợ online backup")
		}
		b, err := backuper.NewBackup(dstPath)
		if err != nil {
			return err
		}
		for {
			more, stepErr := b.Step(-1)
			if stepErr != nil {
				return errors.Join(stepErr, b.Finish())
			}
			if !more {
				return b.Finish()
			}
		}
	})
	if err != nil {
		return "", fmt.Errorf("online backup: %w", err)
	}
	return dstPath, nil
}

// redactLedgerSecrets keeps operational settings in portable backups while
// removing credentials that would otherwise make a plain ZIP a secret dump.
func redactLedgerSecrets(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='settings'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	if _, err := db.Exec(`PRAGMA secure_delete=ON`); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return err
	}
	type setting struct{ key, value string }
	var settings []setting
	for rows.Next() {
		var item setting
		if err := rows.Scan(&item.key, &item.value); err != nil {
			rows.Close()
			return err
		}
		settings = append(settings, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range settings {
		if sensitiveSettingName(item.key) {
			if _, err := tx.Exec(`DELETE FROM settings WHERE key=?`, item.key); err != nil {
				return err
			}
			continue
		}
		var value any
		if json.Unmarshal([]byte(item.value), &value) == nil {
			encoded, err := json.Marshal(redactJSONCredentials(value))
			if err != nil {
				return err
			}
			if string(encoded) != item.value {
				if _, err := tx.Exec(`UPDATE settings SET value=? WHERE key=?`, string(encoded), item.key); err != nil {
					return err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Rebuild pages so deleted credential bytes cannot survive in freelist.
	_, err = db.Exec(`VACUUM`)
	return err
}

func sensitiveSettingName(key string) bool {
	key = strings.ToLower(key)
	if strings.HasPrefix(key, "env:") {
		envName := strings.TrimPrefix(key, "env:")
		if envName == "auth" || strings.HasPrefix(envName, "auth_") || strings.Contains(envName, "_auth") {
			return true
		}
		for _, part := range []string{"_key", "apikey", "token", "secret", "password", "credential", "cookie", "session", "webhook", "telegram", "authorization", "bearer", "privatekey", "private_key", "signing_key", "jwt"} {
			if strings.Contains(envName, part) {
				return true
			}
		}
		return false
	}
	if key == "auth" || strings.HasPrefix(key, "auth.") || strings.HasSuffix(key, ".auth") {
		return true
	}
	for _, part := range []string{"access_key", "api_key", "apikey", "password", "credential", "secret", "token", "cookie", "session", "webhook", "authorization", "bearer", "privatekey", "private_key", "signing_key", "jwt"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "notify.telegram_bot" || key == "notify.telegram_chat"
}

func redactJSONCredentials(value any) any {
	switch item := value.(type) {
	case map[string]any:
		for key, nested := range item {
			if sensitiveCredentialField(key) {
				delete(item, key)
			} else {
				item[key] = redactJSONCredentials(nested)
			}
		}
	case []any:
		for i := range item {
			item[i] = redactJSONCredentials(item[i])
		}
	case string:
		// Some settings embed JSON documents as strings inside their JSON value.
		// Decode only object/array payloads so ordinary text stays byte-for-byte.
		var nested any
		if json.Unmarshal([]byte(item), &nested) == nil {
			switch nested.(type) {
			case map[string]any, []any:
				encoded, err := json.Marshal(redactJSONCredentials(nested))
				if err == nil {
					return string(encoded)
				}
			}
		}
	}
	return value
}

func sensitiveCredentialField(key string) bool {
	name := strings.ToLower(strings.TrimSpace(key))
	if name == "auth" || name == "authorization" || name == "bearer" || name == "jwt" {
		return true
	}
	if strings.HasPrefix(name, "auth_") || strings.Contains(name, "_auth") || strings.HasSuffix(name, "-auth") || strings.Contains(name, "-auth-") {
		return true
	}
	for _, part := range []string{"api_key", "api-key", "apikey", "access_key", "access-key", "token", "secret", "cookie", "password", "credential", "session", "private_key", "privatekey", "signing_key", "authorization", "bearer", "jwt"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

// IntegrityCheck chạy PRAGMA integrity_check trên một file DB và báo lỗi
// nếu DB hỏng (Đợt V). Khác với kiểm tra magic header: phát hiện được DB
// đã corrupt nhưng header còn nguyên — trường hợp Validate() bỏ lọt.
func IntegrityCheck(dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("không thấy file DB: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?cache=shared")
	if err != nil {
		return fmt.Errorf("mở DB: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return fmt.Errorf("đọc integrity_check: %w", err)
		}
		if msg != "ok" {
			return fmt.Errorf("DB hỏng: %s", msg)
		}
	}
	return rows.Err()
}

// CorruptSourceError đánh dấu lỗi CreateValidated do DB nguồn đã hỏng
// (khác với lỗi tạo zip chung chung) — để BackupTick alert critical đúng
// mức: "DB hỏng, khôi phục từ bản cũ sớm".
type CorruptSourceError struct {
	DB  string
	Err error
}

func (e *CorruptSourceError) Error() string {
	return fmt.Sprintf("%s hỏng (%v)", e.DB, e.Err)
}

func (e *CorruptSourceError) Unwrap() error { return e.Err }

// CheckSourceDBs chạy integrity_check cho mọi DB được đóng gói TRƯỚC khi zip.
// File chưa tồn tại thì bỏ qua (Create cũng bỏ qua) — chỉ DB có mặt mà
// hỏng mới báo lỗi.
func CheckSourceDBs(dataDir string) error {
	for _, db := range snapshotDBs {
		dbPath := filepath.Join(dataDir, db)
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		if err := IntegrityCheck(dbPath); err != nil {
			return &CorruptSourceError{DB: db, Err: err}
		}
	}
	return nil
}

// CreateValidatedToFile streams the backup archive directly to disk, validates
// the zip and its embedded SQLite databases via ValidateZipFile, and atomically
// renames it to destPath without buffering 512 MiB in RAM.
func CreateValidatedToFile(dataDir, destPath string) error {
	if err := CheckSourceDBs(dataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".aicos-backup-*.zip.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keepTmp := false
	defer func() {
		if !keepTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	createErr := Create(dataDir, tmp)
	closeErr := tmp.Close()
	if createErr != nil {
		return createErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := ValidateZipFile(tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return err
	}
	keepTmp = true
	return nil
}

// CreateValidated tạo ảnh chụp đã kiểm chứng, dùng CreateValidatedToFile trên
// đĩa tạm để không giữ đồng thời cả buffer nén lẫn buffer giải nén trong RAM.
func CreateValidated(dataDir string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "aicos-validated-backup-*.zip")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := CreateValidatedToFile(dataDir, tmpPath); err != nil {
		return nil, err
	}
	return os.ReadFile(tmpPath)
}

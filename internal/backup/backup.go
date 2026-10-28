package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
var snapshotDBs = []string{"ledger.db", "studio.db", "products.db", "accesstrade.db", "webai.db"}

// snapshotFiles là các file đơn lẻ được đưa vào ảnh chụp.
var snapshotFiles = []string{"content_jobs.json"}

// snapshotDirs contains small one-level state directories.
var snapshotDirs = []string{"tokens"}

// snapshotTreeDirs contains user-owned inputs referenced by studio.db.
// Render outputs and run caches remain excluded.
var snapshotTreeDirs = []string{"output/studio/models", "output/studio/uploads"}

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
	for _, d := range snapshotDirs {
		full := filepath.Join(dataDir, d)
		entries, err := os.ReadDir(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("đọc thư mục %s: %w", d, err)
		}
		for _, e := range entries {
			if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			if err := add(filepath.Join(d, e.Name())); err != nil {
				return err
			}
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

// CheckSourceDBs chạy integrity_check cho 5 DB nguồn TRƯỚC khi zip.
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

// CreateValidated tạo ảnh chụp vào bộ nhớ theo đúng pipeline của BackupTick
// tự động: integrity_check 5 DB nguồn TRƯỚC khi zip, kiểm tra bản zip SAU
// khi tạo. Dùng chung cho nút "Tải bản sao lưu" bấm tay và BackupTick —
// một pipeline duy nhất, không lệch.
func CreateValidated(dataDir string) ([]byte, error) {
	if err := CheckSourceDBs(dataDir); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := Create(dataDir, &buf); err != nil {
		return nil, err
	}
	if uint64(buf.Len()) > MaxBackupArchiveBytes {
		return nil, fmt.Errorf("file sao lưu vượt giới hạn 512 MiB")
	}
	raw := buf.Bytes()
	if err := ValidateZipBytes(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ValidateZipFile kiểm tra một file zip sao lưu trên đĩa: cấu trúc hợp lệ
// (Validate) + ledger.db bên trong vượt integrity_check (Đợt V). Dùng sau
// khi tạo backup để không lưu bản hỏng vào kho.

package backup

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

// ManifestName là file nhận diện "đây là bản sao lưu của aicos".
const ManifestName = "aicos-backup.json"

// RestorePending là cờ file trong data dir: khôi phục đã stage xong,
// chờ restart app để áp dụng.
const RestorePending = "RESTORE_PENDING"

const stagingDir = "restore-staging"

// snapshotDBs là các database được đưa vào ảnh chụp (kèm -wal/-shm nếu có).
var snapshotDBs = []string{"ledger.db", "studio.db", "products.db", "accesstrade.db", "webai.db"}

// snapshotFiles là các file đơn lẻ được đưa vào ảnh chụp.
var snapshotFiles = []string{"content_jobs.json"}

// snapshotDirs là các thư mục được đưa vào ảnh chụp (đệ quy).
var snapshotDirs = []string{"tokens"}

// sqliteMagic là 16 byte đầu của mọi file SQLite hợp lệ.
var sqliteMagic = []byte("SQLite format 3\x00")

// Create ghi ảnh chụp của dataDir vào w.
func Create(dataDir string, w io.Writer) error {
	// Đợt R: checkpoint WAL của từng DB TRƯỚC khi copy — gộp mọi giao dịch
	// đang treo vào file chính để bộ ba db/-wal/-shm nhất quán. Best-effort:
	// checkpoint hỏng (vd: DB đang bị khóa) thì copy như cũ, không fail cả
	// bản sao lưu vì một DB kẹt.
	for _, db := range snapshotDBs {
		checkpointDB(filepath.Join(dataDir, db))
	}
	zw := zip.NewWriter(w)
	added := 0
	add := func(rel string) error {
		full := filepath.Join(dataDir, rel)
		fi, err := os.Stat(full)
		if err != nil || fi.IsDir() {
			return nil // thiếu thì bỏ qua, không fail cả bản sao lưu
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("đọc %s: %w", rel, err)
		}
		fh := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
		fh.SetModTime(fi.ModTime())
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}
		if _, err := fw.Write(raw); err != nil {
			return err
		}
		added++
		return nil
	}
	for _, db := range snapshotDBs {
		if err := add(db); err != nil {
			return err
		}
		for _, suf := range []string{"-wal", "-shm"} {
			if err := add(db + suf); err != nil {
				return err
			}
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
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if err := add(filepath.Join(d, e.Name())); err != nil {
				return err
			}
		}
	}
	manifest := fmt.Sprintf(`{"app":"aicos","created":%q}`, time.Now().UTC().Format(time.RFC3339))
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

// checkpointDB gộp WAL vào file DB chính (PRAGMA wal_checkpoint).
// Bỏ qua im lặng khi file không tồn tại hoặc checkpoint không làm được —
// người gọi coi đây là best-effort trước khi copy file.
func checkpointDB(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	db, err := sql.Open("sqlite", path+"?cache=shared")
	if err != nil {
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, _ = db.Exec("PRAGMA busy_timeout=5000")
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
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
	raw := buf.Bytes()
	if err := ValidateZipBytes(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ValidateZipFile kiểm tra một file zip sao lưu trên đĩa: cấu trúc hợp lệ
// (Validate) + ledger.db bên trong vượt integrity_check (Đợt V). Dùng sau
// khi tạo backup để không lưu bản hỏng vào kho.

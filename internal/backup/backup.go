// Package backup tạo và khôi phục ảnh chụp (snapshot) thư mục dữ liệu
// của app (R2-W7, R2-07). Ảnh chụp là zip gồm 3 database, file job và
// thư mục token — cố ý loại trừ output/, avatars/, models/ (nặng và tái
// tạo được). Khôi phục gồm 2 bước: Stage (giải nén vào thư mục chờ +
// đặt cờ RESTORE_PENDING) rồi ApplyStaged (đổi file vào vị trí thật) —
// ApplyStaged chỉ chạy lúc app khởi động, khi chưa có handle DB nào mở,
// nên không bao giờ làm hỏng database đang chạy.
package backup

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestName là file nhận diện "đây là bản sao lưu của aicos".
const ManifestName = "aicos-backup.json"

// RestorePending là cờ file trong data dir: khôi phục đã stage xong,
// chờ restart app để áp dụng.
const RestorePending = "RESTORE_PENDING"

const stagingDir = "restore-staging"

// snapshotDBs là các database được đưa vào ảnh chụp (kèm -wal/-shm nếu có).
var snapshotDBs = []string{"ledger.db", "studio.db", "products.db"}

// snapshotFiles là các file đơn lẻ được đưa vào ảnh chụp.
var snapshotFiles = []string{"content_jobs.json"}

// snapshotDirs là các thư mục được đưa vào ảnh chụp (đệ quy).
var snapshotDirs = []string{"tokens"}

// sqliteMagic là 16 byte đầu của mọi file SQLite hợp lệ.
var sqliteMagic = []byte("SQLite format 3\x00")

// Create ghi ảnh chụp của dataDir vào w.
func Create(dataDir string, w io.Writer) error {
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

// Validate kiểm tra một bản sao lưu: phải có manifest và ledger.db phải
// là file SQLite thật. Fail-closed: zip lạ/hỏng bị từ chối trước khi đụng
// tới dữ liệu thật.
func Validate(zr *zip.Reader) error {
	var manifest, ledger bool
	for _, f := range zr.File {
		switch filepath.Clean(f.Name) {
		case ManifestName:
			manifest = true
		case "ledger.db":
			ledger = true
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("mở ledger.db trong zip: %w", err)
			}
			head := make([]byte, 16)
			n, _ := io.ReadFull(rc, head)
			rc.Close()
			if n < 16 || !bytes.Equal(head, sqliteMagic) {
				return fmt.Errorf("ledger.db trong bản sao lưu không phải file SQLite")
			}
		}
		// Chặn zip-slip: từ chối entry thoát khỏi thư mục đích.
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("entry đáng ngờ trong bản sao lưu: %s", f.Name)
		}
	}
	if !manifest {
		return fmt.Errorf("thiếu %s — không phải bản sao lưu của aicos", ManifestName)
	}
	if !ledger {
		return fmt.Errorf("thiếu ledger.db trong bản sao lưu")
	}
	return nil
}

// Stage giải nén bản sao lưu đã kiểm chứng vào thư mục chờ và đặt cờ
// RESTORE_PENDING. Dữ liệu thật chưa bị đụng tới.
func Stage(dataDir string, zr *zip.Reader) error {
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
	for _, f := range zr.File {
		rel := filepath.Clean(f.Name)
		if strings.Contains(rel, "..") {
			continue
		}
		dst := filepath.Join(stage, rel)
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
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	flag := filepath.Join(dataDir, RestorePending)
	if err := os.WriteFile(flag, []byte(time.Now().UTC().Format(time.RFC3339)), 0o600); err != nil {
		return err
	}
	return nil
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
	moved := 0
	// Xoá -wal/-shm cũ trước khi đổi db mới vào: tránh WAL của bản cũ
	// bị SQLite đọc nhầm cho database vừa khôi phục.
	for _, db := range snapshotDBs {
		if _, err := os.Stat(filepath.Join(stage, db)); err == nil {
			os.Remove(filepath.Join(dataDir, db+"-wal"))
			os.Remove(filepath.Join(dataDir, db+"-shm"))
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
		moved++
		return nil
	})
	if err != nil {
		return err
	}
	os.RemoveAll(stage)
	os.Remove(filepath.Join(dataDir, RestorePending))
	if moved == 0 {
		return fmt.Errorf("thư mục chờ khôi phục trống")
	}
	return nil
}

package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

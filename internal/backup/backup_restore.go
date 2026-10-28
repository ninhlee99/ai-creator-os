package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

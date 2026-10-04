package automation

// Đợt M2 — tự sao lưu thư mục dữ liệu mỗi ngày.
//
// Trước đây sao lưu chỉ bấm tay (Cài đặt · Hệ thống → Tải bản sao lưu).
// Máy chạy 24/7 không người trông mà mất ledger.db/studio.db là mất hết
// lịch sử quyết định. BackupTick chạy 1 ngày/lần (trong goroutine story/
// self-heal), dùng chung backup.Create, lưu vào <dataDir>/backups/,
// chỉ giữ N bản mới nhất (mặc định 7). Lỗi → alert warn, không crash tick.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/backup"
)

const (
	KeyBackupLastRun = "backup.last_run"
	KeyBackupLastOK  = "backup.last_ok"     // ISO thời điểm bản sao lưu thành công gần nhất
	KeyBackupKeepN   = "system.backup_keep" // số bản giữ lại, mặc định 7
)

const (
	backupInterval    = 24 * time.Hour
	defaultBackupKeep = 7
)

// BackupTick sao lưu data dir 1 ngày/lần. Tôn trọng kill switch + dry-run.
func (s *Service) BackupTick(ctx context.Context) []string {
	_ = ctx
	if s.killed() || s.dryRun() {
		return nil
	}
	if s.DataDir == "" || s.Settings == nil {
		return nil
	}
	if !dueSince(s.Settings, KeyBackupLastRun, backupInterval) {
		return nil
	}
	defer stampRun(s.Settings, KeyBackupLastRun)

	dir := filepath.Join(s.DataDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return []string{fmt.Sprintf("sao lưu tự động thất bại: %v", err)}
	}
	name := fmt.Sprintf("aicos-backup-%s.zip", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := writeBackup(s.DataDir, path); err != nil {
		_ = os.Remove(path) // xóa file dở
		msg := fmt.Sprintf("sao lưu tự động thất bại: %v", err)
		s.alert("backup_fail", "warn", msg, "Kiểm tra dung lượng đĩa và quyền ghi thư mục dữ liệu.")
		return []string{msg}
	}
	kept := pruneBackups(dir, atInt(s.Settings, KeyBackupKeepN, defaultBackupKeep))
	_ = s.Settings.Set(KeyBackupLastOK, time.Now().Format("2006-01-02T15:04:05"))
	var sizeStr string
	if fi, err := os.Stat(path); err == nil {
		sizeStr = " (" + formatBytes(fi.Size()) + ")"
	}
	return []string{fmt.Sprintf("sao lưu tự động: %s%s, giữ %d bản mới nhất", name, sizeStr, kept)}
}

// writeBackup tạo zip sao lưu vào path (đóng file trước khi trả về).
func writeBackup(dataDir, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	berr := backup.Create(dataDir, f)
	cerr := f.Close()
	if berr != nil {
		return berr
	}
	return cerr
}

// pruneBackups xóa bản cũ, chỉ giữ n bản mới nhất. Trả về số bản còn lại.
func pruneBackups(dir string, n int) int {
	if n < 1 {
		n = 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var zips []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "aicos-backup-") && strings.HasSuffix(name, ".zip") {
			zips = append(zips, name)
		}
	}
	sort.Strings(zips) // tên có timestamp → sort = cũ trước
	for len(zips) > n {
		_ = os.Remove(filepath.Join(dir, zips[0]))
		zips = zips[1:]
	}
	return len(zips)
}

// BackupLastOK đọc thời điểm sao lưu thành công gần nhất ("" nếu chưa có).
func BackupLastOK(st Settings) string {
	if st == nil {
		return ""
	}
	v, _ := st.Get(KeyBackupLastOK)
	return v
}

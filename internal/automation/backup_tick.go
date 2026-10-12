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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/backup"
)

const (
	KeyBackupLastRun = "backup.last_run"
	KeyBackupLastOK  = "backup.last_ok"     // ISO thời điểm bản sao lưu thành công gần nhất
	KeyBackupKeepN   = "system.backup_keep" // số bản giữ lại, mặc định 7
	// Đợt V: thư mục offsite (USB/ổ mạng/NAS) — copy bản mới nhất sang đó
	// sau mỗi lần sao lưu thành công. Trống = tắt.
	KeyBackupOffsiteDir = "backup.offsite_dir"
	// Đợt V: số lần copy offsite lỗi liên tiếp (để alert khi lỗi 2 ngày liền).
	KeyBackupOffsiteFails = "backup.offsite_fails"
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

	dir := filepath.Join(s.DataDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return []string{fmt.Sprintf("sao lưu tự động thất bại: %v", err)}
	}
	name := fmt.Sprintf("aicos-backup-%s.zip", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	// Pipeline dùng chung với nút "Tải bản sao lưu" bấm tay:
	// integrity_check 5 DB nguồn TRƯỚC khi zip (DB hỏng → bỏ qua, giữ bản
	// cũ + alert critical), kiểm tra bản zip SAU khi tạo.
	raw, err := backup.CreateValidated(s.DataDir)
	if err != nil {
		var cse *backup.CorruptSourceError
		if errors.As(err, &cse) {
			msg := fmt.Sprintf("sao lưu: bỏ qua — %v, giữ bản cũ", err)
			s.alert("backup_integrity", "critical", msg,
				"Database hỏng nhưng bản sao lưu cũ vẫn còn. Khôi phục từ bản cũ sớm.")
			return []string{msg}
		}
		msg := fmt.Sprintf("sao lưu tự động thất bại: %v", err)
		s.alert("backup_fail", "warn", msg, "Kiểm tra dung lượng đĩa và quyền ghi thư mục dữ liệu.")
		return []string{msg}
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		_ = os.Remove(path) // xóa file dở
		msg := fmt.Sprintf("sao lưu tự động thất bại: %v", err)
		s.alert("backup_fail", "warn", msg, "Kiểm tra dung lượng đĩa và quyền ghi thư mục dữ liệu.")
		return []string{msg}
	}
	kept := pruneBackups(dir, atInt(s.Settings, KeyBackupKeepN, defaultBackupKeep))
	_ = s.Settings.Set(KeyBackupLastOK, time.Now().Format("2006-01-02T15:04:05"))
	stampRun(s.Settings, KeyBackupLastRun)
	var sizeStr string
	if fi, err := os.Stat(path); err == nil {
		sizeStr = " (" + formatBytes(fi.Size()) + ")"
	}
	notes := []string{fmt.Sprintf("sao lưu tự động: %s%s, giữ %d bản mới nhất", name, sizeStr, kept)}
	// Đợt V: copy offsite (nếu đã cấu hình) — đĩa chính chết vẫn còn bản khác chỗ.
	if note := s.backupOffsite(path); note != "" {
		notes = append(notes, note)
	}
	return notes
}

// backupOffsite copy bản sao lưu mới nhất sang thư mục offsite (nếu cấu hình).
// Lỗi 2 lần liên tiếp → alert warn (đĩa offsite đầy/rút USB…).
func (s *Service) backupOffsite(path string) string {
	if s.Settings == nil {
		return ""
	}
	dest, _ := s.Settings.Get(KeyBackupOffsiteDir)
	if strings.TrimSpace(dest) == "" {
		return ""
	}
	fails := atInt(s.Settings, KeyBackupOffsiteFails, 0)
	if err := copyFileOffsite(path, dest); err != nil {
		fails++
		_ = s.Settings.Set(KeyBackupOffsiteFails, strconv.Itoa(fails))
		if fails == 2 {
			s.alert("backup_offsite", "warn",
				fmt.Sprintf("copy bản sao lưu sang %s lỗi 2 lần liên tiếp: %v", dest, err),
				"Kiểm tra ổ offsite (đầy? bị rút? mất kết nối mạng?).")
		}
		return fmt.Sprintf("offsite: copy thất bại (%v)", err)
	}
	if fails > 0 {
		_ = s.Settings.Set(KeyBackupOffsiteFails, "0")
	}
	return fmt.Sprintf("offsite: đã copy sang %s", dest)
}

// copyFileOffsite copy file zip sang thư mục đích (giữ nguyên tên file).
// Nguyên tử: copy vào file .tmp rồi rename — copy lỗi giữa chừng thì bản
// cũ còn nguyên, không bao giờ mất cả hai.
func copyFileOffsite(src, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(destDir, filepath.Base(src))
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, in)
	if cerr2 := out.Close(); cerr == nil {
		cerr = cerr2
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return cerr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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

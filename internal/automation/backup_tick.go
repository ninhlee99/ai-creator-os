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

	// Đợt V: kiểm tra toàn vẹn 5 DB TRƯỚC khi zip — DB đã corrupt thì không
	// tạo bản sao lưu hỏng đè lên bản tốt (giữ bản cũ dùng được).
	// File chưa tồn tại thì bỏ qua (backup.Create cũng bỏ qua).
	for _, db := range []string{"ledger.db", "studio.db", "products.db", "accesstrade.db", "reup.db"} {
		dbPath := filepath.Join(s.DataDir, db)
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		if err := backup.IntegrityCheck(dbPath); err != nil {
			msg := fmt.Sprintf("sao lưu: bỏ qua — %s hỏng (%v), giữ bản cũ", db, err)
			s.alert("backup_integrity", "critical", msg,
				"Database hỏng nhưng bản sao lưu cũ vẫn còn. Khôi phục từ bản cũ sớm.")
			return []string{msg}
		}
	}

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
	// Đợt V: kiểm tra lại bản vừa tạo — zip dở (vd: đĩa đầy giữa chừng) thì
	// xóa ngay, không để bản hỏng trong kho.
	if err := backup.ValidateZipFile(path); err != nil {
		_ = os.Remove(path)
		msg := fmt.Sprintf("sao lưu: bản vừa tạo không đạt kiểm tra (%v) — đã xóa, giữ bản cũ", err)
		s.alert("backup_integrity", "critical", msg,
			"Bản sao lưu hỏng đã bị xóa để không đè bản tốt. Kiểm tra đĩa.")
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
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, in)
	if err := out.Close(); err != nil {
		return err
	}
	return cerr
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

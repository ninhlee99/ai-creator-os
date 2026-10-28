package web

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
)

// Sao lưu & khôi phục thư mục dữ liệu (R2-W7, R2-07), ở Cài đặt · Hệ thống.
// Archive được tạo/kiểm chứng trên file tạm rồi stream về client.
// Khôi phục = upload zip → kiểm chứng → stage → yêu cầu restart app để
// áp dụng (an toàn vì file chỉ được đổi khi chưa có handle DB nào mở).

// handleBackupCreate tạo ảnh chụp data dir ra đĩa tạm và stream về trình duyệt
// để tránh cấp phát 512 MiB trong RAM.
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tmp, err := os.CreateTemp("", "aicos-web-backup-*.zip")
	if err != nil {
		s.fail(w, err, "tạo file tạm sao lưu")
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if err := backup.CreateValidatedToFile(s.DataDir, tmpPath); err != nil {
		s.fail(w, err, "tạo bản sao lưu")
		return
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		s.fail(w, err, "mở bản sao lưu")
		return
	}
	defer f.Close()
	name := fmt.Sprintf("aicos-backup-%s.zip", time.Now().Format("20060102-150405"))
	_ = s.Ledger.Decide("human", "backup_create", nil,
		"Đã tạo bản sao lưu "+name+".", map[string]any{"file": name})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("web: stream backup download: %v", err)
	}
}

// handleBackupRestore nhận file zip qua file tạm trên đĩa, kiểm chứng rồi stage
// để restart áp dụng mà không đọc toàn bộ 512 MiB vào RAM.
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	failClosed := func(reason string) {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape(
			"Khôi phục thất bại: "+reason+" — dữ liệu hiện tại không bị đụng tới."))
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(backup.MaxBackupArchiveBytes)+(1<<20))
	parseErr := r.ParseMultipartForm(16 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		failClosed("không đọc được file upload")
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		failClosed("chưa chọn file sao lưu")
		return
	}
	defer f.Close()
	if header.Size < 0 || uint64(header.Size) > backup.MaxBackupArchiveBytes {
		failClosed("file sao lưu vượt giới hạn 512 MiB")
		return
	}
	zr, err := zip.NewReader(f, header.Size)
	if err != nil {
		failClosed("file không phải zip hợp lệ")
		return
	}
	if err := backup.Stage(s.DataDir, zr); err != nil {
		failClosed(err.Error())
		return
	}
	_ = s.Ledger.Decide("human", "backup_restore", nil,
		"Đã nhận bản khôi phục — chờ restart app để áp dụng.", nil)
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(
		"Đã nhận bản khôi phục. Khởi động lại app để áp dụng."))
}

// handleBackupOffsite lưu thư mục offsite cho bản sao lưu tự động (Đợt V).
// Sau mỗi lần sao lưu thành công, BackupTick copy bản mới nhất sang đây.
func (s *Server) handleBackupOffsite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse offsite form")
		return
	}
	dir := strings.TrimSpace(r.PostFormValue("offsite_dir"))
	_ = s.Ledger.SetSetting(automation.KeyBackupOffsiteDir, dir)
	if dir != "" {
		// Reset đếm lỗi để lần copy tới đánh giá lại từ đầu.
		_ = s.Ledger.SetSetting(automation.KeyBackupOffsiteFails, "0")
	}
	_ = s.Ledger.Decide("human", "backup_offsite", nil,
		"Đã lưu thư mục offsite cho sao lưu: "+dir, map[string]any{"dir": dir})
	msg := "Đã lưu thư mục offsite."
	if dir == "" {
		msg = "Đã tắt copy offsite."
	}
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}

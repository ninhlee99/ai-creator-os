package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
)

// Sao lưu & khôi phục thư mục dữ liệu (R2-W7, R2-07), ở Cài đặt · Hệ
// thống. Sao lưu = zip 3 database + file job + token, tải về máy.
// Khôi phục = upload zip → kiểm chứng → stage → yêu cầu restart app để
// áp dụng (an toàn vì file chỉ được đổi khi chưa có handle DB nào mở).

// handleBackupCreate tạo ảnh chụp data dir và trả về file zip để tải.
// Dùng chung pipeline với BackupTick tự động: integrity_check DB nguồn
// trước, kiểm tra bản zip sau — không bao giờ trả về bản hỏng.
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := backup.CreateValidated(s.DataDir)
	if err != nil {
		s.fail(w, err, "tạo bản sao lưu")
		return
	}
	name := fmt.Sprintf("aicos-backup-%s.zip", time.Now().Format("20060102-150405"))
	_ = s.Ledger.Decide("human", "backup_create", nil,
		"Đã tạo bản sao lưu "+name+".", map[string]any{"file": name})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write(raw)
}

// handleBackupRestore nhận file zip, kiểm chứng rồi stage để restart áp dụng.
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
	parseErr := r.ParseMultipartForm(64 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		failClosed("không đọc được file upload")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		failClosed("chưa chọn file sao lưu")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, int64(backup.MaxBackupArchiveBytes)+1))
	if err != nil {
		failClosed("không đọc được file sao lưu")
		return
	}
	if uint64(len(raw)) > backup.MaxBackupArchiveBytes {
		failClosed("file sao lưu vượt giới hạn 512 MiB")
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
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

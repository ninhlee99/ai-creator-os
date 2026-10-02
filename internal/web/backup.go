package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/backup"
)

// Sao lưu & khôi phục thư mục dữ liệu (R2-W7, R2-07), ở Cài đặt · Hệ
// thống. Sao lưu = zip 3 database + file job + token, tải về máy.
// Khôi phục = upload zip → kiểm chứng → stage → yêu cầu restart app để
// áp dụng (an toàn vì file chỉ được đổi khi chưa có handle DB nào mở).

// handleBackupCreate tạo ảnh chụp data dir và trả về file zip để tải.
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	if err := backup.Create(s.DataDir, &buf); err != nil {
		s.fail(w, err, "tạo bản sao lưu")
		return
	}
	name := fmt.Sprintf("aicos-backup-%s.zip", time.Now().Format("20060102-150405"))
	_ = s.Ledger.Decide("human", "backup_create", nil,
		"Đã tạo bản sao lưu "+name+".", map[string]any{"file": name})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write(buf.Bytes())
}

// handleBackupRestore nhận file zip, kiểm chứng rồi stage để restart áp dụng.
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	failClosed := func(reason string) {
		seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape(
			"Khôi phục thất bại: "+reason+" — dữ liệu hiện tại không bị đụng tới."))
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		failClosed("không đọc được file upload")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		failClosed("chưa chọn file sao lưu")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 512<<20))
	if err != nil {
		failClosed("không đọc được file sao lưu")
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
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(
		"Đã nhận bản khôi phục. Khởi động lại app để áp dụng."))
}

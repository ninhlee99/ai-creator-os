package web

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

// --------------------------------------------------------------- publishers

// capcutExportRoot là nơi xuất gói chỉnh sửa (~/Movies/AI Creator OS).
// Biến để test trỏ về thư mục tạm.
var capcutExportRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Movies", "AI Creator OS")
}

// openOnMac chạy lệnh `open` của macOS (seam cho test).
var openOnMac = func(args ...string) error { return exec.Command("open", args...).Run() }

// handleStudioJobCapCut xuất gói chỉnh sửa, mở Finder tới thư mục và bật
// CapCut nếu máy có (CapCut không có API — người dùng kéo thả file vào).
func (s *Server) handleStudioJobCapCut(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	dir, err := s.Studio.ExportForEditing(r.PathValue("id"), capcutExportRoot())
	if err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Xuất cho CapCut lỗi: "+err.Error()))
		return
	}
	_ = openOnMac("-R", dir)
	msg := "Đã xuất vào " + dir + " — kéo các file vào timeline CapCut."
	if openOnMac("-a", "CapCut") != nil {
		msg += " (Chưa thấy app CapCut — cài từ capcut.com rồi mở thư mục này.)"
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape(msg))
}

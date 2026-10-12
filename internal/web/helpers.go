package web

// helpers.go — tiny cross-domain HTTP helpers shared by every page
// handler: the uniform 303 redirect and the uniform error responder.
// Kept in one place so redirect/error behavior stays identical everywhere.

import (
	"log"
	"net/http"
	"time"
)

func seeOther(w http.ResponseWriter, r *http.Request, loc string) {
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

func (s *Server) fail(w http.ResponseWriter, err error, what string) {
	log.Printf("web: %s: %v", what, err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// serveMediaFile phát file lớn (video/ảnh) mà không bị WriteTimeout 60s của
// http.Server cắt ngang (Đợt AK): mở app từ xa qua mạng chậm, video vài trăm
// MB cần hơn 60s để tải — trước đây trình phát dừng giữa chừng. Deadline đọc
// vẫn giữ nguyên, chỉ nới deadline ghi cho riêng request này.
func serveMediaFile(w http.ResponseWriter, r *http.Request, path string) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(mediaWriteTimeout))
	http.ServeFile(w, r, path)
}

// mediaWriteTimeout đủ cho ~1GB ở ~2Mbps; vẫn có trần để kết nối treo không
// giữ goroutine mãi mãi.
const mediaWriteTimeout = 2 * time.Hour

// clampInt giới hạn v trong [lo, hi]. Dùng cho validate biên server-side.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

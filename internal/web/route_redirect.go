package web

import (
	"net/http"
)

func redirectTo(loc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seeOther(w, r, loc)
	}
}

// ------------------------------------------------------------------ static

// setAssetCache: URL có ?v=<hash> khớp bản hiện tại → cache lâu (nội dung
// không bao giờ đổi dưới URL đó); không có/không khớp → bắt trình duyệt hỏi
// lại mỗi lần, tránh giao diện cũ sau khi cập nhật app (Đợt AK).

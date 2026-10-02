//go:build parked

package web

// settings_film.go — nút kiểm tra quay video Veo (PIVOT 2026-10-02: đã park
// cùng pipeline phim). Chỉ biên dịch với -tags parked.

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// handleProbeVideo quay thử đúng 1 clip 8s bằng Veo — TỐN TIỀN THẬT nên UI
// hiện modal cảnh báo trước (data-confirm), và job chạy nền để không treo
// request (Veo poll vài phút).
func (s *Server) handleProbeVideo(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		_ = s.Studio.ProbeVideoGen(ctx)
	}()
	seeOther(w, r, "/settings/model-local?ok="+url.QueryEscape(
		"Đang kiểm tra quay video (~8s Veo, tốn phí thật) — quay lại sau ít phút để xem kết quả."))
}

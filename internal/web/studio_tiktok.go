package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
)

// handleAPITikTokTrends trả về dữ liệu JSON của Top 10 Video Đu Trend TikTok Việt & Drama Showbiz (Việt/Trung).
func (s *Server) handleAPITikTokTrends(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		studio.InvalidateTikTokRadarCache()
	}
	ttTrends, err := studio.FetchTikTokTrendingVideos(r.Context())
	dramasVN, dramasCN, _ := studio.FetchSplitShowbizDrama(r.Context())
	combinedDramas := append([]studio.ShowbizDrama(nil), dramasVN...)
	combinedDramas = append(combinedDramas, dramasCN...)

	s.writeJSON(w, map[string]any{
		"ok":             err == nil,
		"tiktok_trends":  ttTrends,
		"dramas_vn":      dramasVN,
		"dramas_cn":      dramasCN,
		"showbiz_dramas": combinedDramas,
	})
}

// handleAPITikTokAnalyze phân tích một URL video TikTok cụ thể do người dùng cung cấp.
func (s *Server) handleAPITikTokAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}
	urlStr := r.FormValue("url")
	if urlStr == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Vui lòng nhập link video TikTok"})
		return
	}
	video, err := studio.AnalyzeTikTokVideoURL(r.Context(), urlStr)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "video": video})
}

// handleAPIFaceSwapStatus trả về thông tin trạng thái hoạt động của Face Swap engine cục bộ.
func (s *Server) handleAPIFaceSwapStatus(w http.ResponseWriter, r *http.Request) {
	st := studio.CheckFaceSwapEngine(r.Context())
	s.writeJSON(w, map[string]any{
		"ok":     true,
		"status": st,
	})
}

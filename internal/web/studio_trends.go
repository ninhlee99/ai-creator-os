package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleStudioTrends shows viral TikTok videos and each video's sound metadata.
func (s *Server) handleStudioTrends(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	source := studio.TikTokRadarSourceLabelForTopic(topic != "")
	configuredSource := source
	sourceNote := ""
	var videos []studio.TikTokTrendVideo
	var err error
	if s.Studio == nil {
		err = fmt.Errorf("Studio chưa sẵn sàng để lưu video Radar TikTok")
	} else if topic != "" {
		videos, err = s.Studio.SearchTikTokRadarVideos(r.Context(), topic)
	} else {
		videos, err = s.Studio.CollectTikTokRadarVideos(r.Context())
	}
	if len(videos) > 0 && strings.TrimSpace(videos[0].Provider) != "" {
		usedSource := videos[0].Provider
		if usedSource == studio.TikTokApifyTrendingSource {
			usedSource = studio.TikTokApifySource
		}
		if usedSource != configuredSource {
			sourceNote = fmt.Sprintf("Nguồn chính %s gặp lỗi; Radar đang dùng nguồn dự phòng %s. Nguồn này không có số lượt xem/tym.", configuredSource, usedSource)
		}
		source = usedSource
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source":      source,
			"source_note": sourceNote,
			"topic":       topic,
			"videos":      videos,
			"rank_note":   tikTokRadarRankNote(videos),
			"error":       errText(err),
		})
		return
	}
	data := s.ctx(
		"Title", "Radar TikTok", "Path", "/studio/trends",
		"Videos", videos,
		"Topic", topic,
		"SourceConfigured", studio.TikTokRadarSourceConfigured(),
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
		"TrendSource", source, "SourceNote", sourceNote, "TrendErr", errText(err),
		"RankNote", tikTokRadarRankNote(videos),
		"ShowApifySetup", source != studio.TikTokApifySource,
	)
	if r.Header.Get("HX-Request") == "true" {
		s.renderStudioTrendResults(w, data)
		return
	}
	s.render(w, "studio_trends", data)
}

func tikTokRadarRankNote(videos []studio.TikTokTrendVideo) string {
	if len(videos) == 0 {
		return ""
	}
	knownMetrics := 0
	for _, video := range videos {
		if video.PlayCountKnown || video.DiggCountKnown {
			knownMetrics++
		}
	}
	if videos[0].Provider == studio.TikTokApifyTrendingSource && knownMetrics == 0 {
		return "Apify tìm theo 3 cụm từ xu hướng Việt Nam; nguồn chưa trả view/tym nên giữ thứ tự kết quả."
	}
	if knownMetrics == 0 {
		return "Nguồn này không cung cấp bộ đếm view/tym; thứ tự theo xếp hạng của nguồn."
	}
	if videos[0].Provider == studio.TikTokApifyTrendingSource {
		return "Apify tìm theo 3 cụm từ xu hướng Việt Nam; xếp theo lượt xem giảm dần, lượt tym dùng để phá hòa."
	}
	for _, video := range videos {
		if !video.PlayCountKnown || !video.DiggCountKnown {
			return "Nguồn thiếu view/tym ở một số video; video có số liệu xếp trước, phần còn lại giữ thứ tự nguồn."
		}
	}
	return "Xếp theo lượt xem giảm dần; lượt tym dùng để phá hòa."
}

func (s *Server) handleStudioTrendDanceVerify(w http.ResponseWriter, r *http.Request) {
	redirect := func(key, message string) {
		values := url.Values{}
		if topic := strings.TrimSpace(r.FormValue("topic")); topic != "" {
			values.Set("topic", topic)
		}
		values.Set(key, message)
		http.Redirect(w, r, "/studio/trends?"+values.Encode(), http.StatusSeeOther)
	}
	if s.Studio == nil {
		redirect("err", "Studio chưa sẵn sàng để xác nhận video nhảy.")
		return
	}
	verifiedValue := r.FormValue("verified")
	if verifiedValue != "0" && verifiedValue != "1" {
		redirect("err", "Yêu cầu xác nhận video nhảy không hợp lệ.")
		return
	}
	if verifiedValue == "1" && r.PostFormValue("confirm_dance_content") != "1" {
		redirect("err", "Mở video và xác nhận đây thực sự là video nhảy trước khi dùng âm thanh.")
		return
	}
	if err := s.Studio.SetTikTokRadarDanceVerification(r.Context(), r.PathValue("videoID"), verifiedValue == "1"); err != nil {
		redirect("err", err.Error())
		return
	}
	if verifiedValue == "1" {
		redirect("ok", "Đã xác nhận video nhảy; âm thanh có thể dùng cho affiliate tự động.")
		return
	}
	redirect("ok", "Đã bỏ xác nhận video nhảy.")
}

// handleStudioTrendsRefresh refreshes the TikTok video feed cache.
func (s *Server) handleStudioTrendsRefresh(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.FormValue("topic"))
	studio.InvalidateTikTokRadarCache()
	redirect := "/studio/trends"
	if topic != "" {
		redirect += "?" + url.Values{"topic": []string{topic}}.Encode()
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) renderStudioTrendResults(w http.ResponseWriter, data map[string]any) {
	t := s.templates["studio_trends"]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "studio-trends-results", data); err != nil {
		log.Printf("web: render TikTok Radar results: %v", err)
		http.Error(w, "Không hiển thị được kết quả Radar TikTok", http.StatusInternalServerError)
	}
}

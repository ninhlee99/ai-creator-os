package web

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleStudioTrends shows viral TikTok videos and each video's sound metadata.
func (s *Server) handleStudioTrends(w http.ResponseWriter, r *http.Request) {
	var topics []string
	if s.Studio != nil {
		var err error
		topics, err = s.Studio.ListTikTokRadarTopics(r.Context())
		if err != nil || len(topics) == 0 {
			topics = studio.DefaultTikTokTopics
		}
	} else {
		topics = studio.DefaultTikTokTopics
	}

	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	if topic == "" && len(topics) > 0 {
		topic = topics[0]
	}

	source := studio.TikTokRadarSourceLabelForTopic(topic != "")
	var videos []studio.TikTokTrendVideo
	var err error
	if s.Studio == nil {
		err = fmt.Errorf("Studio chưa sẵn sàng để lưu video Radar TikTok")
	} else if topic != "" {
		videos, err = s.Studio.GetTikTokRadarVideosForTopic(r.Context(), topic)
	} else {
		videos, err = s.Studio.CollectTikTokRadarVideos(r.Context())
	}

	if len(videos) > 0 && strings.TrimSpace(videos[0].Provider) != "" {
		source = videos[0].Provider
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source":    source,
			"topic":     topic,
			"topics":    topics,
			"videos":    videos,
			"rank_note": tikTokRadarRankNote(videos),
			"error":     errText(err),
		})
		return
	}
	coveragePending := false
	if s.Studio != nil {
		coveragePending = s.Studio.QueueTikTokRadarTopicRefresh(topic, len(videos))
	}
	data := s.ctx(
		"Title", "Radar TikTok", "Path", "/studio/trends",
		"Videos", videos,
		"CoveragePending", coveragePending,
		"Topic", topic,
		"Topics", topics,
		"SourceConfigured", studio.TikTokRadarSourceConfigured(),
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
		"TrendSource", source, "TrendErr", errText(err),
		"RankNote", tikTokRadarRankNote(videos),
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
	if knownMetrics == 0 {
		return "Tối đa 8 video xếp theo thứ tự tìm kiếm từ TikTok Việt Nam."
	}
	return "Tối đa 8 video hot nhất tại Việt Nam (xếp theo lượt xem giảm dần; lượt tym dùng để phá hòa)."
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

// handleStudioTrendsRefresh refreshes the TikTok video feed cache for the given topic.
func (s *Server) handleStudioTrendsRefresh(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.FormValue("topic"))
	studio.InvalidateTikTokRadarCache()
	if s.Studio != nil && topic != "" {
		_, _ = s.Studio.CollectTikTokRadarVideosForTopic(r.Context(), topic)
	}
	redirect := "/studio/trends"
	values := url.Values{}
	if topic != "" {
		values.Set("topic", topic)
	}
	values.Set("ok", "Đã cập nhật tối đa 8 video hot nhất cho chủ đề này.")
	http.Redirect(w, r, redirect+"?"+values.Encode(), http.StatusSeeOther)
}

// handleStudioTrendsAddTopic adds a new topic and collects up to 8 hot videos.
func (s *Server) handleStudioTrendsAddTopic(w http.ResponseWriter, r *http.Request) {
	newTopic := strings.TrimSpace(r.FormValue("new_topic"))
	if newTopic == "" {
		http.Redirect(w, r, "/studio/trends?err="+url.QueryEscape("Vui lòng nhập tên chủ đề cần tìm kiếm."), http.StatusSeeOther)
		return
	}
	if s.Studio == nil {
		http.Redirect(w, r, "/studio/trends?err="+url.QueryEscape("Studio chưa sẵn sàng."), http.StatusSeeOther)
		return
	}

	if err := s.Studio.AddTikTokRadarTopic(r.Context(), newTopic); err != nil {
		http.Redirect(w, r, "/studio/trends?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/studio/trends?topic="+url.QueryEscape(newTopic)+"&ok="+url.QueryEscape("Đã thêm chủ đề '"+newTopic+"' và lấy tối đa 8 video hot nhất."), http.StatusSeeOther)
}

// handleStudioTrendsDeleteTopic removes a topic from the list.
func (s *Server) handleStudioTrendsDeleteTopic(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.FormValue("topic"))
	if s.Studio != nil && topic != "" {
		_ = s.Studio.DeleteTikTokRadarTopic(r.Context(), topic)
	}
	http.Redirect(w, r, "/studio/trends?ok="+url.QueryEscape("Đã xóa chủ đề."), http.StatusSeeOther)
}

func (s *Server) renderStudioTrendResults(w http.ResponseWriter, data map[string]any) {
	t := s.templates["studio_trends"]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Ưu tiên render studio-trends-content để cập nhật đồng bộ cả thanh chủ đề active và danh sách video
	if err := t.ExecuteTemplate(w, "studio-trends-content", data); err != nil {
		if fbErr := t.ExecuteTemplate(w, "studio-trends-results", data); fbErr != nil {
			log.Printf("web: render TikTok Radar content: %v", err)
			http.Error(w, "Không hiển thị được kết quả Radar TikTok", http.StatusInternalServerError)
		}
	}
}

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
	coveragePending := false
	refreshErr := ""
	if s.Studio != nil {
		s.Studio.QueueTikTokRadarTopicRefresh(topic, len(videos))
		coveragePending = s.Studio.IsTikTokRadarRefreshing(topic)
		refreshErr = s.Studio.TikTokRadarTopicRefreshError(topic)
	}
	if err == nil && refreshErr != "" {
		err = fmt.Errorf("quét nền Radar: %s", refreshErr)
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source":               source,
			"topic":                topic,
			"topics":               topics,
			"videos":               videos,
			"dance_filter_enabled": studio.TikTokRadarTopicRequiresDance(topic),
			"coverage_pending":     coveragePending,
			"rank_note":            tikTokRadarRankNote(videos),
			"error":                errText(err),
		})
		return
	}
	data := s.ctx(
		"Title", "Radar TikTok", "Path", "/studio/trends",
		"Videos", videos,
		"DanceFilterEnabled", studio.TikTokRadarTopicRequiresDance(topic),
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
		data["HXTarget"] = r.Header.Get("HX-Target")
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
	knownVelocity := 0
	for _, video := range videos {
		if video.PlayCountKnown || video.DiggCountKnown {
			knownMetrics++
		}
		if video.VelocityKnown {
			knownVelocity++
		}
	}
	if knownMetrics == 0 {
		return "Tối đa 8 video đạt chủ đề đã phân tích; xếp theo thứ tự tìm kiếm khi chưa có chỉ số."
	}
	if knownVelocity > 0 {
		return "Tối đa 8 video đạt chủ đề; ưu tiên tốc độ tăng view, rồi engagement và tổng view."
	}
	return "Tối đa 8 video đạt chủ đề; xếp theo engagement rồi tổng view khi chưa đủ snapshot tăng trưởng."
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
	if s.Studio == nil || topic == "" {
		values := url.Values{"err": {"Không thể làm mới Radar khi Studio hoặc chủ đề chưa sẵn sàng."}}
		http.Redirect(w, r, "/studio/trends?"+values.Encode(), http.StatusSeeOther)
		return
	}
	if !s.Studio.QueueTikTokRadarTopicRefreshNow(topic) {
		values := url.Values{"err": {"Không thể chạy quét Radar; kiểm tra crawler TikTok."}, "topic": {topic}}
		http.Redirect(w, r, "/studio/trends?"+values.Encode(), http.StatusSeeOther)
		return
	}
	redirect := "/studio/trends"
	values := url.Values{}
	if topic != "" {
		values.Set("topic", topic)
	}
	values.Set("ok", "Đã xếp lịch phân tích và cập nhật Top 8 cho chủ đề này.")
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
	studio.InvalidateTikTokRadarCache()
	if !s.Studio.QueueTikTokRadarTopicRefreshNow(newTopic) {
		http.Redirect(w, r, "/studio/trends?topic="+url.QueryEscape(newTopic)+"&err="+url.QueryEscape("Đã lưu chủ đề nhưng crawler TikTok chưa sẵn sàng để lấy Top 8."), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/studio/trends?topic="+url.QueryEscape(newTopic)+"&ok="+url.QueryEscape("Đã thêm chủ đề '"+newTopic+"'; Radar đang lấy và phân tích Top 8."), http.StatusSeeOther)
}

// handleStudioTrendsDeleteTopic removes a topic from the list.
func (s *Server) handleStudioTrendsDeleteTopic(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimSpace(r.FormValue("topic"))
	if s.Studio == nil || topic == "" {
		http.Redirect(w, r, "/studio/trends?err="+url.QueryEscape("Studio hoặc chủ đề chưa sẵn sàng."), http.StatusSeeOther)
		return
	}
	if err := s.Studio.DeleteTikTokRadarTopic(r.Context(), topic); err != nil {
		http.Redirect(w, r, "/studio/trends?topic="+url.QueryEscape(topic)+"&err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	studio.InvalidateTikTokRadarCache()
	http.Redirect(w, r, "/studio/trends?ok="+url.QueryEscape("Đã xóa chủ đề."), http.StatusSeeOther)
}

func (s *Server) renderStudioTrendResults(w http.ResponseWriter, data map[string]any) {
	t := s.templates["studio_trends"]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if rTarget := data["HXTarget"]; rTarget == "studio-trends-results" {
		if err := t.ExecuteTemplate(w, "studio-trends-results", data); err != nil {
			log.Printf("web: render TikTok Radar results: %v", err)
			http.Error(w, "Không hiển thị được kết quả Radar TikTok", http.StatusInternalServerError)
		}
		return
	}
	// Topic selection refreshes both active chip and video results. Background polling targets results only.
	if err := t.ExecuteTemplate(w, "studio-trends-content", data); err != nil {
		if fbErr := t.ExecuteTemplate(w, "studio-trends-results", data); fbErr != nil {
			log.Printf("web: render TikTok Radar content: %v", err)
			http.Error(w, "Không hiển thị được kết quả Radar TikTok", http.StatusInternalServerError)
		}
	}
}

package web

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleStudioTikTokVideoLink(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("id"))
	redirect := func(key, value string) {
		seeOther(w, r, "/studio/jobs?tab=done&id="+url.QueryEscape(jobID)+"&"+key+"="+url.QueryEscape(value)+"#job-"+url.QueryEscape(jobID))
	}
	if s.Studio == nil {
		redirect("err", "Studio chưa sẵn sàng.")
		return
	}
	if err := r.ParseForm(); err != nil {
		redirect("err", "Không đọc được URL video TikTok.")
		return
	}
	if _, err := s.Studio.LinkTikTokPublishedVideo(jobID, r.FormValue("video_url")); err != nil {
		redirect("err", err.Error())
		return
	}
	redirect("msg", "Đã liên kết video TikTok với job Studio.")
}

package web

import (
	"encoding/json"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"net/url"
)

// handleStudioJobCancel cancels a running/queued job.
func (s *Server) handleStudioJobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio != nil {
		s.Studio.CancelRunning(id)
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobDelete removes a job and all its files.
func (s *Server) handleStudioJobDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	returnTo := "/studio/jobs"
	if r.FormValue("return_to") == "/" {
		returnTo = "/"
	}
	if s.Studio == nil {
		seeOther(w, r, returnTo)
		return
	}
	if err := s.Studio.DeleteJob(id); err != nil {
		seeOther(w, r, returnTo+"?err="+url.QueryEscape("Không xoá được job: "+err.Error()))
		return
	}
	if returnTo == "/" {
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xóa tác vụ thất bại."))
		return
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape("Đã xóa job."))
}

// handleStudioJobRetry creates a new affiliate job from an old job's params.
func (s *Server) handleStudioJobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	j, ok := s.Studio.GetJob(id)
	if !ok {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy job "+id))
		return
	}
	if j.Kind != studio.KindAffiliate {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Chỉ thử lại được job affiliate"))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không đọc được tham số job cũ"))
		return
	}
	planID, err := s.Studio.ReissueAffiliatePreparation(p)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không thể khôi phục shot plan: "+err.Error()))
		return
	}
	p.PreparedPlanID = planID
	p.FlowCreditsAcknowledged = r.FormValue("confirm_flow_credits") == "1"
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job mới: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
}

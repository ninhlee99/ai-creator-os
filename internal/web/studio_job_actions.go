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

// handleStudioJobRetry creates a new job from an old job's params (supports viral and affiliate).
func (s *Server) handleStudioJobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	j, ok := s.Studio.GetJob(id)
	if !ok {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không tìm thấy job "+id))
		return
	}

	switch j.Kind {
	case studio.KindViral:
		var p studio.ViralParams
		if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không đọc được tham số job viral cũ"))
			return
		}
		p.FlowCreditsAcknowledged = true
		newID, err := s.Studio.EnqueueViral(r.Context(), p)
		if err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không thể thử lại job viral: "+err.Error()))
			return
		}
		seeOther(w, r, "/studio/jobs?msg=created&job_id="+url.QueryEscape(newID)+"#job-"+url.QueryEscape(newID))
		return

	case studio.KindAffiliate:
		var p studio.AffiliateParams
		if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không đọc được tham số job cũ"))
			return
		}
		if err := s.ensureAffiliateRadarMusic(r.Context(), &p); err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không lấy được sound từ Radar: "+err.Error()))
			return
		}
		planID, err := s.Studio.ReissueAffiliatePreparation(p)
		if err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không thể khôi phục shot plan: "+err.Error()))
			return
		}
		p.PreparedPlanID = planID
		p.FlowCreditsAcknowledged = true
		if err := s.attachAffiliateVideoTracking(r.Context(), &p, planID); err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape(err.Error()))
			return
		}
		newID, err := s.Studio.CreateAffiliateJob(p)
		if err != nil {
			seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Không tạo được job mới: "+err.Error()))
			return
		}
		seeOther(w, r, "/studio/jobs?msg=created&job_id="+url.QueryEscape(newID)+"#job-"+url.QueryEscape(newID))
		return

	default:
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Chưa hỗ trợ thử lại loại job này: "+j.Kind))
		return
	}
}

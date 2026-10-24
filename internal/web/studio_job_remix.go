package web

import (
	"encoding/json"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// handleStudioJobRemix dựng lại 1 video affiliate với tham số đã sửa
// (thời lượng, thứ tự ảnh, chú thích). Bản cũ
// giữ nguyên — đây là "chỉnh sửa" trong app vì CapCut không có API.
func (s *Server) handleStudioJobRemix(w http.ResponseWriter, r *http.Request) {
	back := func(q string) { seeOther(w, r, "/studio/jobs?"+q) }
	if s.Studio == nil {
		back("err=" + url.QueryEscape("Studio chưa sẵn sàng."))
		return
	}
	j, ok := s.Studio.GetJob(r.PathValue("id"))
	if !ok || j.Kind != studio.KindAffiliate {
		back("err=" + url.QueryEscape("Chỉ chỉnh sửa được video affiliate đã có."))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		back("err=" + url.QueryEscape("Không đọc được tham số video cũ."))
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		back("err=" + url.QueryEscape("Không đọc được form — thử lại."))
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds"))); err == nil {
		p.Seconds = clampInt(v, 10, 120)
	}
	p = reorderPhotos(p, r.FormValue("order"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("caption"))
	p.FlowCreditsAcknowledged = r.FormValue("confirm_flow_credits") == "1"
	if err := s.ensureAffiliateRadarMusic(r.Context(), &p); err != nil {
		back("err=" + url.QueryEscape("Không lấy được sound từ Radar: "+err.Error()))
		return
	}
	planID, err := s.Studio.ReissueAffiliatePreparation(p)
	if err != nil {
		back("err=" + url.QueryEscape("Không thể khôi phục shot plan: "+err.Error()))
		return
	}
	p.PreparedPlanID = planID
	if err := s.attachAffiliateVideoTracking(r.Context(), &p, planID); err != nil {
		back("err=" + url.QueryEscape(err.Error()))
		return
	}
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		back("err=" + url.QueryEscape("Không dựng lại được: "+err.Error()))
		return
	}
	back("msg=" + url.QueryEscape("Đang dựng bản chỉnh sửa — bản cũ vẫn giữ để so sánh."))
}

// reorderPhotos đổi thứ tự ảnh sản phẩm + ảnh thêm (ảnh người mẫu giữ
// nguyên vì là ảnh khóa khuôn mặt). order: "reverse" | "rotate" | "".
func reorderPhotos(p studio.AffiliateParams, order string) studio.AffiliateParams {
	var all []string
	if p.ProductPhoto != "" {
		all = append(all, p.ProductPhoto)
	}
	all = append(all, p.ExtraPhotos...)
	if len(all) < 2 {
		return p
	}
	switch order {
	case "reverse":
		for i, k := 0, len(all)-1; i < k; i, k = i+1, k-1 {
			all[i], all[k] = all[k], all[i]
		}
	case "rotate":
		all = append(all[1:], all[0])
	default:
		return p
	}
	p.ProductPhoto, p.ExtraPhotos = all[0], append([]string(nil), all[1:]...)
	return p
}

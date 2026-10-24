package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// handleStudioPublishReview resolves an ambiguous TikTok Creator Inbox upload.
func (s *Server) handleStudioPublishReview(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	fail := func(message string) {
		seeOther(w, r, "/?err="+url.QueryEscape(message))
	}
	if s.Studio == nil {
		fail("Studio chưa sẵn sàng.")
		return
	}
	job, ok := s.Studio.GetJob(jobID)
	if !ok || job.Kind != studio.KindAffiliate {
		fail("Chỉ đối chiếu được job affiliate.")
		return
	}
	if s.Studio.PublishLegStatus(jobID, "tiktok") != studio.LegNeedsReview {
		fail("TikTok draft không còn ở trạng thái cần đối chiếu.")
		return
	}
	switch r.PostFormValue("resolution") {
	case "published":
		remoteID := strings.TrimSpace(r.PostFormValue("remote_id"))
		if remoteID == "" {
			fail("Nhập mã draft đã kiểm tra trong TikTok Creator Inbox.")
			return
		}
		if err := s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegDone, remoteID); err != nil {
			fail("Không lưu được kết quả đối chiếu: " + err.Error())
			return
		}
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xác nhận TikTok draft tồn tại."))
	case "retry":
		if err := s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegFailed, ""); err != nil {
			fail("Không thể xếp lại TikTok draft: " + err.Error())
			return
		}
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xác nhận chưa có draft; Autopilot sẽ thử lại theo lịch."))
	default:
		fail("Chọn kết quả đối chiếu hợp lệ.")
	}
}

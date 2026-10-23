package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// handleStudioPublishReview records an operator's reconciliation of an
// ambiguous Autopilot upload. Retry becomes eligible only after explicit
// confirmation that no post exists.
func (s *Server) handleStudioPublishReview(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	returnTo := "/?err="
	fail := func(message string) {
		seeOther(w, r, returnTo+url.QueryEscape(message))
	}
	if s.Studio == nil {
		fail("Studio chưa sẵn sàng.")
		return
	}
	job, ok := s.Studio.GetJob(jobID)
	if !ok || job.Kind != studio.KindAffiliate {
		fail("Chỉ xác nhận được publish leg của job affiliate.")
		return
	}
	platform := strings.ToLower(strings.TrimSpace(r.PostFormValue("platform")))
	switch platform {
	case "youtube", "tiktok", "facebook", "instagram", "threads":
	default:
		fail("Nền tảng này chưa có luồng đối chiếu tự động.")
		return
	}
	if s.Studio.PublishLegStatus(jobID, platform) != studio.LegNeedsReview {
		fail("Publish leg không còn ở trạng thái cần đối chiếu.")
		return
	}
	switch r.PostFormValue("resolution") {
	case "published":
		remoteID := strings.TrimSpace(r.PostFormValue("remote_id"))
		if remoteID == "" {
			fail("Nhập mã video/bài đã kiểm tra trên nền tảng.")
			return
		}
		if err := s.Studio.SetPublishLeg(jobID, platform, studio.LegDone, remoteID); err != nil {
			fail("Không lưu được kết quả đối chiếu: " + err.Error())
			return
		}
		if platform == "youtube" {
			s.resolveGrowthReviewForJob(jobID, true, remoteID)
		}
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xác nhận nội dung tồn tại trên nền tảng."))
	case "retry":
		if err := s.Studio.SetPublishLeg(jobID, platform, studio.LegFailed, ""); err != nil {
			fail("Không thể xếp lại publish leg: " + err.Error())
			return
		}
		if platform == "youtube" {
			s.resolveGrowthReviewForJob(jobID, false, "")
		}
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã xác nhận chưa có nội dung; Autopilot sẽ thử lại theo lịch."))
	default:
		fail("Chọn kết quả đối chiếu hợp lệ.")
	}
}

// handleGrowthPublishReview resolves an ambiguous direct Growth upload. If
// Autopilot owns the linked publish leg, its retry remains the only retry path.
func (s *Server) handleGrowthPublishReview(w http.ResponseWriter, r *http.Request) {
	back := "/growth?err="
	fail := func(message string) {
		seeOther(w, r, back+url.QueryEscape(message))
	}
	if s.Growth == nil {
		fail("Growth chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail("Mã mục Growth không hợp lệ.")
		return
	}
	item, err := s.Growth.GetItem(id)
	if err != nil || item == nil || item.Status != growth.ItemNeedsReview {
		fail("Mục Growth không còn cần đối chiếu.")
		return
	}
	legStatus := ""
	if s.Studio != nil && item.StudioJobID != "" {
		legStatus = s.Studio.PublishLegStatus(item.StudioJobID, "youtube")
	}
	switch r.PostFormValue("resolution") {
	case "published":
		remoteID := strings.TrimSpace(r.PostFormValue("remote_id"))
		if remoteID == "" {
			fail("Nhập mã video đã kiểm tra trên YouTube.")
			return
		}
		if s.Studio != nil && item.StudioJobID != "" {
			if job, ok := s.Studio.GetJob(item.StudioJobID); ok && job.Kind == studio.KindAffiliate {
				if err := s.Studio.SetPublishLeg(item.StudioJobID, "youtube", studio.LegDone, remoteID); err != nil {
					fail("Không lưu được kết quả Studio: " + err.Error())
					return
				}
			}
		}
		if err := s.Growth.SetItemPublished(item.ID, remoteID); err != nil {
			fail("Không lưu được kết quả Growth: " + err.Error())
			return
		}
		seeOther(w, r, "/growth?ok="+url.QueryEscape("Đã xác nhận video YouTube tồn tại."))
	case "retry":
		if legStatus == studio.LegNeedsReview || legStatus == studio.LegFailed {
			if s.Studio != nil {
				if err := s.Studio.SetPublishLeg(item.StudioJobID, "youtube", studio.LegFailed, ""); err != nil {
					fail("Không thể xếp lại publish leg: " + err.Error())
					return
				}
			}
			if err := s.Growth.SetItemStatus(item.ID, growth.ItemProduced,
				"Đã xác nhận chưa có video; chờ Autopilot thử lại chân YouTube theo lịch."); err != nil {
				fail("Không cập nhật được trạng thái Growth: " + err.Error())
				return
			}
			seeOther(w, r, "/growth?ok="+url.QueryEscape("Đã xác nhận chưa có video; Autopilot sẽ thử lại theo lịch."))
			return
		}
		if legStatus == studio.LegDone {
			fail("Studio đã ghi nhận chân đăng hoàn tất; đối chiếu trạng thái trong Studio trước.")
			return
		}
		if err := s.Growth.SetItemStatus(item.ID, growth.ItemProduced,
			"Đã xác nhận chưa có video YouTube; Growth sẽ thử lại ở vòng sau."); err != nil {
			fail("Không thể xếp lại mục Growth: " + err.Error())
			return
		}
		seeOther(w, r, "/growth?ok="+url.QueryEscape("Đã xác nhận chưa có video; Growth sẽ thử lại theo lịch."))
	default:
		fail("Chọn kết quả đối chiếu hợp lệ.")
	}
}

func (s *Server) resolveGrowthReviewForJob(jobID string, published bool, remoteID string) {
	if s.Growth == nil || s.Mgr == nil {
		return
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		return
	}
	for _, account := range accounts {
		items, err := s.Growth.ActivePlanItems(account.ID)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.StudioJobID != jobID || item.Status != growth.ItemNeedsReview {
				continue
			}
			if published {
				_ = s.Growth.SetItemPublished(item.ID, remoteID)
			} else {
				_ = s.Growth.SetItemStatus(item.ID, growth.ItemProduced,
					"Đã xác nhận chưa có video; chờ Autopilot thử lại chân YouTube theo lịch.")
			}
		}
	}
}

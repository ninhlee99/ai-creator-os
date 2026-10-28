package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func (s *Server) handleStudioTikTokDraft(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("id"))
	redirect := func(key, message string) {
		seeOther(w, r, "/studio/jobs?tab=done&id="+url.QueryEscape(jobID)+"&"+key+"="+url.QueryEscape(message)+"#job-"+url.QueryEscape(jobID))
	}
	fail := func(message string) { redirect("err", message) }
	if r.PostFormValue("publish_consent") != "yes" {
		fail("Xác nhận đã xem trước và đồng ý gửi video vào TikTok Draft trước khi tiếp tục.")
		return
	}
	if s.Studio == nil {
		fail("Studio chưa sẵn sàng.")
		return
	}
	job, ok := s.Studio.GetJob(jobID)
	if !ok || (job.Kind != studio.KindAffiliate && job.Kind != studio.KindViral) {
		fail("Không tìm thấy job TikTok hợp lệ.")
		return
	}
	if job.Status != studio.StatusDone || !studioOutputQCVerified(job) || studioOutputKind(job.Output) != "video" {
		fail("Chỉ gửi video Affiliate/Viral đã hoàn tất và qua QC.")
		return
	}
	accountID := studioTikTokAccountID(job)
	if accountID <= 0 || s.Mgr == nil {
		fail("Job chưa gắn tài khoản TikTok đích.")
		return
	}
	account, err := s.Mgr.Get(accountID)
	if err != nil {
		fail("Không tìm thấy tài khoản TikTok của job.")
		return
	}
	publisher := publishers.NewTikTokPublisher(account.Username)
	if !publisher.IsConfigured() || !publisher.Handles("short_video") {
		fail("Tài khoản TikTok chưa kết nối Draft API.")
		return
	}
	outputPath := s.Studio.OutputPath(job.Output)
	info, err := os.Stat(outputPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		fail("Không tìm thấy file video hợp lệ của job.")
		return
	}

	claimed, err := s.Studio.BeginPublishLegWithConsent(jobID, "tiktok",
		"Người vận hành xác nhận đã xem trước video và đồng ý gửi vào TikTok Creator Inbox dưới dạng Draft; chưa đăng công khai.")
	if err != nil {
		fail("Không lưu được trạng thái gửi và bằng chứng xác nhận; chưa tải video lên.")
		return
	}
	if !claimed {
		switch s.Studio.PublishLegStatus(jobID, "tiktok") {
		case studio.LegDone:
			redirect("msg", "TikTok Draft đã được tải lên trước đó; không gửi trùng.")
		case studio.LegPublishing:
			redirect("msg", "TikTok Draft đang được gửi; chờ kết quả.")
		case studio.LegNeedsReview:
			fail("Kết quả lần gửi trước chưa rõ; kiểm tra TikTok Creator Inbox trước khi thử lại.")
		default:
			fail("Không thể claim lượt gửi TikTok; hãy tải lại trang.")
		}
		return
	}
	title := job.Title
	if job.Kind == studio.KindAffiliate {
		var params studio.AffiliateParams
		if json.Unmarshal([]byte(job.Params), &params) == nil && strings.TrimSpace(params.ProductName) != "" {
			title = strings.TrimSpace(params.ProductName)
		}
	}
	s.Studio.AppendLog(jobID, "Đang gửi video lên TikTok Creator Inbox dưới dạng Draft.")
	result := publisher.Publish(r.Context(), outputPath, title, job.Caption, "short_video")
	if !result.Ok {
		if result.DeliveryUncertain {
			note := "TikTok chưa xác nhận kết quả gửi; kiểm tra Creator Inbox trước khi thử lại."
			_ = s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegNeedsReview, "")
			s.Studio.AppendLog(jobID, note+" Lỗi: "+result.Error)
			fail(note)
			return
		}
		_ = s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegFailed, "")
		s.Studio.AppendLog(jobID, "Tải TikTok Draft thất bại: "+result.Error)
		fail("Tải TikTok Draft thất bại: " + result.Error)
		return
	}
	if !result.Draft {
		_ = s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegNeedsReview, result.RemoteID)
		s.Studio.AppendLog(jobID, "TikTok trả kết quả không xác nhận đây là Draft; cần kiểm tra Creator Inbox.")
		fail("TikTok chưa xác nhận đây là Draft; kiểm tra Creator Inbox trước khi làm tiếp.")
		return
	}
	if err := s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegDone, result.RemoteID); err != nil {
		_ = s.Studio.SetPublishLeg(jobID, "tiktok", studio.LegNeedsReview, result.RemoteID)
		s.Studio.AppendLog(jobID, "TikTok có thể đã nhận Draft nhưng trạng thái không lưu được; cần đối chiếu Creator Inbox.")
		fail("TikTok có thể đã nhận Draft nhưng app chưa lưu được kết quả; kiểm tra Creator Inbox trước khi thử lại.")
		return
	}
	s.Studio.AppendLog(jobID, "Đã tải video lên TikTok Creator Inbox dưới dạng Draft; cần duyệt trong app TikTok.")
	redirect("msg", "Đã gửi TikTok Draft. Mở TikTok để kiểm tra và duyệt đăng.")
}

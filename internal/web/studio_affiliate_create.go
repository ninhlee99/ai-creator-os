package web

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) attachAffiliateVideoTracking(ctx context.Context, p *studio.AffiliateParams, jobID string) error {
	if p == nil || p.ProductID <= 0 {
		return nil
	}
	if s.Products == nil {
		return fmt.Errorf("kho sản phẩm chưa sẵn sàng để tạo tracking riêng cho video")
	}
	product, err := s.Products.GetByID(p.ProductID)
	if err != nil || product.ReviewStatus != products.ReviewApproved {
		return fmt.Errorf("sản phẩm không còn ở trạng thái đã duyệt")
	}
	if product.Source != accesstrade.HunterSource {
		return nil
	}
	client, err := s.atClient()
	if err != nil {
		return err
	}
	linkCtx, cancel := context.WithTimeout(ctx, atTimeout)
	defer cancel()
	link, err := client.CreateTikTokVideoTrackingLink(linkCtx, product, jobID)
	if err != nil {
		return fmt.Errorf("không tạo được link AccessTrade riêng cho video: %w", err)
	}
	p.AffLink = link.TrackingURL
	p.AffiliateAttributionID = jobID
	return nil
}

func (s *Server) ensureAffiliateRadarMusic(ctx context.Context, p *studio.AffiliateParams) error {
	if p == nil {
		return fmt.Errorf("thiếu tham số affiliate")
	}
	if strings.TrimSpace(p.SourceMusicVideoURL) != "" {
		if s.Studio == nil {
			return fmt.Errorf("Studio chưa sẵn sàng để xác minh video Radar TikTok")
		}
		source, err := s.Studio.ResolveTikTokRadarDanceVideo(ctx, p.SourceMusicVideoURL)
		if err != nil {
			return err
		}
		p.SourceMusicTitle = source.SoundTitle()
		p.SourceMusicVideoURL = source.VideoURL
		p.SourceMusicVideoID = source.VideoID
		p.SourceMusicAuthor = source.Author
		p.SourceMusicCollectedAt = source.CollectedAt
		return nil
	}
	if s.Studio == nil {
		return fmt.Errorf("Studio chưa sẵn sàng để đọc nguồn nhạc Radar TikTok")
	}
	source, err := s.Studio.ResolveTikTokRadarMusic(ctx)
	if err != nil {
		return err
	}
	p.SourceMusicTitle = source.SoundTitle()
	p.SourceMusicVideoURL = source.VideoURL
	p.SourceMusicVideoID = source.VideoID
	p.SourceMusicAuthor = source.Author
	p.SourceMusicCollectedAt = source.CollectedAt
	return nil
}

func (s *Server) handleStudioAffiliateCreatePrepared(w http.ResponseWriter, r *http.Request, planID string) {
	prep, err := s.Studio.GetAffiliatePreparation(planID)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape(err.Error()))
		return
	}
	p := prep.Params
	if p.ProductID > 0 {
		if s.Products == nil {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Kho sản phẩm chưa sẵn sàng; không thể xác minh trạng thái duyệt."))
			return
		}
		product, err := s.Products.GetByID(p.ProductID)
		if err != nil || product.ReviewStatus != products.ReviewApproved {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Sản phẩm đã bị gỡ khỏi danh sách duyệt; hãy duyệt lại trước khi tạo video."))
			return
		}
	}
	if raw := strings.TrimSpace(r.FormValue("account_id")); raw != "" {
		id, _ := strconv.ParseInt(raw, 10, 64)
		if id != p.AccountID {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Kênh đã đổi sau khi lập shot plan; hãy lập lại plan."))
			return
		}
	}
	if name := strings.TrimSpace(r.FormValue("product_name")); name != "" && name != p.ProductName {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Tên sản phẩm đã đổi sau khi lập shot plan; hãy lập lại plan."))
		return
	}
	if raw := strings.TrimSpace(r.FormValue("seconds")); raw != "" {
		seconds, _ := strconv.Atoi(raw)
		if seconds > 0 && seconds != p.Seconds {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Thời lượng đã đổi sau khi lập shot plan; hãy lập lại plan."))
			return
		}
	}
	if engine := strings.TrimSpace(r.FormValue("video_engine")); engine != "" && engine != p.VideoEngine {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Engine đã đổi sau khi lập shot plan; hãy lập lại plan."))
		return
	}
	if p.VideoEngine == studio.AffiliateVideoEngineGoogleFlow {
		if r.FormValue("confirm_flow_credits") != "1" {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Xác nhận Google Flow có thể dùng credits trước khi tạo video."))
			return
		}
		p.FlowCreditsAcknowledged = true
		if r.MultipartForm != nil && len(r.MultipartForm.File["motion_clips"]) > 0 {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Bỏ clip tải lên hoặc chọn chế độ thủ công."))
			return
		}
	} else {
		clips, err := saveMotionClips(r, "motion_clips", s.Studio.UploadDir(), 30)
		if err != nil {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Clip chuyển động lỗi: "+err.Error()))
			return
		}
		if len(clips) != len(prep.Shots) {
			seeOther(w, r, "/studio?err="+url.QueryEscape(fmt.Sprintf("Shot plan cần đúng %d clip theo đúng thứ tự; đã nhận %d.", len(prep.Shots), len(clips))))
			return
		}
		p.Clips = clips
	}
	if resolution := strings.TrimSpace(r.FormValue("output_resolution")); resolution != "" {
		p.OutputResolution = resolution
	}
	p.PreparedPlanID = prep.ID
	p.AspectRatio = "9:16"
	if title := strings.TrimSpace(r.FormValue("source_music_title")); title != "" {
		p.SourceMusicTitle = title
	}
	if sourceURL := strings.TrimSpace(r.FormValue("source_music_video_url")); sourceURL != "" {
		p.SourceMusicVideoURL = sourceURL
	}
	if err := s.ensureAffiliateRadarMusic(r.Context(), &p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape(err.Error()))
		return
	}
	if err := s.attachAffiliateVideoTracking(r.Context(), &p, prep.ID); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape(err.Error()))
		return
	}
	p.FixedCaption = strings.TrimSpace(r.FormValue("fixed_caption"))
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs?tab=running&msg=created")
}

// handleStudioAffiliateCreate queues an affiliate video job.
func (s *Server) handleStudioAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		studioErr("Không đọc được form — thử lại.")
		return
	}
	planID := strings.TrimSpace(r.FormValue("prepared_plan_id"))
	if planID == "" {
		studioErr("Lập shot plan trước khi tải clip chuyển động.")
		return
	}
	s.handleStudioAffiliateCreatePrepared(w, r, planID)
}

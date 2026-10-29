package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func (s *Server) attachAffiliateVideoTracking(ctx context.Context, p *studio.AffiliateParams, jobID string) error {
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
	legacyEngine := func(value string) string {
		switch strings.TrimSpace(value) {
		case "", studio.AffiliateVideoEngineGoogleFlow, studio.AffiliateVideoEngineManual:
			return studio.AffiliateVideoEngineMuseAI
		default:
			return strings.TrimSpace(value)
		}
	}
	if engine := strings.TrimSpace(r.FormValue("video_engine")); engine != "" && legacyEngine(engine) != legacyEngine(p.VideoEngine) {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Engine đã đổi sau khi lập shot plan; hãy lập lại plan."))
		return
	}
	p.VideoEngine = studio.AffiliateVideoEngineMuseAI
	p.FlowCreditsAcknowledged = r.PostFormValue("confirm_flow_credits") == "1"
	keepClipsForJob := false
	var clips []string
	defer func() {
		if !keepClipsForJob {
			removeUploadedFiles(clips)
		}
	}()
	clips, err = saveMotionClips(r, "motion_clips", s.Studio.UploadDir(), 30)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Clip tham chiếu lỗi: "+err.Error()))
		return
	}
	if len(clips) > 0 && len(clips) != len(prep.Shots) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(fmt.Sprintf("Nếu dùng clip tham chiếu, cần đúng %d clip theo thứ tự shot; đã nhận %d.", len(prep.Shots), len(clips))))
		return
	}
	p.Clips = clips
	if resolution := strings.TrimSpace(r.FormValue("output_resolution")); resolution != "" {
		p.OutputResolution = resolution
	}
	p.PreparedPlanID = prep.ID
	p.AspectRatio = "9:16"
	if title := strings.TrimSpace(r.FormValue("source_music_title")); title != "" {
		p.SourceMusicTitle = title
	}
	if wfID := strings.TrimSpace(r.FormValue("workflow_id")); wfID != "" {
		p.WorkflowID = wfID
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
		if job, exists := s.Studio.GetJob(prep.ID); exists {
			var saved studio.AffiliateParams
			if studioUnmarshalJSON(job.Params, &saved) == nil && sameUploadPaths(saved.Clips, clips) {
				keepClipsForJob = true
			}
		}
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job: "+err.Error()))
		return
	}
	keepClipsForJob = true
	seeOther(w, r, "/studio/jobs?tab=running&msg=created")
}

func sameUploadPaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
	r.Body = http.MaxBytesReader(w, r.Body, (601 << 20))
	parseErr := r.ParseMultipartForm(60 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		studioErr("Không đọc được form — thử lại.")
		return
	}
	if r.PostFormValue("confirm_flow_credits") != "1" {
		studioErr("Xác nhận video có thể dùng Muse AI credits trước khi tạo job.")
		return
	}
	planID := strings.TrimSpace(r.FormValue("prepared_plan_id"))
	if planID == "" {
		studioErr("Lập shot plan trước khi tạo video.")
		return
	}
	s.handleStudioAffiliateCreatePrepared(w, r, planID)
}

package web

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleStudioAffiliateCreatePrepared(w http.ResponseWriter, r *http.Request, planID string) {
	prep, err := s.Studio.GetAffiliatePreparation(planID)
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape(err.Error()))
		return
	}
	p := prep.Params
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
	p.BPM, _ = strconv.Atoi(r.FormValue("bpm"))
	p.MusicStart, _ = strconv.ParseFloat(r.FormValue("music_start"), 64)
	p.MusicTitle = strings.TrimSpace(r.FormValue("music_title"))
	p.MusicArtist = strings.TrimSpace(r.FormValue("music_artist"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("fixed_caption"))
	p.MusicPath, err = saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("File nhạc lỗi: "+err.Error()))
		return
	}
	if p.MusicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("%d-affiliate-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			err = studio.DownloadAudio(ctx, u, dst)
			cancel()
			if err != nil {
				seeOther(w, r, "/studio?err="+url.QueryEscape("Không tải được nhạc: "+err.Error()))
				return
			}
			p.MusicPath = dst
		}
	}
	if p.MusicPath == "" && strings.TrimSpace(r.FormValue("music_category")) != "" {
		p.MusicPath = studio.FindTrendingMusic(strings.TrimSpace(r.FormValue("music_category")))
		if p.MusicPath == "" {
			seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy nhạc cục bộ cho lựa chọn này."))
			return
		}
	}
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

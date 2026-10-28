package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Studio) runAffiliate(ctx context.Context, id string, p AffiliateParams, museCreditsAcknowledged bool) {
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, fmt.Sprintf("LỖI: panic (%v) — job dừng, app vẫn chạy.", r))
		}
	}()
	if p.Mode != AffiliateModeShots {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job Affiliate cũ dùng chế độ ảnh tĩnh; tạo job mới bằng footage chuyển động và ảnh listing thật.")
		return
	}
	switch p.VideoEngine {
	case "", AffiliateVideoEngineGoogleFlow, AffiliateVideoEngineManual:
		// Old saved jobs are migrated to the only supported production route.
		p.VideoEngine = AffiliateVideoEngineMuseAI
	case AffiliateVideoEngineMuseAI:
	default:
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, fmt.Sprintf("LỖI: engine affiliate không được hỗ trợ: %q", p.VideoEngine))
		return
	}
	targetSec := p.Seconds
	if targetSec <= 0 {
		targetSec = 60
	}
	if targetSec < 30 {
		targetSec = 30
	}
	if targetSec > 180 {
		targetSec = 180
	}
	p.Seconds = targetSec
	if p.PreparedProductBrief == nil || len(p.PreparedShots) == 0 {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job thiếu Product Analyst brief hoặc Director shot list đã lập trước clip")
		return
	}
	if !affiliateShotPlanMatchesDuration(p.Seconds, p.PreparedShots) {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, fmt.Sprintf("LỖI: shot plan không phủ đúng %d giây; tạo lại plan trước khi render", p.Seconds))
		return
	}
	if ready, reason := s.MuseVideoReady(ctx); !ready {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: Muse AI chưa sẵn sàng: "+reason)
		return
	}
	sem, slotErr := s.renderSlot(ctx)
	if slotErr != nil {
		if ctx.Err() != nil {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		} else {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, "LỖI: "+slotErr.Error())
		}
		return
	}
	defer func() { <-sem }()

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}
	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}
	modelPhotos := make([]string, 0, len(p.ModelPhotos)+1)
	seenPhotos := map[string]bool{}
	for _, photo := range append([]string{p.ModelPhoto}, p.ModelPhotos...) {
		clean := filepath.Clean(strings.TrimSpace(photo))
		if photo == "" || !fileExists(photo) || seenPhotos[clean] {
			continue
		}
		seenPhotos[clean] = true
		modelPhotos = append(modelPhotos, photo)
	}
	if len(modelPhotos) == 0 {
		fail(fmt.Errorf("không tìm thấy ảnh nhân vật gốc; dừng để tránh dùng nhầm người mẫu"))
		return
	}
	p.ModelPhoto = modelPhotos[0]
	p.ModelPhotos = modelPhotos
	if len(affiliateModelReferences(p, 3)) == 0 || len(affiliateProductReferences(p)) == 0 {
		fail(fmt.Errorf("thiếu bộ tham chiếu gồm nhân vật kênh và ảnh listing sản phẩm"))
		return
	}
	productProfile := *p.PreparedProductBrief
	shots := append([]ProShot(nil), p.PreparedShots...)
	if p.HasClips() {
		if len(p.Clips) != len(shots) {
			fail(fmt.Errorf("clip tham chiếu cần khớp %d shot; đã nhận %d", len(shots), len(p.Clips)))
			return
		}
		for i, clip := range p.Clips {
			if !fileExists(clip) || !isViralVideoFile(clip) {
				fail(fmt.Errorf("clip tham chiếu %d không phải video hợp lệ", i+1))
				return
			}
		}
	}
	if !IsTikTokVideoURL(p.SourceMusicVideoURL) {
		fail(fmt.Errorf("job thiếu video TikTok từ Radar để lấy sound"))
		return
	}
	s.appendLog(id, "PRODUCT_ANALYSIS_START: dùng lại Product Analyst brief đã duyệt trước khi quay clip")
	if err := writeAffiliateProductProfile(filepath.Join(work, "product-profile.json"), productProfile); err != nil {
		fail(fmt.Errorf("không lưu được product brief: %w", err))
		return
	}
	s.appendLog(id, fmt.Sprintf("PRODUCT_ANALYSIS_PASS: nhóm=%s · dùng=%s · focus=%s", productProfile.Category, productProfile.UseAction, productProfile.Focus))

	if p.VoiceoverPath == "" && strings.TrimSpace(p.VoiceoverText) != "" {
		if s.narrator == nil {
			fail(fmt.Errorf("đã yêu cầu voiceover nhưng TTS chưa sẵn sàng"))
			return
		}
		wavBytes, err := s.narrator(ctx, p.VoiceoverText, "warm-female")
		if err != nil || len(wavBytes) == 0 {
			if err == nil {
				err = fmt.Errorf("TTS trả audio rỗng")
			}
			fail(fmt.Errorf("tạo voiceover thất bại: %w", err))
			return
		}
		p.VoiceoverPath = filepath.Join(work, "affiliate_voice.wav")
		if err := os.WriteFile(p.VoiceoverPath, wavBytes, 0o644); err != nil {
			fail(fmt.Errorf("lưu voiceover thất bại: %w", err))
			return
		}
	}
	if p.VoiceoverPath != "" && !fileExists(p.VoiceoverPath) {
		fail(fmt.Errorf("file voiceover không tồn tại"))
		return
	}
	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media engine chưa sẵn sàng để chạy visual QC"))
		return
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok {
		fail(fmt.Errorf("media engine chưa hỗ trợ visual QC ảnh/video affiliate"))
		return
	}

	s.appendLog(id, "DIRECTOR_START: dùng lại shot list và prompt đã duyệt trước khi quay clip")
	s.appendLog(id, fmt.Sprintf("DIRECTOR_PASS: %d shot · hook, 3+ cỡ cảnh, hành động riêng, detail, payoff", len(shots)))
	s.appendLog(id, fmt.Sprintf("WORKFLOW_START: gửi %d shot qua Workflow API và Muse AI", len(shots)))
	final, err := s.runAffiliateWorkflow(ctx, id, work, p, productProfile, shots, museCreditsAcknowledged)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, "WORKFLOW_PASS: Muse AI, trim shot, concat, sound, grade, upscale và export hoàn tất")
	s.setStatus(id, StatusRunning, 85)
	if err := s.runAffiliateVisualGate(ctx, id, final, work, p, productProfile, shots); err != nil {
		// Preserve the rendered clip inside the job work directory for inspection
		// before removing the public delivery file, so rendered shots are not lost.
		rejectedPath := filepath.Join(work, "qc_rejected_"+filepath.Base(final))
		if renameErr := os.Rename(final, rejectedPath); renameErr != nil {
			_ = os.Remove(final)
		} else {
			s.appendLog(id, "Đã lưu bản nháp chưa qua QC tại workdir để kiểm tra lại từng cảnh: "+filepath.Base(rejectedPath))
		}
		fail(err)
		return
	}

	s.setOutput(id, filepath.Base(final))
	caption := strings.TrimSpace(p.FixedCaption)
	if caption == "" {
		caption = p.ProductName
	} else {
		s.appendLog(id, "Dùng chú thích bạn đã sửa.")
	}
	s.setCaption(id, caption)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, "Xong: "+filepath.Base(final))
	s.fireOnDone(id)
}

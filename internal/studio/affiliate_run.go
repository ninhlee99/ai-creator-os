package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Studio) runAffiliate(id string, p AffiliateParams) {
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
	if p.VideoEngine == AffiliateVideoEngineManual && !p.HasClips() {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: "+affiliateMotionClipsError)
		return
	}
	if p.VideoEngine != AffiliateVideoEngineGoogleFlow && p.VideoEngine != AffiliateVideoEngineManual {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, fmt.Sprintf("LỖI: engine affiliate không được hỗ trợ: %q", p.VideoEngine))
		return
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		if ready, reason := s.WebAIVideoReady(AffiliateVideoEngineGoogleFlow); !ready {
			s.setStatus(id, StatusFailed, 100)
			s.appendLog(id, "LỖI: Google Flow chưa sẵn sàng: "+reason)
			return
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

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

	if p.PreparedProductBrief == nil || len(p.PreparedShots) == 0 {
		fail(fmt.Errorf("job thiếu Product Analyst brief hoặc Director shot list đã lập trước clip"))
		return
	}
	if !IsTikTokVideoURL(p.SourceMusicVideoURL) {
		fail(fmt.Errorf("job thiếu video TikTok từ Radar để lấy sound"))
		return
	}
	productProfile := *p.PreparedProductBrief
	shots := append([]ProShot(nil), p.PreparedShots...)
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
	if p.VideoEngine == AffiliateVideoEngineManual && len(p.Clips) != len(shots) {
		fail(fmt.Errorf("cần đúng %d clip chuyển động theo shot list; đã nhận %d", len(shots), len(p.Clips)))
		return
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		s.appendLog(id, fmt.Sprintf("PRODUCTION_START: tạo %d shot riêng bằng Google Flow Ingredients; dùng ảnh nhân vật và listing thật", len(shots)))
	} else {
		s.appendLog(id, fmt.Sprintf("PRODUCTION_START: chuẩn hóa %d clip chuyển động người dùng cung cấp", len(shots)))
	}
	clips := make([]string, 0, len(shots))
	clipDurations := make([]float64, 0, len(shots))
	for i, sh := range shots {
		clipInput := ""
		if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
			clipInput = filepath.Join(work, fmt.Sprintf("flow-shot-%02d.mp4", i+1))
			refs := affiliateFlowReferencePaths(p)
			if len(refs) < 2 {
				fail(fmt.Errorf("shot %d: Google Flow cần ít nhất ảnh nhân vật và ảnh listing sản phẩm", i+1))
				return
			}
			prompt := AffiliateFlowVideoPrompt(p.ProductName, sh)
			s.appendLog(id, fmt.Sprintf("FLOW_SHOT_START: %d/%d · %s · focus=%s · use=%s", i+1, len(shots), sh.ShotSize, sh.ProductFocus, sh.ProductUseMode))
			_, err := s.GenerateWebAIVideo(ctx, AffiliateVideoEngineGoogleFlow, WebAIVideoRequest{
				Mode: "ingredients", Prompt: prompt, AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT", ImagePaths: refs,
			}, clipInput, func(line string) {
				if strings.TrimSpace(line) != "" {
					s.appendLog(id, line)
				}
			})
			if err != nil {
				fail(fmt.Errorf("Google Flow shot %d/%d: %w", i+1, len(shots), err))
				return
			}
			if !fileExists(clipInput) || !isViralVideoFile(clipInput) {
				fail(fmt.Errorf("Google Flow shot %d/%d không trả MP4 hợp lệ", i+1, len(shots)))
				return
			}
		} else {
			clipInput = strings.TrimSpace(p.Clips[i])
		}
		assetPrompt := fmt.Sprintf("%s · %s · %s · wardrobe: %s · setting: %s · product: %s · focus: %s · motion prompt: %s",
			sh.ShotSize, sh.LensLight, sh.EmotionBeat, sh.WardrobeStyling, sh.SceneSetting,
			sh.ProductInteraction, sh.ProductFocus, sh.VideoPrompt)
		aid := s.addAsset(id, 100+i, "clip", assetPrompt)
		s.setAsset(aid, StatusRunning, "")
		inputDuration := probeDuration(ctx, clipInput)
		if inputDuration < float64(sh.Seconds)-0.15 {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("shot %d: clip chỉ dài %.2fs, cần ít nhất %ds; không lặp footage để lấp thời lượng", i+1, inputDuration, sh.Seconds))
			return
		}
		s.setAsset(aid, StatusDone, clipInput)
		clips = append(clips, clipInput)
		clipDurations = append(clipDurations, float64(sh.Seconds))
		s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(shots))))
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		s.appendLog(id, fmt.Sprintf("PRODUCTION_PASS: Google Flow đã tạo %d shot có ingredients", len(clips)))
	} else {
		s.appendLog(id, fmt.Sprintf("PRODUCTION_PASS: đã nhận %d clip chuyển động thủ công", len(clips)))
	}
	silent := filepath.Join(work, "silent.mp4")
	s.appendLog(id, fmt.Sprintf("REMASTER_START: ghép shot và upscale một lượt lên %s · giữ trọn khung dọc", strings.ToUpper(p.OutputResolution)))
	if err := ConcatClipsToMaster(ctx, clips, clipDurations, p.AspectRatio, p.OutputResolution, silent, false); err != nil {
		fail(fmt.Errorf("nối các shot: %w", err))
		return
	}
	s.appendLog(id, "SOUND_SOURCE_START: tải video đã chọn từ Radar và tách âm thanh gốc")
	sourceMusicPath, err := downloadTikTokRadarAudio(ctx, p.SourceMusicVideoURL, work)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, "SOUND_SOURCE_PASS: sound đã tách từ video TikTok Radar")
	assembled := filepath.Join(work, "assembled.mp4")
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	_ = os.Remove(final)
	masterComplete := false
	defer func() {
		if !masterComplete {
			_ = os.Remove(final)
		}
	}()
	if p.VoiceoverPath != "" {
		voiceBase := filepath.Join(work, "voice-base.mp4")
		if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v:0", "-map", "1:a:0", "-af", "apad", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceBase); err != nil {
			fail(fmt.Errorf("ghép voiceover: %w", err))
			return
		}
		if err := MixMusicBed(ctx, voiceBase, sourceMusicPath, assembled); err != nil {
			fail(fmt.Errorf("ghép sound Radar dưới voiceover: %w", err))
			return
		}
	} else {
		if err := MuxMusic(ctx, silent, sourceMusicPath, 0, assembled); err != nil {
			fail(fmt.Errorf("ghép sound Radar vào video: %w", err))
			return
		}
	}
	if err := verifyVideoDuration(ctx, assembled, float64(p.Seconds)); err != nil {
		fail(fmt.Errorf("video affiliate đầu ra phải dài %ds: %w", p.Seconds, err))
		return
	}
	if err := UpscaleVideo(ctx, assembled, final, p.AspectRatio, p.OutputResolution); err != nil {
		fail(fmt.Errorf("upscale master %s: %w", strings.ToUpper(p.OutputResolution), err))
		return
	}
	if !ProbeHasAudio(ctx, final) {
		fail(fmt.Errorf("đầu ra affiliate bị mất sound đã tách từ Radar"))
		return
	}
	masterWidth, masterHeight := ProbeDims(ctx, final)
	s.appendLog(id, fmt.Sprintf("REMASTER_PASS: master %s %dx%d", strings.ToUpper(p.OutputResolution), masterWidth, masterHeight))
	s.setStatus(id, StatusRunning, 85)
	if err := s.runAffiliateVisualGate(ctx, id, final, work, p, productProfile, shots); err != nil {
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
	masterComplete = true
	s.fireOnDone(id)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// CancelRunning hủy context của job đang chạy trong process này (nếu có) —
// ffmpeg/ffprobe được chạy bằng exec.CommandContext nên sẽ bị kill thật,
// giải phóng cả slot render semaphore. Dùng bởi SweepHungJobs (Đợt S):
// job treo trước đây chỉ bị đánh dấu failed trên DB trong khi process vẫn
// ngốn CPU/đĩa và giữ slot tới khi restart app.

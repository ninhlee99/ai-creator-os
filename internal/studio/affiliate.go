package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const affiliateMotionClipsError = "chế độ free-only không gọi API video tính phí; cần tải lên một clip chuyển động cho mỗi shot, nhân vật phải mặc hoặc dùng đúng sản phẩm"

// AutoAffiliateProductionReady reports whether Studio can produce all footage
// without operator-supplied clips. Current manual workflow is not autonomous.
func (s *Studio) AutoAffiliateProductionReady() (bool, string) {
	return false, "tạo video affiliate tự động đang tạm dừng; cần lập shot plan rồi cung cấp clip chuyển động cho từng shot"
}

func (s *Studio) CreateAffiliateJob(p AffiliateParams) (string, error) {
	preparationID := strings.TrimSpace(p.PreparedPlanID)
	if preparationID == "" {
		return "", fmt.Errorf("lập Product Analyst + Director shot plan trước khi tải clip; %s", affiliateMotionClipsError)
	}
	prep, err := s.GetAffiliatePreparation(preparationID)
	if err != nil {
		return "", err
	}
	if p.AccountID > 0 && p.AccountID != prep.Params.AccountID {
		return "", fmt.Errorf("shot plan thuộc kênh khác")
	}
	submitted := p
	p = prep.Params
	p.Clips = submitted.Clips
	if submitted.MusicPath != "" {
		p.MusicPath = submitted.MusicPath
	}
	if submitted.MusicStart != 0 {
		p.MusicStart = submitted.MusicStart
	}
	if submitted.MusicTitle != "" {
		p.MusicTitle = submitted.MusicTitle
	}
	if submitted.MusicArtist != "" {
		p.MusicArtist = submitted.MusicArtist
	}
	if submitted.VoiceoverPath != "" {
		p.VoiceoverPath = submitted.VoiceoverPath
	}
	if submitted.VoiceoverText != "" {
		p.VoiceoverText = submitted.VoiceoverText
	}
	if submitted.FixedCaption != "" {
		p.FixedCaption = submitted.FixedCaption
	}
	if submitted.AffLink != "" {
		p.AffLink = submitted.AffLink
	}
	p.PreparedPlanID = preparationID
	if p.PreparedProductBrief == nil || len(p.PreparedShots) == 0 {
		return "", fmt.Errorf("shot plan thiếu Product Analyst brief hoặc Director shot list")
	}
	if p.Seconds < 30 {
		p.Seconds = 30 // Video affiliate thời lượng tối thiểu 30s - 60s
	}
	if p.Seconds > 180 {
		p.Seconds = 180
	}
	if p.Mode == "" {
		p.Mode = AffiliateModeShots
	}
	if p.Mode != AffiliateModeShots {
		return "", fmt.Errorf("affiliate production chỉ nhận video có footage chuyển động; chế độ ảnh tĩnh không được xuất thành video hoàn chỉnh")
	}
	if !p.HasClips() {
		return "", fmt.Errorf("%s", affiliateMotionClipsError)
	}
	expectedClips := affiliateShotCount(p.Seconds)
	if len(p.PreparedShots) > 0 {
		expectedClips = len(p.PreparedShots)
	}
	if len(p.Clips) != expectedClips {
		return "", fmt.Errorf("video %ds cần %d clip theo shot list; đã nhận %d", p.Seconds, expectedClips, len(p.Clips))
	}
	for i, clip := range p.Clips {
		if !isViralVideoFile(strings.TrimSpace(clip)) {
			return "", fmt.Errorf("clip %d không phải video hợp lệ", i+1)
		}
	}
	if strings.TrimSpace(p.ProductName) == "" {
		return "", fmt.Errorf("thiếu tên sản phẩm")
	}
	if p.AccountID <= 0 {
		return "", fmt.Errorf("cần chọn kênh để khóa đúng ảnh nhân vật")
	}
	if !fileExists(p.ProductPhoto) {
		return "", fmt.Errorf("thiếu ảnh sản phẩm thật để khóa hình dáng và chi tiết")
	}
	channelPhotos := s.queryAccountModelPhotos(p.AccountID)
	allowedPhotos := make(map[string]bool, len(channelPhotos))
	for _, photo := range channelPhotos {
		allowedPhotos[filepath.Clean(photo)] = true
	}
	if p.ModelPhoto == "" {
		for _, photo := range p.ModelPhotos {
			if allowedPhotos[filepath.Clean(photo)] {
				p.ModelPhoto = photo
				break
			}
		}
	}
	if !fileExists(p.ModelPhoto) || !allowedPhotos[filepath.Clean(p.ModelPhoto)] {
		return "", fmt.Errorf("thiếu ảnh nhân vật của kênh; chọn hoặc tải ảnh người mẫu trước khi dựng")
	}
	seenModelPhotos := map[string]bool{filepath.Clean(p.ModelPhoto): true}
	selectedModelPhotos := []string{p.ModelPhoto}
	for _, photo := range p.ModelPhotos {
		clean := filepath.Clean(photo)
		if !allowedPhotos[clean] {
			return "", fmt.Errorf("ảnh nhân vật không thuộc thư viện của kênh đã chọn")
		}
		if !seenModelPhotos[clean] {
			seenModelPhotos[clean] = true
			selectedModelPhotos = append(selectedModelPhotos, photo)
		}
	}
	p.ModelPhotos = selectedModelPhotos
	if s.mg == nil || !s.mg.Healthy(context.Background()) {
		return "", fmt.Errorf("media engine chưa sẵn sàng để kiểm tra ảnh/video đầu ra")
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok {
		return "", fmt.Errorf("media engine chưa hỗ trợ visual QC ảnh/video affiliate")
	}
	if p.VideoEngine == "" {
		p.VideoEngine = "manual"
	}
	if p.VideoEngine != "manual" {
		return "", fmt.Errorf("engine %q bị tắt theo chính sách free-only; tải clip chuyển động do bạn cung cấp", p.VideoEngine)
	}
	if p.BPM < 60 || p.BPM > 200 {
		p.BPM = 120
	}
	planID := p.PreparedPlanID
	p.PreparedPlanID = ""
	if err := s.consumeAffiliatePreparation(planID); err != nil {
		return "", err
	}
	title := p.ProductName
	if title == "" {
		title = "Video affiliate"
	}
	id, err := s.insertJob(KindAffiliate, title, p)
	if err != nil {
		return "", err
	}
	go s.runAffiliate(id, p)
	return id, nil
}

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
	if !p.HasClips() {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: "+affiliateMotionClipsError)
		return
	}
	if p.VideoEngine != "" && p.VideoEngine != "manual" {
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: video API bị tắt theo chính sách free-only; dùng clip chuyển động do người vận hành cung cấp.")
		return
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
	if len(p.Clips) != len(shots) {
		fail(fmt.Errorf("cần đúng %d clip chuyển động theo shot list; đã nhận %d", len(shots), len(p.Clips)))
		return
	}
	s.appendLog(id, fmt.Sprintf("PRODUCTION_START: chuẩn hóa %d clip chuyển động người dùng cung cấp", len(shots)))
	clips := make([]string, 0, len(shots))
	for i, sh := range shots {
		clipInput := strings.TrimSpace(p.Clips[i])
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
		out := filepath.Join(work, fmt.Sprintf("shot-%02d.mp4", i))
		if err := TrimVideo(ctx, clipInput, float64(sh.Seconds), "9:16", out); err != nil || !fileExists(out) {
			if err == nil {
				err = fmt.Errorf("không chuẩn hóa được clip chuyển động")
			}
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("shot %d: chuẩn hóa footage: %w", i+1, err))
			return
		}
		if err := verifyVideoDuration(ctx, out, float64(sh.Seconds)); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("shot %d: thời lượng clip sau chuẩn hóa sai: %w", i+1, err))
			return
		}
		s.setAsset(aid, StatusDone, out)
		clips = append(clips, out)
		s.setStatus(id, StatusRunning, 10+int(60*float64(i+1)/float64(len(shots))))
	}
	s.appendLog(id, fmt.Sprintf("PRODUCTION_PASS: đã chuẩn hóa %d clip chuyển động; không gọi video API", len(clips)))
	silent := filepath.Join(work, "silent.mp4")
	if err := ConcatClips(ctx, clips, "9:16", silent); err != nil {
		fail(fmt.Errorf("nối các shot: %w", err))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.VoiceoverPath != "" {
		voiceVideo := filepath.Join(work, "voice.mp4")
		if err := ffmpegRun(ctx, "-i", silent, "-i", p.VoiceoverPath, "-map", "0:v:0", "-map", "1:a:0", "-af", "apad", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", voiceVideo); err != nil {
			fail(fmt.Errorf("ghép voiceover: %w", err))
			return
		}
		if p.MusicPath != "" {
			if err := MixMusicBed(ctx, voiceVideo, p.MusicPath, final); err != nil {
				fail(fmt.Errorf("hòa âm voiceover và nhạc: %w", err))
				return
			}
		} else if err := os.Rename(voiceVideo, final); err != nil {
			fail(err)
			return
		}
	} else if p.MusicPath != "" {
		if err := MuxMusic(ctx, silent, p.MusicPath, p.MusicStart, final); err != nil {
			fail(fmt.Errorf("ghép nhạc: %w", err))
			return
		}
	} else {
		s.appendLog(id, "Không có audio do người vận hành chỉ định; bỏ track âm thanh sinh tự động.")
		if err := ffmpegRun(ctx, "-i", silent, "-map", "0:v:0", "-c:v", "copy", "-an", final); err != nil {
			fail(fmt.Errorf("loại audio sinh tự động: %w", err))
			return
		}
	}
	if err := verifyVideoDuration(ctx, final, float64(p.Seconds)); err != nil {
		fail(fmt.Errorf("video affiliate đầu ra phải dài %ds: %w", p.Seconds, err))
		return
	}
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

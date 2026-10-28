package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func affiliateFlowReferencePaths(p AffiliateParams) []string {
	paths := []string{p.ModelPhoto, p.ProductPhoto}
	if len(p.ExtraPhotos) > 0 {
		paths = append(paths, p.ExtraPhotos[0])
	}
	seen := map[string]bool{}
	refs := make([]string, 0, 3)
	for _, path := range paths {
		path = strings.TrimSpace(path)
		clean := filepath.Clean(path)
		if path == "" || !fileExists(path) || seen[clean] {
			continue
		}
		seen[clean] = true
		refs = append(refs, path)
		if len(refs) == 3 {
			break
		}
	}
	return refs
}

// AutoAffiliateProductionReady reports whether Studio can render planned shots
// through Workflow API and Muse AI without operator-supplied motion references.
func (s *Studio) AutoAffiliateProductionReady() (bool, string) {
	if _, err := s.ResolveTikTokRadarMusic(context.Background()); err != nil {
		return false, err.Error()
	}
	if ready, reason := s.MuseVideoReady(context.Background()); !ready {
		return false, reason
	}
	if s.llm == nil || !s.llm.Healthy(context.Background()) {
		return false, "LLM chưa sẵn sàng để lập shot plan"
	}
	if s.mg == nil || !s.mg.Healthy(context.Background()) {
		return false, "media engine chưa sẵn sàng để phân tích sản phẩm và QC"
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok {
		return false, "media engine chưa hỗ trợ phân tích ảnh và QC video"
	}
	return true, "Muse AI, Product Analyst, Director và visual QC sẵn sàng"
}

func (s *Studio) CreateAffiliateJob(p AffiliateParams) (string, error) {
	preparationID := strings.TrimSpace(p.PreparedPlanID)
	if preparationID == "" {
		return "", fmt.Errorf("lập Product Analyst + Director shot plan trước khi tạo video")
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
	p.FlowCreditsAcknowledged = submitted.FlowCreditsAcknowledged
	if submitted.OutputResolution != "" {
		p.OutputResolution = submitted.OutputResolution
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
	if submitted.AffiliateAttributionID != "" {
		p.AffiliateAttributionID = submitted.AffiliateAttributionID
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
	if p.VideoEngine == "" {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	if p.VideoEngine == AffiliateVideoEngineGoogleFlow {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	if p.VideoEngine == AffiliateVideoEngineManual {
		p.VideoEngine = AffiliateVideoEngineMuseAI
	}
	p.OutputResolution, err = NormalizeVideoOutputResolution(p.OutputResolution)
	if err != nil {
		return "", err
	}
	if p.AspectRatio == "" {
		p.AspectRatio = "9:16"
	}
	if p.AspectRatio != "9:16" {
		return "", fmt.Errorf("affiliate hiện chỉ hỗ trợ khung dọc 9:16")
	}
	if p.VideoEngine != AffiliateVideoEngineMuseAI {
		return "", fmt.Errorf("engine affiliate không được hỗ trợ: %q", p.VideoEngine)
	}
	if p.VideoEngine == AffiliateVideoEngineMuseAI && !p.FlowCreditsAcknowledged {
		return "", fmt.Errorf("cần xác nhận Muse AI có thể dùng credits trước khi tạo video")
	}
	expectedClips := len(p.PreparedShots)
	if p.HasClips() && len(p.Clips) != expectedClips {
		return "", fmt.Errorf("clip tham chiếu cần khớp %d shot; đã nhận %d", expectedClips, len(p.Clips))
	}
	if !IsTikTokVideoURL(p.SourceMusicVideoURL) {
		return "", fmt.Errorf("chọn video nhảy có sound trong Radar TikTok trước khi tạo affiliate")
	}
	source, err := s.ResolveTikTokRadarDanceVideo(context.Background(), p.SourceMusicVideoURL)
	if err != nil {
		return "", fmt.Errorf("không xác minh được sound Radar TikTok: %w", err)
	}
	p.SourceMusicTitle = source.SoundTitle()
	p.SourceMusicVideoURL = source.VideoURL
	p.SourceMusicVideoID = source.VideoID
	p.SourceMusicAuthor = source.Author
	p.SourceMusicCollectedAt = source.CollectedAt
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelProbe()
	for i, clip := range p.Clips {
		clip = strings.TrimSpace(clip)
		if !isViralVideoFile(clip) || probeDuration(probeCtx, clip) <= 0 {
			return "", fmt.Errorf("clip %d không phải video hợp lệ", i+1)
		}
		width, height := ProbeDims(probeCtx, clip)
		if width <= 0 || height <= 0 {
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
	if p.VideoEngine == AffiliateVideoEngineMuseAI {
		if ready, reason := s.MuseVideoReady(context.Background()); !ready {
			return "", fmt.Errorf("Muse AI chưa sẵn sàng: %s", reason)
		}
	}
	// Consent applies to this submission only; retries must ask again.
	museCreditsAcknowledged := p.FlowCreditsAcknowledged
	p.FlowCreditsAcknowledged = false
	planID := p.PreparedPlanID
	p.PreparedPlanID = ""
	title := p.ProductName
	if title == "" {
		title = "Video affiliate"
	}
	// The one-use plan ID is also the job ID. AccessTrade Sub2 can therefore
	// point directly to the exact video job without a separate lookup table.
	if err := s.insertAffiliateJobWithPlan(planID, title, p); err != nil {
		return "", err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	if err := s.startTrackedJob(planID, runCtx, cancel, func(ctx context.Context) {
		s.runAffiliate(ctx, planID, p, museCreditsAcknowledged)
	}); err != nil {
		s.appendLog(planID, "LỖI: "+err.Error())
		s.setStatus(planID, StatusFailed, 100)
		return "", err
	}
	return planID, nil
}

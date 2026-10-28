package studio

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"time"
)

const (
	minViralSourceDurationSeconds  = 3.0
	maxViralSegmentDurationSeconds = 10.0
	maxViralRenderSegments         = 20
	maxViralSourceDurationSeconds  = maxViralSegmentDurationSeconds * maxViralRenderSegments
)

func validateViralSourceDuration(duration float64) error {
	if math.IsNaN(duration) || math.IsInf(duration, 0) {
		return fmt.Errorf("thời lượng video nguồn không hợp lệ")
	}
	if duration < minViralSourceDurationSeconds {
		return fmt.Errorf("video nguồn phải dài ít nhất %.0f giây", minViralSourceDurationSeconds)
	}
	if duration > maxViralSourceDurationSeconds {
		return fmt.Errorf("video nguồn dài tối đa %.0f giây do giới hạn %d shot Muse AI", maxViralSourceDurationSeconds, maxViralRenderSegments)
	}
	return nil
}

// EnqueueViral đưa một tác vụ tạo video viral vào hàng đợi render của Studio.
func (s *Studio) EnqueueViral(ctx context.Context, p ViralParams) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !p.FlowCreditsAcknowledged {
		return "", fmt.Errorf("cần xác nhận Muse AI có thể dùng credits trước khi tạo video")
	}
	if ready, reason := s.ViralCharacterEditorStatus(ctx); !ready {
		return "", fmt.Errorf("viral editor chưa sẵn sàng: %s", reason)
	}
	if p.AIMode != "" && p.AIMode != ViralModeFullCharacterEdit {
		return "", fmt.Errorf("chỉ hỗ trợ thay toàn bộ nhân vật trên video nguồn")
	}
	if !fileExists(p.SampleVideo) || !isViralVideoFile(p.SampleVideo) {
		return "", fmt.Errorf("cần file video nguồn hợp lệ")
	}
	if !IsTikTokVideoURL(p.SourceVideoURL) {
		return "", fmt.Errorf("cần video nguồn TikTok đã chọn trong Radar")
	}
	sourceVideo, err := s.ResolveTikTokRadarDanceVideo(ctx, p.SourceVideoURL)
	if err != nil {
		return "", fmt.Errorf("video nguồn không thuộc Radar đã lưu: %w", err)
	}
	if p.SourceVideoID != "" && p.SourceVideoID != sourceVideo.VideoID {
		return "", fmt.Errorf("video ID không khớp URL nguồn Radar")
	}
	p.SourceVideoURL = sourceVideo.VideoURL
	p.SourceVideoID = sourceVideo.VideoID
	p.SourceVideoAuthor = sourceVideo.Author
	p.SourceMusicTitle = sourceVideo.SoundTitle()
	p.SourceCollectedAt = sourceVideo.CollectedAt
	if !ProbeHasAudio(ctx, p.SampleVideo) {
		return "", fmt.Errorf("video nguồn Radar không có track âm thanh")
	}
	duration := probeDuration(ctx, p.SampleVideo)
	if err := validateViralSourceDuration(duration); err != nil {
		return "", err
	}
	width, height, err := probeViralDimensions(ctx, p.SampleVideo)
	if err != nil {
		return "", fmt.Errorf("không đọc được kích thước video nguồn: %w", err)
	}
	if math.Abs(float64(width)/float64(height)-9.0/16.0) > 0.015 {
		return "", fmt.Errorf("video nguồn %dx%d không phải khung dọc 9:16", width, height)
	}
	if p.TargetDuration > 0 && math.Abs(p.TargetDuration-duration) > 0.25 {
		return "", fmt.Errorf("thời lượng gửi không khớp video nguồn")
	}
	p.TargetDuration = duration
	resolution, err := NormalizeVideoOutputResolution(p.OutputResolution)
	if err != nil {
		return "", err
	}
	p.OutputResolution = resolution
	p.AIMode = ViralModeFullCharacterEdit
	photos := append([]string(nil), p.ModelPhotos...)
	if p.ModelPhoto != "" {
		photos = append([]string{p.ModelPhoto}, photos...)
	}
	photos = validCharacterPhotos(photos, 3)
	if len(photos) == 0 {
		return "", fmt.Errorf("cần ít nhất một ảnh nhân vật hợp lệ")
	}
	if p.AccountID <= 0 {
		return "", fmt.Errorf("chọn kênh đích để khóa đúng nhân vật")
	}
	allowed := map[string]bool{}
	for _, path := range s.queryAccountModelPhotos(p.AccountID) {
		allowed[filepath.Clean(path)] = true
	}
	for _, path := range photos {
		if !allowed[filepath.Clean(path)] {
			return "", fmt.Errorf("ảnh nhân vật không thuộc thư viện kênh đã chọn")
		}
	}
	p.ModelPhoto = photos[0]
	p.ModelPhotos = photos
	if p.Topic == "" {
		p.Topic = "Video viral"
	}
	museCreditsAcknowledged := p.FlowCreditsAcknowledged
	p.FlowCreditsAcknowledged = false
	id, err := s.insertJob(KindViral, p.Topic, p)
	if err != nil {
		return "", err
	}
	timeout := 20*time.Minute + time.Duration(duration*10)*time.Second
	if timeout > 6*time.Hour {
		timeout = 6 * time.Hour
	}
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	if err := s.startTrackedJob(id, runCtx, cancel, func(ctx context.Context) {
		s.runViral(ctx, id, p, museCreditsAcknowledged)
	}); err != nil {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
		return "", err
	}
	return id, nil
}

// ListViralJobs trả về danh sách các job viral đã và đang sản xuất.
func (s *Studio) ListViralJobs(limit int) []Job {
	if limit <= 0 {
		limit = 30
	}
	jobs := s.ListJobs(limit)
	var out []Job
	for _, j := range jobs {
		if j.Kind == KindViral {
			out = append(out, j)
		}
	}
	return out
}

// runViral runs full-character source editing on the selected viral video.
func (s *Studio) runViral(ctx context.Context, jobID string, p ViralParams, museCreditsAcknowledged bool) {
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(jobID, StatusFailed, 100)
			s.appendLog(jobID, fmt.Sprintf("LỖI panic (%v) — job dừng an toàn.", r))
		}
	}()
	s.runViralFullCharacterEdit(ctx, jobID, p, museCreditsAcknowledged)
}

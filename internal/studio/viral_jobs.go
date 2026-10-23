package studio

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
)

// EnqueueViral đưa một tác vụ tạo video viral vào hàng đợi render của Studio.
func (s *Studio) EnqueueViral(ctx context.Context, p ViralParams) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !p.FlowCreditsAcknowledged {
		return "", fmt.Errorf("cần xác nhận Google Flow có thể dùng credits trước khi tạo video")
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
	duration := probeDuration(ctx, p.SampleVideo)
	if duration < 3 {
		return "", fmt.Errorf("video nguồn phải dài ít nhất 3 giây")
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
	id, err := s.insertJob(KindViral, p.Topic, p)
	if err != nil {
		return "", err
	}
	go s.runViral(id, p)
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
func (s *Studio) runViral(jobID string, p ViralParams) {
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(jobID, StatusFailed, 100)
			s.appendLog(jobID, fmt.Sprintf("LỖI panic (%v) — job dừng an toàn.", r))
		}
	}()
	s.runViralFullCharacterEdit(jobID, p)
}

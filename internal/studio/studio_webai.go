package studio

import (
	"context"
	"fmt"
	"strings"
)

func (s *Studio) SetWebAIResolver(r WebAIResolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webAI = r
}

func (s *Studio) SetWebAIImageAnalyzer(analyzer WebAIImageAnalyzer) {
	s.mu.Lock()
	s.webAIImageAnalyzer = analyzer
	s.mu.Unlock()
}

func (s *Studio) SetWebAIVideoRenderer(renderer WebAIVideoRenderer, availability WebAIAvailability) {
	s.mu.Lock()
	s.webAIVideo = renderer
	s.webAIAvailability = availability
	s.mu.Unlock()
}

func (s *Studio) WebAIVideoReady(platform string) (bool, string) {
	s.mu.Lock()
	availability := s.webAIAvailability
	rendererReady := s.webAIVideo != nil
	s.mu.Unlock()
	if !rendererReady {
		return false, "bộ tạo video Web AI chưa được nối"
	}
	if availability == nil {
		return false, "chưa kiểm tra được account Web AI"
	}
	return availability(platform)
}

func (s *Studio) GenerateWebAIVideo(ctx context.Context, platform string, request WebAIVideoRequest, outputPath string, progress func(string)) (string, error) {
	s.mu.Lock()
	renderer := s.webAIVideo
	s.mu.Unlock()
	if renderer == nil {
		return "", fmt.Errorf("bộ tạo video Web AI chưa sẵn sàng")
	}
	session, err := s.GetWebAISession(platform)
	if err != nil {
		return "", err
	}
	if session == nil || strings.TrimSpace(session.CookieHeader) == "" && len(session.CookieJar) == 0 {
		return "", fmt.Errorf("account %s chưa có phiên đăng nhập hợp lệ", platform)
	}
	if progress == nil {
		progress = func(string) {}
	}
	return renderer(ctx, session, request, outputPath, progress)
}

// GetWebAISession lấy thông tin phiên Web AI đang hoạt động cho một nền tảng.
func (s *Studio) GetWebAISession(platform string) (*WebAISession, error) {
	s.mu.Lock()
	r := s.webAI
	s.mu.Unlock()
	if r == nil {
		return nil, fmt.Errorf("studio: web ai resolver chưa được thiết lập")
	}
	return r(platform)
}

// maxConcurrentStudioJobs bounds how many jobs render at once. Each job
// runs heavy local ffmpeg work (1080x1920 zoompan graphs plus a lanczos
// 4K upscale per photo); more than 2 parallel renders thrash CPU and memory
// for no wall-clock win.
// Extra jobs wait with status "queued" (their DB state until they start).

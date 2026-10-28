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

func (s *Studio) SetWebAIAccountResolver(r WebAIAccountResolver) {
	s.mu.Lock()
	s.webAIAccount = r
	s.mu.Unlock()
}

func (s *Studio) SetWebAIAccountIDResolver(r WebAIAccountIDResolver) {
	s.mu.Lock()
	s.webAIAccountByID = r
	s.mu.Unlock()
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

func (s *Studio) SetWebAIMuseRenderer(renderer MuseChatRenderer) {
	s.mu.Lock()
	s.webAIMuse = renderer
	s.mu.Unlock()
}

func (s *Studio) WebAIVideoReady(platform string) (bool, string) {
	s.mu.Lock()
	availability := s.webAIAvailability
	rendererReady := s.webAIVideo != nil
	if platform == "museai" {
		rendererReady = s.webAIMuse != nil
	}
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
	projectID, _, err := s.GenerateWebAIVideoForAccount(ctx, platform, "", request, outputPath, progress)
	return projectID, err
}

// GenerateWebAIVideoForAccount keeps successive workflow shots on the same
// Google Flow account so their project remains accessible between nodes.
func (s *Studio) GenerateWebAIVideoForAccount(ctx context.Context, platform, accountID string, request WebAIVideoRequest, outputPath string, progress func(string)) (string, string, error) {
	s.mu.Lock()
	renderer := s.webAIVideo
	s.mu.Unlock()
	if renderer == nil {
		return "", accountID, fmt.Errorf("bộ tạo video Web AI chưa sẵn sàng")
	}
	var session *WebAISession
	var err error
	if strings.TrimSpace(accountID) != "" {
		session, err = s.GetWebAISessionByID(platform, accountID)
	} else {
		session, err = s.GetWebAISession(platform)
	}
	if err != nil {
		return "", accountID, err
	}
	if session == nil || strings.TrimSpace(session.CookieHeader) == "" && len(session.CookieJar) == 0 {
		return "", accountID, fmt.Errorf("account %s chưa có phiên đăng nhập hợp lệ", platform)
	}
	if progress == nil {
		progress = func(string) {}
	}
	projectID, err := renderer(ctx, session, request, outputPath, progress)
	return projectID, session.AccountID, err
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

func (s *Studio) GetWebAISessionForEmail(platform, email string) (*WebAISession, error) {
	if strings.TrimSpace(email) == "" {
		return s.GetWebAISession(platform)
	}
	s.mu.Lock()
	r := s.webAIAccount
	s.mu.Unlock()
	if r == nil {
		return nil, fmt.Errorf("studio: explicit web ai account resolver is not configured")
	}
	return r(platform, email)
}

func (s *Studio) GetWebAISessionByID(platform, accountID string) (*WebAISession, error) {
	if strings.TrimSpace(accountID) == "" {
		return s.GetWebAISession(platform)
	}
	s.mu.Lock()
	r := s.webAIAccountByID
	s.mu.Unlock()
	if r == nil {
		return nil, fmt.Errorf("studio: explicit web ai account ID resolver is not configured")
	}
	return r(platform, accountID)
}

func (s *Studio) RenderMuseChat(ctx context.Context, session *WebAISession, request MuseChatRequest, outputPath string, progress func(string)) (string, error) {
	s.mu.Lock()
	renderer := s.webAIMuse
	s.mu.Unlock()
	if renderer == nil {
		return "", fmt.Errorf("Muse internal renderer is not configured")
	}
	if session == nil || strings.TrimSpace(session.CookieHeader) == "" && len(session.CookieJar) == 0 {
		return "", fmt.Errorf("Muse account has no valid saved session")
	}
	if progress == nil {
		progress = func(string) {}
	}
	return renderer(ctx, session, request, outputPath, progress)
}

// maxConcurrentStudioJobs bounds how many jobs render at once. Each job
// runs heavy local ffmpeg work (1080x1920 zoompan graphs plus a lanczos
// 4K upscale per photo); more than 2 parallel renders thrash CPU and memory
// for no wall-clock win.
// Extra jobs wait with status "queued" (their DB state until they start).

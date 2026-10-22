package webai

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

type CapturedWebAuth struct {
	Platform                string
	SessionValue            string
	CookieHeader            string
	Email                   string
	BrowserUsed             string
	BrowserIdentityVerified bool
}

// ActiveLoginSession represents an ongoing browser login attempt.
type ActiveLoginSession struct {
	ID          string
	Platform    string
	Status      string // "opening", "waiting_for_user", "success", "failed", "canceled"
	Message     string
	Email       string
	Error       string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	CancelFn    func()
	browserPort int
	ctx         context.Context
}

// Context returns the session's lifecycle context.
func (s *ActiveLoginSession) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// BrowserLoginManager coordinates in-flight browser login sessions.
type BrowserLoginManager struct {
	mu       sync.Mutex
	sessions map[string]*ActiveLoginSession
}

var globalLoginManager = &BrowserLoginManager{
	sessions: make(map[string]*ActiveLoginSession),
}

// GetLoginManager returns the singleton browser login manager.
func GetLoginManager() *BrowserLoginManager {
	return globalLoginManager
}

// StartSession initiates an interactive browser login session.
func (m *BrowserLoginManager) StartSession(platform string, timeout time.Duration) (*ActiveLoginSession, error) {
	adapter, ok := webLoginAdapterFor(platform)
	if !ok {
		return nil, fmt.Errorf("nền tảng %q chưa được hỗ trợ đăng nhập tự động", platform)
	}
	target := adapter.target()

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, fmt.Errorf("không tạo được mã phiên đăng nhập an toàn: %w", err)
	}
	sessionID := "login_" + base64.RawURLEncoding.EncodeToString(token[:])
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	session := &ActiveLoginSession{
		ID:        sessionID,
		Platform:  platform,
		Status:    "opening",
		Message:   fmt.Sprintf("Đang khởi chạy trình duyệt bảo mật cho %s...", target.Name),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(timeout),
		CancelFn:  cancel,
		ctx:       ctx,
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return session, nil
}

// GetSession returns the current state of a login session.
func (m *BrowserLoginManager) GetSession(id string) (*ActiveLoginSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	snapshot := *s
	return &snapshot, true
}

// UpdateSession changes public session state while holding the manager lock.
func (m *BrowserLoginManager) UpdateSession(id, status, message, email, errorText string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || s.Status == "canceled" || s.Status == "success" {
		return
	}
	if status != "" {
		s.Status = status
	}
	if message != "" {
		s.Message = message
	}
	if email != "" {
		s.Email = email
	}
	if errorText != "" {
		s.Error = errorText
	}
	if loginSessionTerminal(s.Status) {
		s.browserPort = 0
	}
}

// CancelSession terminates an active login attempt.
func (m *BrowserLoginManager) CancelSession(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		if s.CancelFn != nil {
			s.CancelFn()
		}
		s.Status = "canceled"
		s.Message = "Đã hủy phiên đăng nhập."
		s.browserPort = 0
	}
	m.mu.Unlock()
	return ok
}

// SetBrowserPort binds the loopback-only CDP port to an active login session.
func (m *BrowserLoginManager) SetBrowserPort(id string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || loginSessionTerminal(s.Status) {
		return false
	}
	s.browserPort = port
	return true
}

// BrowserScreenshot captures the visible provider page for the authenticated viewer.
func (m *BrowserLoginManager) BrowserScreenshot(ctx context.Context, id string) ([]byte, int, int, error) {
	port, err := m.activeBrowserPort(id)
	if err != nil {
		return nil, 0, 0, err
	}
	return captureLoginBrowserScreenshot(ctx, port)
}

// BrowserInput forwards a bounded interaction to the active provider page.
func (m *BrowserLoginManager) BrowserInput(ctx context.Context, id string, input LoginBrowserInput) error {
	port, err := m.activeBrowserPort(id)
	if err != nil {
		return err
	}
	return dispatchLoginBrowserInput(ctx, port, input)
}

func (m *BrowserLoginManager) activeBrowserPort(id string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return 0, errors.New("không tìm thấy phiên đăng nhập")
	}
	if loginSessionTerminal(s.Status) || s.browserPort == 0 {
		return 0, errors.New("trình duyệt đăng nhập chưa sẵn sàng")
	}
	return s.browserPort, nil
}

func loginSessionTerminal(status string) bool {
	switch status {
	case "success", "failed", "canceled":
		return true
	default:
		return false
	}
}

// RunCaptureViaCDP opens a dedicated Chromium window with remote debugging,
// watches CDP cookies until the target cookie appears, and returns the captured session.

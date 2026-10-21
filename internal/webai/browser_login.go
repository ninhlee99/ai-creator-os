package webai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WebLoginTarget describes a target service to open and which cookie to wait for.
type WebLoginTarget struct {
	Platform    string
	Name        string
	StartURL    string
	CookieHost  string // substring match on cookie domain
	CookieName  string // primary cookie name
	Profile     string // subdirectory name under ~/.aicos/browser-profiles
	LandingHost string // required host where the user finishes logging in
}

// SupportedWebLoginTargets returns all browser-automatable login targets.
func SupportedWebLoginTargets() map[string]WebLoginTarget {
	return map[string]WebLoginTarget{
		PlatformChatGPT: {
			Platform:    PlatformChatGPT,
			Name:        "ChatGPT",
			StartURL:    "https://chatgpt.com/auth/login",
			CookieHost:  "chatgpt.com",
			CookieName:  "__Secure-next-auth.session-token",
			Profile:     "chatgpt",
			LandingHost: "chatgpt.com",
		},
		PlatformClaude: {
			Platform:    PlatformClaude,
			Name:        "Claude",
			StartURL:    "https://claude.ai/login",
			CookieHost:  "claude.ai",
			CookieName:  "sessionKey",
			Profile:     "claude",
			LandingHost: "claude.ai",
		},
		PlatformGemini: {
			Platform:    PlatformGemini,
			Name:        "Gemini",
			StartURL:    "https://gemini.google.com/app",
			CookieHost:  "google.com",
			CookieName:  "__Secure-1PSID",
			Profile:     "gemini",
			LandingHost: "gemini.google.com",
		},
		PlatformGrok: {
			Platform:    PlatformGrok,
			Name:        "Grok (xAI)",
			StartURL:    "https://grok.com/",
			CookieHost:  "grok.com",
			CookieName:  "sso",
			Profile:     "grok",
			LandingHost: "grok.com",
		},
		PlatformMuseAI: {
			Platform:    PlatformMuseAI,
			Name:        "Muse AI",
			StartURL:    "https://muse.ai/signin",
			CookieHost:  "muse.ai",
			CookieName:  "hatch_sess",
			Profile:     "muse",
			LandingHost: "muse.ai",
		},
		PlatformGoogleFlow: {
			Platform:    PlatformGoogleFlow,
			Name:        "Google Flow",
			StartURL:    "https://flow.google.com/",
			CookieHost:  "google.com",
			CookieName:  "__Secure-1PSID",
			Profile:     "googleflow",
			LandingHost: "flow.google.com",
		},
	}
}

// CapturedWebAuth holds the extracted credentials from the dedicated browser window.
type CapturedWebAuth struct {
	Platform     string
	SessionValue string
	CookieHeader string
	Email        string
	BrowserUsed  string
}

// ActiveLoginSession represents an ongoing browser login attempt.
type ActiveLoginSession struct {
	ID        string
	Platform  string
	Status    string // "opening", "waiting_for_user", "success", "failed", "canceled"
	Message   string
	Email     string
	Error     string
	CreatedAt time.Time
	ExpiresAt time.Time
	CancelFn  func()
	ctx       context.Context
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
	targets := SupportedWebLoginTargets()
	target, ok := targets[platform]
	if !ok {
		return nil, fmt.Errorf("nền tảng %q chưa được hỗ trợ đăng nhập tự động", platform)
	}

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	sessionID := fmt.Sprintf("login_%s_%d", platform, time.Now().UnixNano())
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
	}
	m.mu.Unlock()
	return ok
}

// RunCaptureViaCDP opens a dedicated Chromium window with remote debugging,
// watches CDP cookies until the target cookie appears, and returns the captured session.
func RunCaptureViaCDP(ctx context.Context, target WebLoginTarget, timeout time.Duration, updateStatus func(status, msg string)) (*CapturedWebAuth, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin, err := findChromiumBinary()
	if err != nil {
		return nil, err
	}

	// Mỗi lần mở là một profile sạch hoàn toàn mới để người dùng có thể đăng nhập nhiều tài khoản khác nhau (multi-account)
	dir, err := os.MkdirTemp("", fmt.Sprintf("aicos-login-%s-*", target.Platform))
	if err != nil {
		return nil, fmt.Errorf("không thể tạo profile đăng nhập tạm thời: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	clearChromiumSingletonLocks(dir)

	port, portHold, err := pickFreePort()
	if err != nil {
		return nil, err
	}

	// Release port immediately before starting browser
	_ = portHold.Close()

	if updateStatus != nil {
		updateStatus("opening", fmt.Sprintf("Mở %s (cổng gỡ lỗi %d)...", filepath.Base(bin), port))
	}

	cmd := exec.CommandContext(ctx, bin,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir="+dir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--new-window",
		target.StartURL,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không thể khởi chạy trình duyệt: %w", err)
	}
	defer terminateBrowser(cmd)

	var wsURL string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		wsURL, err = debuggerWSURL(port)
		if err == nil && wsURL != "" {
			break
		}
		time.Sleep(350 * time.Millisecond)
	}

	if updateStatus != nil {
		updateStatus("waiting_for_user", fmt.Sprintf("Trình duyệt đã mở. Vui lòng đăng nhập %s trên cửa sổ vừa xuất hiện...", target.Name))
	}

	var musePendingEmail string
	var landingSeenCount int

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Specialized handler for Muse AI: uses in-browser DOM & session state
		// to distinguish between OTP step, 2FA step, and successful login.
		if target.Platform == PlatformMuseAI {
			isLoggedIn, isWaitingOTP, isWaiting2FA, detEmail, _ := cdpProbeMuseAI(port)
			if detEmail != "" && cleanExtractedEmail(detEmail) != "" {
				musePendingEmail = cleanExtractedEmail(detEmail)
			}

			if isWaitingOTP {
				if updateStatus != nil {
					msg := "Đang đợi nhập mã OTP trên màn hình Muse AI..."
					if musePendingEmail != "" {
						msg = fmt.Sprintf("Mã OTP đã gửi tới %s. Vui lòng nhập mã để tiếp tục...", musePendingEmail)
					}
					updateStatus("waiting_for_user", msg)
				}
				time.Sleep(1000 * time.Millisecond)
				continue
			}

			if isWaiting2FA {
				if updateStatus != nil {
					updateStatus("waiting_for_user", "Đang đợi nhập mã xác thực 2FA (Authenticator/App) trên màn hình Muse AI...")
				}
				time.Sleep(1000 * time.Millisecond)
				continue
			}

			// Check session cookies directly via CDP
			cookies, gerr := cdpListCookies(wsURL)
			hasMuseSessionCookie := false
			hasHatchSessionCookie := false
			var sessionCookieVal, sessionCookieName string
			museSessionCookieRank := 0
			if gerr == nil && len(cookies) > 0 {
				for _, c := range cookies {
					d := strings.TrimPrefix(c.Domain, ".")
					isMuseCookie := d == "muse.ai" || strings.HasSuffix(d, ".muse.ai")
					if isMuseCookie {
						// STRICT: ps_l=0 and ps_n=0 are placeholders for unauthenticated / challenge state!
						if c.Value == "0" || c.Value == "1" || len(c.Value) < 3 {
							continue
						}
						if (c.Name == "hatch_sess" || c.Name == "hatch_native_auth_token" || c.Name == "c_user" || c.Name == "session" || c.Name == "auth_token" || c.Name == "token" || c.Name == "connect.sid") && len(c.Value) > 4 {
							hasMuseSessionCookie = true
							if c.Name == "hatch_sess" {
								hasHatchSessionCookie = true
							}
							rank := 3
							if c.Name == "hatch_sess" {
								rank = 6
							} else if c.Name == "hatch_native_auth_token" {
								rank = 4
							}
							if rank > museSessionCookieRank {
								sessionCookieVal = c.Value
								sessionCookieName = c.Name
								museSessionCookieRank = rank
							}
						} else if c.Name == "ps_l" && len(c.Value) > 5 {
							hasMuseSessionCookie = true
							if museSessionCookieRank < 5 {
								sessionCookieVal = c.Value
								sessionCookieName = c.Name
								museSessionCookieRank = 5
							}
						}
					}
				}
			}

			museLandingPage := cdpHasMuseLandingPage(port)

			// Capture only after Muse app state and a first-party session cookie both confirm login.
			if !isWaitingOTP && !isWaiting2FA && (isLoggedIn || hasHatchSessionCookie) && hasMuseSessionCookie && museLandingPage {
				confirmedEmail := detEmail
				if confirmedEmail == "" {
					confirmedEmail = musePendingEmail
				}
				if confirmedEmail == "" && gerr == nil {
					_, emailFromCookies := pickSessionCookieAndEmail(cookies, target)
					confirmedEmail = emailFromCookies
				}
				if confirmedEmail == "" {
					for attempt := 0; attempt < 5 && ctx.Err() == nil; attempt++ {
						if liveEmail := cdpExtractPageIdentity(port, target); liveEmail != "" {
							confirmedEmail = liveEmail
							break
						}
						if attempt < 4 {
							time.Sleep(500 * time.Millisecond)
						}
					}
				}
				confirmedEmail = cleanExtractedEmail(confirmedEmail)
				if confirmedEmail == "" {
					if updateStatus != nil {
						updateStatus("waiting_for_user", "Muse đã đăng nhập. Đang tự trích xuất email tài khoản; giữ cửa sổ Muse mở để hoàn tất.")
					}
					time.Sleep(time.Second)
					continue
				}

				if sessionCookieVal == "" && gerr == nil {
					sessionCookieVal, _ = pickSessionCookieAndEmail(cookies, target)
				}
				if sessionCookieVal != "" && sessionCookieVal != "0" && sessionCookieVal != "1" {
					hdr := cookieHeaderForHost(cookies, target.CookieHost)
					hdr = replaceCookiePair(hdr, sessionCookieName, sessionCookieVal)
					if !cookieHeaderHasPair(hdr, sessionCookieName, sessionCookieVal) {
						if updateStatus != nil {
							updateStatus("waiting_for_user", "Đã nhận diện đăng nhập Muse nhưng chưa ghép được cookie phiên; giữ cửa sổ mở và tải lại trang Muse.")
						}
						time.Sleep(time.Second)
						continue
					}
					auth := &CapturedWebAuth{
						Platform:     target.Platform,
						SessionValue: sessionCookieVal,
						CookieHeader: hdr,
						Email:        confirmedEmail,
						BrowserUsed:  filepath.Base(bin),
					}
					enrichAccountProfile(target, auth)
					return auth, nil
				}
			}

			// For Muse AI: always sleep and continue until genuinely logged in.
			if updateStatus != nil {
				msg := fmt.Sprintf("Trình duyệt đã mở. Vui lòng đăng nhập %s trên cửa sổ vừa xuất hiện...", target.Name)
				if gerr != nil {
					msg = "Chưa đọc được cookie từ cửa sổ Muse. Giữ cửa sổ mở và tải lại trang sau khi đăng nhập."
				} else if isLoggedIn && !hasMuseSessionCookie {
					msg = "Muse đã nhận diện tài khoản; chưa thấy cookie phiên trên domain muse.ai. Đang chờ trang hoàn tất đăng nhập..."
				} else if hasMuseSessionCookie && !museLandingPage {
					msg = "Đã thấy cookie phiên Muse; đang chờ quay về trang Muse..."
				} else if hasMuseSessionCookie && !isLoggedIn && !hasHatchSessionCookie {
					msg = "Đã thấy cookie Muse; đang chờ trang xác nhận đăng nhập hoàn tất..."
				}
				updateStatus("waiting_for_user", msg)
			}
			time.Sleep(1000 * time.Millisecond)
			continue
		}

		cookies, gerr := cdpListCookies(wsURL)
		if gerr == nil && len(cookies) > 0 {
			if val, email := pickSessionCookieAndEmail(cookies, target); val != "" {
				if cdpHasLandingHost(port, target.LandingHost) {
					landingSeenCount++
					if liveEmail := cdpExtractPageIdentity(port, target); liveEmail != "" {
						email = liveEmail
					}
					hdr := cookieHeaderForHost(cookies, target.CookieHost)
					auth := &CapturedWebAuth{
						Platform:     target.Platform,
						SessionValue: val,
						CookieHeader: hdr,
						Email:        email,
						BrowserUsed:  filepath.Base(bin),
					}
					enrichAccountProfile(target, auth)
					if auth.Email == "" && auth.SessionValue != "" {
						if liveEmail := cdpExtractPageIdentity(port, target); liveEmail != "" {
							auth.Email = liveEmail
						}
					}
					if auth.SessionValue != "" && (auth.Email != "" || landingSeenCount >= 2) {
						if auth.Email == "" {
							auth.Email = fmt.Sprintf("%s_user_%d@web.ai", target.Platform, time.Now().Unix()%100000)
						}
						return auth, nil
					}
					if updateStatus != nil {
						updateStatus("waiting_for_user", fmt.Sprintf("Đã có phiên kết nối. Vui lòng hoàn tất đăng nhập/chọn tài khoản %s...", target.Name))
					}
				}
			}
		}
		time.Sleep(1000 * time.Millisecond)
	}
}

func cdpHasLandingHost(port int, host string) bool {
	if host == "" {
		return true
	}
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false
	}
	for _, t := range tabs {
		if t.Type != "page" {
			continue
		}

		isMatch := strings.Contains(t.URL, host)
		if !isMatch && (host == "flow.google.com" || host == "labs.google" || host == "flow.google") {
			if strings.Contains(t.URL, "flow.google") || strings.Contains(t.URL, "labs.google") || strings.Contains(t.URL, "aitestkitchen.withgoogle.com") {
				isMatch = true
			}
		}
		if !isMatch {
			continue
		}

		// Đang ở màn hình chọn tài khoản Google hoặc trang login thì CHƯA được coi là xong!
		if (host == "gemini.google.com" || host == "flow.google.com" || host == "labs.google" || host == "flow.google") && (strings.Contains(t.URL, "accounts.google.com") || strings.Contains(t.URL, "AccountChooser")) {
			continue
		}
		if host == "chatgpt.com" && (strings.Contains(t.URL, "auth/login") || strings.Contains(t.URL, "auth0") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "claude.ai" && (strings.Contains(t.URL, "claude.ai/login") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "grok.com" && (strings.Contains(t.URL, "accounts.x.ai") || strings.Contains(t.URL, "twitter.com") || strings.Contains(t.URL, "x.com") || strings.Contains(t.URL, "accounts.google.com")) {
			continue
		}
		if host == "muse.ai" {
			u := strings.ToLower(t.URL)
			if strings.Contains(u, "/signin") || strings.Contains(u, "/signup") || strings.Contains(u, "/login") || strings.Contains(u, "/aymh") || strings.Contains(u, "auth.muse.ai") || strings.Contains(u, "accounts.google.com") || strings.Contains(u, "two_factor") || strings.Contains(u, "two-factor") || strings.Contains(u, "checkpoint") || strings.Contains(u, "challenge") || strings.Contains(u, "2fa") || strings.Contains(u, "mfa") {
				continue
			}
			return true
		}
		return true
	}
	return false
}

// cdpHasMuseLandingPage only accepts a first-party Muse app page. The
// generic landing-host check is intentionally substring-based for legacy
// providers; for Muse that also matches auth subdomains and redirect URLs.
func cdpHasMuseLandingPage(port int) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false
	}
	for _, tab := range tabs {
		if tab.Type != "page" {
			continue
		}
		u, err := url.Parse(tab.URL)
		if err != nil || u.Scheme != "https" {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host != "muse.ai" && host != "www.muse.ai" {
			continue
		}
		path := strings.ToLower(strings.TrimRight(u.EscapedPath(), "/"))
		isAuthPath := false
		for _, prefix := range []string{
			"/signin", "/sign-in", "/signup", "/sign-up", "/login", "/logout",
			"/auth", "/aymh", "/checkpoint", "/challenge", "/two_factor",
			"/two-factor", "/2fa", "/mfa",
		} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				isAuthPath = true
				break
			}
		}
		if !isAuthPath {
			return true
		}
	}
	return false
}

var (
	wizEmailRe       = regexp.MustCompile(`"oPEP7c"\s*:\s*"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})"`)
	ariaLabelEmailRe = regexp.MustCompile(`aria-label="[^"]*?\(([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})\)"`)
	htmlTagEmailRe   = regexp.MustCompile(`>([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})<`)
	generalEmailRe   = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4})`)
)

func cleanExtractedEmail(cand string) string {
	cand = strings.TrimSpace(cand)
	if isSystemOrStaticEmail(cand) {
		return ""
	}
	if m := generalEmailRe.FindString(cand); m != "" {
		if !isSystemOrStaticEmail(m) {
			return m
		}
	}
	if strings.Contains(cand, "@") && !isSystemOrStaticEmail(cand) {
		return cand
	}
	return ""
}

func isSystemOrStaticEmail(email string) bool {
	low := strings.ToLower(email)
	return strings.HasSuffix(low, "@google.com") ||
		strings.Contains(low, "example.com") ||
		strings.Contains(low, "w3.org") ||
		strings.Contains(low, "schema.org") ||
		strings.Contains(low, "sentry") ||
		strings.Contains(low, "mixpanel") ||
		strings.Contains(low, "cloudflare")
}

func parseGeminiAccountEmail(html string) string {
	if m := wizEmailRe.FindStringSubmatch(html); len(m) > 1 {
		cand := strings.TrimSpace(m[1])
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	if m := ariaLabelEmailRe.FindStringSubmatch(html); len(m) > 1 {
		cand := strings.TrimSpace(m[1])
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	for _, m := range htmlTagEmailRe.FindAllStringSubmatch(html, -1) {
		if len(m) > 1 {
			cand := strings.TrimSpace(m[1])
			if !isSystemOrStaticEmail(cand) {
				return cand
			}
		}
	}
	for _, cand := range generalEmailRe.FindAllString(html, -1) {
		cand = strings.TrimSpace(cand)
		if !isSystemOrStaticEmail(cand) {
			return cand
		}
	}
	return ""
}

func cdpExtractPageIdentity(port int, target WebLoginTarget) string {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type                 string `json:"type"`
		URL                  string `json:"url"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return ""
	}
	if target.Platform == PlatformMuseAI {
		var museTargets []string
		var identityProviderTargets []string
		for _, t := range tabs {
			if t.Type != "page" || t.WebSocketDebuggerURL == "" {
				continue
			}
			u, err := url.Parse(t.URL)
			if err != nil {
				continue
			}
			host := strings.ToLower(u.Hostname())
			if host == "muse.ai" || host == "www.muse.ai" || strings.HasSuffix(host, ".muse.ai") {
				museTargets = append(museTargets, t.WebSocketDebuggerURL)
			} else if isMuseIdentityProviderHost(host) {
				identityProviderTargets = append(identityProviderTargets, t.WebSocketDebuggerURL)
			}
		}
		return cdpExtractMuseEmailFromTargets(append(museTargets, identityProviderTargets...))
	}

	var targetWS string
	for _, t := range tabs {
		if t.Type != "page" || t.WebSocketDebuggerURL == "" {
			continue
		}
		isMatch := strings.Contains(t.URL, target.LandingHost)
		if !isMatch && (target.Platform == PlatformGoogleFlow || target.LandingHost == "flow.google.com" || target.LandingHost == "labs.google") {
			if strings.Contains(t.URL, "flow.google") || strings.Contains(t.URL, "labs.google") || strings.Contains(t.URL, "aitestkitchen.withgoogle.com") {
				isMatch = true
			}
		}
		if !isMatch {
			continue
		}
		if (target.Platform == PlatformGemini || target.Platform == PlatformGoogleFlow) && (strings.Contains(t.URL, "accounts.google.com") || strings.Contains(t.URL, "AccountChooser")) {
			continue
		}
		if target.Platform == PlatformChatGPT && strings.Contains(t.URL, "auth/login") {
			continue
		}
		if target.Platform == PlatformClaude && strings.Contains(t.URL, "claude.ai/login") {
			continue
		}
		if target.Platform == PlatformMuseAI && strings.Contains(t.URL, "accounts.google.com") {
			continue
		}
		targetWS = t.WebSocketDebuggerURL
		break
	}
	if targetWS == "" {
		return ""
	}

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(targetWS, nil)
	if err != nil {
		return ""
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	var script string
	switch target.Platform {
	case PlatformGemini, PlatformGoogleFlow:
		script = `(() => {
			try {
				if (window.WIZ_global_data && typeof window.WIZ_global_data.oPEP7c === 'string') {
					const em = window.WIZ_global_data.oPEP7c.trim();
					if (em.includes('@') && !em.endsWith('@google.com')) return em;
				}
			} catch(e) {}
			try {
				const els = document.querySelectorAll('[aria-label*="@"], [alt*="@"], [data-email]');
				for (const el of els) {
					const txt = el.getAttribute('aria-label') || el.getAttribute('alt') || el.getAttribute('data-email') || '';
					const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m && !m[1].endsWith('@google.com')) return m[1];
				}
			} catch(e) {}
			try {
				for (let i = 0; i < localStorage.length; i++) {
					const v = localStorage.getItem(localStorage.key(i)) || '';
					const m = v.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m && !m[1].endsWith('@google.com')) return m[1];
				}
			} catch(e) {}
			return "";
		})()`

	case PlatformChatGPT:
		script = `(() => {
			try {
				if (window.__NEXT_DATA__ && window.__NEXT_DATA__.props) {
					const str = JSON.stringify(window.__NEXT_DATA__.props);
					const m = str.match(/"email"\s*:\s*"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})"/);
					if (m) return m[1];
				}
			} catch(e) {}
			try {
				const els = document.querySelectorAll('[aria-label*="@"]');
				for (const el of els) {
					const m = el.getAttribute('aria-label').match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m) return m[1];
				}
			} catch(e) {}
			return "";
		})()`

	case PlatformClaude:
		script = `(() => {
			try {
				const els = document.querySelectorAll('[aria-label*="@"], [data-testid*="user"]');
				for (const el of els) {
					const txt = el.getAttribute('aria-label') || el.textContent;
					const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
					if (m) return m[1];
				}
			} catch(e) {}
			return "";
		})()`

	case PlatformGrok:
		script = `(async () => {
			const re = /([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.(?:com\.vn|edu\.vn|gov\.vn|net\.vn|org\.vn|com|net|org|edu|gov|io|ai|vn|me|co|app|dev|xyz|info|[a-z]{2,4}))/i;
			try {
				const footerBtn = document.querySelector('[data-sidebar="footer"] button, button[aria-label*="user"], [class*="profile"]');
				if (footerBtn) {
					footerBtn.click();
					await new Promise(r => setTimeout(r, 200));
				}
				const menu = document.querySelector('[role="menu"], [data-radix-popper-content-wrapper]');
				if (menu) {
					for (const el of menu.querySelectorAll('*')) {
						const txt = el.textContent ? el.textContent.trim() : '';
						const m = txt.match(re);
						if (m && !m[1].includes('example') && !m[1].includes('sentry')) {
							document.body.click();
							return m[1];
						}
					}
					const m2 = menu.textContent.match(re);
					document.body.click();
					if (m2 && !m2[1].includes('example') && !m2[1].includes('sentry')) return m2[1];
				}
				document.body.click();
			} catch(e) {}
			try {
				const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
				let node;
				while (node = walker.nextNode()) {
					const val = node.nodeValue ? node.nodeValue.trim() : '';
					const m = val.match(re);
					if (m && !m[1].includes('example') && !m[1].includes('sentry') && !m[1].includes('w3.org') && !m[1].includes('schema.org')) {
						return m[1];
					}
				}
			} catch(e) {}
			try {
				for (let i = 0; i < localStorage.length; i++) {
					const val = localStorage.getItem(localStorage.key(i));
					if (val && val.includes('@')) {
						const m = val.match(re);
						if (m && !m[1].includes('example') && !m[1].includes('sentry') && !m[1].includes('w3.org')) return m[1];
					}
				}
			} catch(e) {}
			return "";
		})()`

	}

	if script == "" {
		return ""
	}

	reqID := cdpReqID.Add(1)
	req := map[string]any{
		"id":     reqID,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    script,
			"returnByValue": true,
			"awaitPromise":  true,
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		return ""
	}

	for {
		var msg struct {
			ID     int64 `json:"id"`
			Result struct {
				Result struct {
					Type  string `json:"type"`
					Value any    `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return ""
		}
		if msg.ID != reqID {
			continue
		}
		if str, ok := msg.Result.Result.Value.(string); ok {
			cand := strings.TrimSpace(str)
			if clean := cleanExtractedEmail(cand); clean != "" {
				return clean
			}
			if !isSystemOrStaticEmail(cand) && cand != "" {
				if strings.Contains(cand, "@") {
					return cand
				}
				if target.Platform == PlatformMuseAI {
					return ""
				}
				return cand + "@" + target.Platform
			}
		}
		break
	}
	return ""
}

func cdpExtractMuseEmailFromTargets(targets []string) string {
	script := `(async () => {
		const emailFromText = value => {
			if (typeof value !== 'string') return '';
			const matches = value.match(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g) || [];
			for (const candidate of matches) {
				const email = candidate.trim();
				const low = email.toLowerCase();
				if (low.includes('sentry') || low.includes('example.') || low.includes('w3.org') || low.includes('schema.org') || low.includes('facebook.com') || low.includes('meta.com') || low.includes('support@')) continue;
				return email;
			}
			return '';
		};
		const emailFromObject = (value, depth = 0) => {
			if (!value || typeof value !== 'object' || depth > 6) return '';
			for (const [key, child] of Object.entries(value)) {
				if (/^email(address)?$/i.test(key)) {
					const email = emailFromText(child);
					if (email) return email;
				}
				if (child && typeof child === 'object') {
					const email = emailFromObject(child, depth + 1);
					if (email) return email;
				}
			}
			return '';
		};
		try {
			if ((location.hostname === 'muse.ai' || location.hostname.endsWith('.muse.ai')) &&
				(!window.__aicosMuseAccountLookupAt || Date.now() - window.__aicosMuseAccountLookupAt > 10000)) {
				window.__aicosMuseAccountLookupAt = Date.now();
				const response = await fetch('/api/account', { credentials: 'include' });
				if (response.ok) {
					const payload = await response.json();
					const email = emailFromObject(payload);
					if (email) return email;
				}
			}
		} catch(e) {}
		try {
			for (const key of ['user', '__INITIAL_STATE__', '__NEXT_DATA__', '__NUXT__', '__APOLLO_STATE__']) {
				const email = emailFromObject(window[key]);
				if (email) return email;
			}
		} catch(e) {}
		try {
			for (const storage of [localStorage, sessionStorage]) {
				for (let i = 0; i < storage.length; i++) {
					const value = storage.getItem(storage.key(i));
					if (!value || !value.includes('@')) continue;
					try {
						const email = emailFromObject(JSON.parse(value));
						if (email) return email;
					} catch(e) {}
					const keyed = value.match(/"(?:\$?email|emailAddress)"\s*:\s*"([^"]+)"/i);
					if (keyed) {
						const email = emailFromText(keyed[1]);
						if (email) return email;
					}
				}
			}
		} catch(e) {}
		try {
			const selectors = '[data-email], [aria-label*="@"], [title*="@"], a[href^="mailto:"], a[href*="account"], [class*="profile"], [class*="avatar"], button[aria-label*="user" i], [data-testid*="user"], [data-testid*="account"]';
			for (const el of document.querySelectorAll(selectors)) {
				for (const value of [el.getAttribute('data-email'), el.getAttribute('aria-label'), el.getAttribute('title'), el.getAttribute('href'), el.textContent]) {
					const email = emailFromText(value || '');
					if (email) return email;
				}
			}
		} catch(e) {}
		try {
			for (const input of document.querySelectorAll('input[type="email"], input[name*="email" i], input[autocomplete="email"]')) {
				const email = emailFromText(input.value || '');
				if (email) return email;
			}
		} catch(e) {}
		try {
			const email = emailFromText(document.body ? document.body.innerText : '');
			if (email) return email;
		} catch(e) {}
		try {
			if (!window.__aicosMuseProfileOpened) {
				const trigger = document.querySelector('button[aria-label*="account" i], button[aria-label*="profile" i], button[aria-label*="user" i], button[class*="avatar" i], [data-testid*="user-menu"], [data-testid*="account-menu"]');
				if (trigger) {
					window.__aicosMuseProfileOpened = true;
					trigger.click();
					await new Promise(resolve => setTimeout(resolve, 300));
					for (const el of document.querySelectorAll('[role="menu"], [role="dialog"], [data-radix-popper-content-wrapper], [class*="profile"], [class*="account"], [data-testid*="user"], [data-testid*="account"]')) {
						const email = emailFromText(el.innerText || el.textContent || '');
						if (email) return email;
					}
					const email = emailFromText(document.body ? document.body.innerText : '');
					if (email) return email;
				}
			}
		} catch(e) {}
		return '';
	})()`
	for _, targetWS := range targets {
		dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
		conn, _, err := dialer.Dial(targetWS, nil)
		if err != nil {
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		reqID := cdpReqID.Add(1)
		req := map[string]any{
			"id":     reqID,
			"method": "Runtime.evaluate",
			"params": map[string]any{"expression": script, "returnByValue": true, "awaitPromise": true},
		}
		if err := conn.WriteJSON(req); err != nil {
			_ = conn.Close()
			continue
		}
		for {
			var msg struct {
				ID     int64 `json:"id"`
				Result struct {
					Result struct {
						Value any `json:"value"`
					} `json:"result"`
				} `json:"result"`
			}
			if err := conn.ReadJSON(&msg); err != nil {
				break
			}
			if msg.ID != reqID {
				continue
			}
			if value, ok := msg.Result.Result.Value.(string); ok {
				if email := cleanExtractedEmail(value); email != "" {
					_ = conn.Close()
					return email
				}
			}
			break
		}
		_ = conn.Close()
	}
	return ""
}

func isMuseIdentityProviderHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, root := range []string{"meta.com", "meta.ai", "facebook.com", "instagram.com", "google.com"} {
		if host == root || strings.HasSuffix(host, "."+root) {
			return true
		}
	}
	return false
}

// cdpProbeMuseAI evaluates the Muse AI page state via CDP to distinguish between OTP step, 2FA step, and logged-in state.
func cdpProbeMuseAI(port int) (isLoggedIn bool, isWaitingOTP bool, isWaiting2FA bool, detectedEmail string, err error) {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return false, false, false, "", err
	}
	defer resp.Body.Close()

	var tabs []struct {
		Type                 string `json:"type"`
		URL                  string `json:"url"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return false, false, false, "", err
	}

	var targetWS, providerWS string
	for _, t := range tabs {
		if t.Type != "page" || t.WebSocketDebuggerURL == "" {
			continue
		}
		u, err := url.Parse(t.URL)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "muse.ai" || host == "www.muse.ai" || strings.HasSuffix(host, ".muse.ai") {
			targetWS = t.WebSocketDebuggerURL
			break
		}
		if providerWS == "" && isMuseIdentityProviderHost(host) {
			providerWS = t.WebSocketDebuggerURL
		}
	}
	if targetWS == "" {
		targetWS = providerWS
	}
	if targetWS == "" {
		return false, false, false, "", fmt.Errorf("không tìm thấy tab muse.ai")
	}

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(targetWS, nil)
	if err != nil {
		return false, false, false, "", err
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	script := `(async () => {
		try {
			const bodyText = (document.body && document.body.innerText) ? document.body.innerText : '';
			const bodyLower = bodyText.toLowerCase();
			const path = (location.pathname || '').toLowerCase();
			const host = (location.host || '').toLowerCase();
			const isAuthPage = path.includes('/signin') || path.includes('/signup') || path.includes('/login') || path.includes('/aymh') || host.includes('auth.muse.ai') || host.includes('facebook.com') || path.includes('two_factor') || path.includes('two-factor') || path.includes('2fa') || path.includes('checkpoint') || path.includes('challenge') || path.includes('mfa');

			// Check for 2FA specific fields and keywords
			const twoFAInput = document.querySelector(
				'input[name*="2fa" i], input[placeholder*="2fa" i], input[placeholder*="authenticator" i], input[placeholder*="security" i], input[id*="approvals"], input[name*="approvals"], input[name*="totp" i]'
			);
			const has2FAKeyword = bodyLower.includes('two-factor') || bodyLower.includes('two factor') || bodyLower.includes('2-factor') || bodyLower.includes('authenticator') || bodyLower.includes('authentication app') || bodyLower.includes('security code') || bodyLower.includes('login approval') || bodyLower.includes('check your other device') || bodyLower.includes('xác thực 2 yếu tố') || bodyLower.includes('hai bước') || bodyLower.includes('mã 2fa');

			// Check for general OTP code fields and keywords
			const otpInput = document.querySelector(
				'input[autocomplete*="one-time-code"], input[name*="code" i], input[placeholder*="code" i], input[maxlength="6"], input[maxlength="8"], input[inputmode="numeric"]'
			);
			const hasCodeKeyword = bodyLower.includes('verification code') || bodyLower.includes('enter the code') || bodyLower.includes('enter code') || bodyLower.includes('we sent a code') || bodyLower.includes('sent a 6-digit') || bodyLower.includes('check your email') || bodyLower.includes('mã xác nhận');

			let candEmail = '';
			const emailMatches = bodyText.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/g);
			if (emailMatches) {
				for (const em of emailMatches) {
					const emLower = em.toLowerCase();
					if (!emLower.includes('sentry') && !emLower.includes('example') && !emLower.includes('w3.org') && !emLower.includes('facebook.com') && !emLower.includes('meta.com') && !emLower.includes('support@')) {
						candEmail = em;
						break;
					}
				}
			}
			if (!candEmail) {
				const emailInp = document.querySelector('input[type="email"], input[name*="email" i]');
				if (emailInp && emailInp.value && emailInp.value.includes('@')) {
					candEmail = emailInp.value.trim();
				}
			}
			if (!candEmail) {
				try {
					for (let i = 0; i < localStorage.length; i++) {
						const val = localStorage.getItem(localStorage.key(i));
						if (val && val.includes('@')) {
							const m = val.match(/"\$?email"\s*:\s*"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})"/i) ||
							          val.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
							if (m && !m[1].includes('sentry') && !m[1].includes('example') && !m[1].includes('w3.org') && !m[1].includes('facebook.com')) {
								candEmail = m[1];
								break;
							}
						}
					}
				} catch(e) {}
			}
			if (!candEmail) {
				try {
					const els = document.querySelectorAll('[aria-label*="@"], [title*="@"], [data-email*="@"]');
					for (const el of els) {
						const txt = el.getAttribute('data-email') || el.getAttribute('aria-label') || el.getAttribute('title') || '';
						const m = txt.match(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/);
						if (m && !m[1].includes('sentry') && !m[1].includes('example')) {
							candEmail = m[1];
							break;
						}
					}
				} catch(e) {}
			}

			// 1. Detect 2FA waiting state first
			if (twoFAInput || (has2FAKeyword && (twoFAInput || otpInput || bodyLower.includes('submit') || bodyLower.includes('continue') || bodyLower.includes('verify') || bodyLower.includes('enter') || bodyLower.includes('confirm')))) {
				return JSON.stringify({ state: "waiting_2fa", email: candEmail });
			}

			// 2. Detect OTP waiting state
			if (otpInput || (hasCodeKeyword && (bodyLower.includes('resend') || bodyLower.includes('verify') || bodyLower.includes('submit') || bodyLower.includes('continue')))) {
				return JSON.stringify({ state: "waiting_otp", email: candEmail });
			}

			// Auth/checkpoint URLs are never successful landing pages, even if a
			// SPA fallback or public endpoint responds with HTTP 200.
				if (isAuthPage || path.includes('/checkpoint') || path.includes('/two-factor') || path.includes('/2fa')) {
				return JSON.stringify({ state: "not_logged_in", email: candEmail });
			}

			// Muse serves its public SPA HTML at /api/account, so HTTP 200 there
			// cannot establish identity. Use authenticated app UI markers instead.
			// 3. Check for explicit sign-out / log-out links
			const signoutLink = document.querySelector('a[href*="/signout"], a[href*="/logout"], button[data-testid*="logout"], [data-action*="logout"], button[aria-label*="log out" i], button[aria-label*="sign out" i]');
			if (signoutLink && !otpInput && !twoFAInput && !has2FAKeyword) {
				return JSON.stringify({ state: "logged_in", email: candEmail });
			}

			// 4. Check user avatar/profile or dashboard navigation outside auth pages
			const userProfileElement = document.querySelector('[class*="user-menu"], [class*="account-menu"], [data-testid*="user-menu"], [data-testid*="profile"], [data-testid*="account"], a[href*="/account"], a[href*="/profile"], button[aria-label*="account" i], button[aria-label*="profile" i], [role="button"][aria-label*="user" i], button[class*="avatar"], a[class*="avatar"], [role="button"][class*="avatar"]');
			const appNavigation = document.querySelector('a[href*="/videos"], a[href*="/upload"], [data-testid*="upload"], button[class*="upload"]');
			const loginControl = Array.from(document.querySelectorAll('button, a')).some(el => /^(log in|sign in)$/i.test((el.innerText || '').trim()));
			if (loginControl && !signoutLink) {
				return JSON.stringify({ state: "not_logged_in", email: candEmail });
			}
			if (!otpInput && !twoFAInput && !has2FAKeyword && (userProfileElement || appNavigation || path.includes('/dashboard'))) {
				return JSON.stringify({ state: "logged_in", email: candEmail });
			}

			return JSON.stringify({ state: "not_logged_in", email: candEmail });
		} catch(e) {
			return JSON.stringify({ state: "error", error: e.toString() });
		}
	})()`

	reqID := cdpReqID.Add(1)
	req := map[string]any{
		"id":     reqID,
		"method": "Runtime.evaluate",
		"params": map[string]any{
			"expression":    script,
			"returnByValue": true,
			"awaitPromise":  true,
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		return false, false, false, "", err
	}

	for {
		var msg struct {
			ID     int64 `json:"id"`
			Result struct {
				Result struct {
					Type  string `json:"type"`
					Value any    `json:"value"`
				} `json:"result"`
			} `json:"result"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return false, false, false, "", err
		}
		if msg.ID != reqID {
			continue
		}
		if str, ok := msg.Result.Result.Value.(string); ok {
			var res struct {
				State string `json:"state"`
				Email string `json:"email"`
			}
			if err := json.Unmarshal([]byte(str), &res); err == nil {
				cleanEm := cleanExtractedEmail(res.Email)
				if res.State == "waiting_2fa" {
					return false, false, true, cleanEm, nil
				}
				if res.State == "waiting_otp" {
					return false, true, false, cleanEm, nil
				}
				if res.State == "logged_in" {
					return true, false, false, cleanEm, nil
				}
				return false, false, false, cleanEm, nil
			}
		}
		break
	}
	return false, false, false, "", nil
}

func enrichAccountProfile(target WebLoginTarget, auth *CapturedWebAuth) {
	if auth == nil || auth.CookieHeader == "" {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}

	switch target.Platform {
	case PlatformChatGPT:
		req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/api/auth/session", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", auth.CookieHeader)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var data struct {
						User struct {
							Email string `json:"email"`
							Name  string `json:"name"`
						} `json:"user"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
						if data.User.Email != "" {
							auth.Email = strings.TrimSpace(data.User.Email)
						}
					}
				}
			}
		}

	case PlatformClaude:
		cookie := auth.CookieHeader
		if !strings.Contains(cookie, "sessionKey=") && auth.SessionValue != "" {
			cookie = "sessionKey=" + auth.SessionValue + "; " + cookie
		}
		req, err := http.NewRequest(http.MethodGet, "https://claude.ai/api/account", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("Origin", "https://claude.ai")
			req.Header.Set("Referer", "https://claude.ai/")
			req.Header.Set("anthropic-client-platform", "web_claude_ai")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var data struct {
						Email        string `json:"email"`
						EmailAddress string `json:"email_address"`
						Account      struct {
							Email string `json:"email"`
						} `json:"account"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
						em := data.Email
						if em == "" {
							em = data.EmailAddress
						}
						if em == "" {
							em = data.Account.Email
						}
						if em != "" {
							auth.Email = strings.TrimSpace(em)
						}
					}
				}
			}
		}

	case PlatformGemini, PlatformGoogleFlow:
		cookie := auth.CookieHeader
		if !strings.Contains(cookie, "__Secure-1PSID=") && auth.SessionValue != "" {
			cookie = "__Secure-1PSID=" + auth.SessionValue + "; " + cookie
		}
		probeURLs := []string{"https://gemini.google.com/app", "https://myaccount.google.com/"}
		if target.Platform == PlatformGoogleFlow {
			probeURLs = append([]string{"https://flow.google.com/", "https://labs.google/flow"}, probeURLs...)
		}
		for _, targetURL := range probeURLs {
			if auth.Email != "" && !isSystemOrStaticEmail(auth.Email) {
				break
			}
			req, err := http.NewRequest(http.MethodGet, targetURL, nil)
			if err != nil {
				continue
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", cookie)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			if resp, err := client.Do(req); err == nil {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					html := string(body)
					if email := parseGeminiAccountEmail(html); email != "" {
						auth.Email = email
						break
					}
				}
			}
		}

	case PlatformGrok:
		req, err := http.NewRequest(http.MethodGet, "https://grok.com/rest/auth/user", nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
			req.Header.Set("Cookie", auth.CookieHeader)
			req.Header.Set("Accept", "application/json")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var data struct {
						Email    string `json:"email"`
						Username string `json:"username"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
						if data.Email != "" && !isSystemOrStaticEmail(data.Email) {
							auth.Email = strings.TrimSpace(data.Email)
						} else if data.Username != "" {
							auth.Email = data.Username + "@x.ai"
						}
					}
				}
			}
		}

	}
}

func aicosProfileDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".aicos", "browser-profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func pickFreePort() (int, net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	return ln.Addr().(*net.TCPAddr).Port, ln, nil
}

func debuggerWSURL(port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var meta struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", err
	}
	if meta.WS == "" {
		return "", fmt.Errorf("webSocketDebuggerUrl rỗng")
	}
	return meta.WS, nil
}

var cdpReqID atomic.Int64

type cdpCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

func cdpListCookies(wsURL string) ([]cdpCookie, error) {
	cookies, err := cdpCallCookies(wsURL, "Storage.getCookies")
	if err == nil && len(cookies) > 0 {
		return cookies, nil
	}
	alt, err2 := cdpCallCookies(wsURL, "Network.getAllCookies")
	if err2 == nil && len(alt) > 0 {
		return alt, nil
	}
	if err != nil {
		return nil, err
	}
	return cookies, err2
}

func cdpCallCookies(wsURL, method string) ([]cdpCookie, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))

	id := cdpReqID.Add(1)
	req := map[string]any{"id": id, "method": method}
	if err := conn.WriteJSON(req); err != nil {
		return nil, err
	}

	for {
		var msg struct {
			ID     int64 `json:"id"`
			Result struct {
				Cookies []cdpCookie `json:"cookies"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, err
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("cdp %s: %s", method, msg.Error.Message)
		}
		return msg.Result.Cookies, nil
	}
}

func extractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.RawStdEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}
	for _, key := range []string{"email", "user_email", "preferred_username"} {
		if val, ok := data[key].(string); ok {
			val = strings.TrimSpace(val)
			if strings.Contains(val, "@") && !isSystemOrStaticEmail(val) {
				return val
			}
		}
	}
	return ""
}

func pickSessionCookieAndEmail(cookies []cdpCookie, target WebLoginTarget) (string, string) {
	hostOK := func(domain string) bool {
		if target.CookieHost == "" {
			return true
		}
		d := strings.TrimPrefix(domain, ".")
		if target.Platform == PlatformMuseAI {
			return d == "muse.ai" || strings.HasSuffix(d, ".muse.ai")
		}
		matched := strings.Contains(d, target.CookieHost) || strings.Contains(target.CookieHost, d)
		if !matched && (target.Platform == PlatformGoogleFlow || target.CookieHost == "google.com") && (strings.Contains(d, "google.com") || strings.Contains(d, "google") || strings.Contains(d, "withgoogle.com")) {
			matched = true
		}
		return matched
	}

	var sessionVal string
	var email string
	var providerEmail string

	// Look for exact cookie
	for _, c := range cookies {
		inTargetDomain := hostOK(c.Domain)
		isMuseProviderCookie := target.Platform == PlatformMuseAI && isMuseIdentityProviderHost(strings.TrimPrefix(c.Domain, "."))
		if !inTargetDomain && !isMuseProviderCookie {
			continue
		}

		// Read identity hints from Muse and its login providers. Provider cookies
		// are inspected only for an email; cookie persistence remains first-party.
		valUnesc, _ := url.QueryUnescape(c.Value)
		var cookieEmail string
		if strings.Contains(strings.ToLower(c.Name), "email") || strings.Contains(strings.ToLower(c.Name), "user") || isMuseProviderCookie {
			for _, value := range []string{c.Value, valUnesc} {
				if m := generalEmailRe.FindString(value); m != "" && !isSystemOrStaticEmail(m) {
					cookieEmail = m
					break
				}
			}
		}
		if cookieEmail == "" && strings.Count(c.Value, ".") == 2 {
			cookieEmail = extractEmailFromJWT(c.Value)
		}
		if cookieEmail != "" {
			if inTargetDomain && email == "" {
				email = cookieEmail
			} else if isMuseProviderCookie && providerEmail == "" {
				providerEmail = cookieEmail
			}
		}
		if !inTargetDomain {
			continue
		}

		if c.Name == target.CookieName && c.Value != "" {
			if target.Platform == PlatformMuseAI && (c.Value == "0" || c.Value == "1" || len(c.Value) < 3) {
				// ps_l=0 or ps_l=1 are placeholders for unauthenticated / challenge state
			} else {
				sessionVal = c.Value
			}
		}

		// Grok fallback cookies
		if target.Platform == PlatformGrok && sessionVal == "" {
			if (c.Name == "sso" || c.Name == "sso-rw" || c.Name == "auth_token") && c.Value != "" {
				sessionVal = c.Value
			}
		}

		// Google Flow fallback cookies
		if target.Platform == PlatformGoogleFlow && sessionVal == "" {
			if (c.Name == "__Secure-1PSID" || c.Name == "__Secure-3PSID" || c.Name == "SID" || c.Name == "SSID" || c.Name == "SAPISID" || c.Name == "__Secure-1PAPISID" || c.Name == "__Secure-1PSIDTS" || c.Name == "__Secure-3PSIDTS" || c.Name == "__session" || c.Name == "authToken" || c.Name == "auth_token" || c.Name == "NID") && c.Value != "" {
				sessionVal = c.Value
			}
		}

		// Muse AI session cookies stay scoped to first-party Muse domains.
		if target.Platform == PlatformMuseAI && sessionVal == "" {
			d := strings.TrimPrefix(c.Domain, ".")
			if d == "muse.ai" || strings.HasSuffix(d, ".muse.ai") {
				if c.Value == "0" || c.Value == "1" || len(c.Value) < 3 {
					continue
				}
				if (c.Name == "hatch_sess" || c.Name == "hatch_native_auth_token" || c.Name == "c_user" || c.Name == "session" || c.Name == "auth_token" || c.Name == "token" || c.Name == "connect.sid") && len(c.Value) > 4 {
					sessionVal = c.Value
				} else if c.Name == "ps_l" && len(c.Value) > 5 {
					sessionVal = c.Value
				}
			}
		}
	}
	if target.Platform == PlatformMuseAI && email == "" {
		email = providerEmail
	}

	// For Grok: anonymous users do not have x-userid or htjs_user_id
	if target.Platform == PlatformGrok {
		hasUserCookie := false
		for _, c := range cookies {
			if (c.Name == "x-userid" || c.Name == "htjs_user_id") && c.Value != "" {
				hasUserCookie = true
				break
			}
		}
		if !hasUserCookie && email == "" {
			sessionVal = ""
		}
	}

	// Chunked next-auth session token (.0 + .1 + .2 ...) for ChatGPT
	if sessionVal == "" && strings.Contains(target.CookieName, "session-token") {
		var hdr []string
		for i := 0; i < 8; i++ {
			want := fmt.Sprintf("%s.%d", target.CookieName, i)
			for _, c := range cookies {
				if !hostOK(c.Domain) {
					continue
				}
				if c.Name == want && c.Value != "" {
					hdr = append(hdr, want+"="+c.Value)
					break
				}
			}
		}
		if len(hdr) > 0 {
			sessionVal = strings.Join(hdr, "; ")
		}
	}

	return sessionVal, email
}

func cookieHeaderForHost(cookies []cdpCookie, hostSubstr string) string {
	var parts []string
	seen := map[string]bool{}
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if hostSubstr != "" {
			d := strings.TrimPrefix(c.Domain, ".")
			matched := false
			if hostSubstr == "muse.ai" {
				matched = d == "muse.ai" || strings.HasSuffix(d, ".muse.ai")
			} else {
				matched = strings.Contains(d, hostSubstr) || strings.Contains(hostSubstr, d)
			}
			if !matched && (hostSubstr == "google.com" || hostSubstr == "labs.google" || hostSubstr == "flow.google.com") {
				if strings.Contains(d, "google.com") || strings.Contains(d, "google") || strings.Contains(d, "withgoogle.com") {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func replaceCookiePair(header, name, value string) string {
	if name == "" || value == "" {
		return header
	}
	parts := make([]string, 0, strings.Count(header, ";")+1)
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		cookieName, _, ok := strings.Cut(part, "=")
		if ok && cookieName != name {
			parts = append(parts, part)
		}
	}
	parts = append(parts, name+"="+value)
	return strings.Join(parts, "; ")
}

func cookieHeaderHasPair(header, name, value string) bool {
	for _, part := range strings.Split(header, ";") {
		cookieName, cookieValue, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && cookieName == name && cookieValue == value {
			return true
		}
	}
	return false
}

func clearChromiumSingletonLocks(dir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func terminateBrowser(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func findChromiumBinary() (string, error) {
	if env := strings.TrimSpace(os.Getenv("AICOS_BROWSER_BINARY")); env != "" {
		st, err := os.Stat(env)
		if err == nil && !st.IsDir() {
			return env, nil
		}
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Volumes/Macintosh HD/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
	case "windows":
		candidates = append(candidates,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		)
	default:
		candidates = append(candidates,
			"/usr/bin/google-chrome",
			"/usr/bin/microsoft-edge",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		)
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	for _, name := range []string{"msedge", "microsoft-edge", "google-chrome", "chromium", "chromium-browser", "brave"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("không tìm thấy trình duyệt Edge hoặc Chrome trên máy để mở giao diện đăng nhập")
}

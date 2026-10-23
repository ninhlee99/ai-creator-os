package webai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const webSessionProbeUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type webSessionHTTPStatusError struct {
	statusCode int
}

func (e *webSessionHTTPStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.statusCode) }

// ValidateWebAccountSession confirms that a saved web cookie still authenticates
// and is associated with a real account email.
func ValidateWebAccountSession(ctx context.Context, platform, sessionKey, cookie, email string) error {
	return ValidateWebAccountSessionWithCookieUpdate(ctx, platform, sessionKey, cookie, email, nil)
}

// ValidateWebAccountSessionWithCookieUpdate reports cookies rotated by the
// provider probe so callers can persist them against the stable account ID.
func ValidateWebAccountSessionWithCookieUpdate(ctx context.Context, platform, sessionKey, cookie, email string, onCookieUpdate func(string) error) error {
	return validateWebAccountSession(ctx, platform, sessionKey, cookie, nil, email, nil, onCookieUpdate)
}

// ValidateWebAccountSessionWithCookieSnapshot preserves imported cookie
// domain/path metadata and persists provider rotations without flattening scope.
func ValidateWebAccountSessionWithCookieSnapshot(ctx context.Context, platform, sessionKey, cookie string, cookies []BrowserCookie, email string, updater CookieSnapshotUpdater) error {
	return validateWebAccountSession(ctx, platform, sessionKey, cookie, cookies, email, updater, nil)
}

func validateWebAccountSession(ctx context.Context, platform, sessionKey, cookie string, cookies []BrowserCookie, email string, snapshotUpdater CookieSnapshotUpdater, legacyUpdater func(string) error) error {
	platform = strings.ToLower(strings.TrimSpace(platform))
	target, ok := webSessionTargetFor(platform)
	if !ok {
		return fmt.Errorf("nền tảng %q chưa hỗ trợ xác thực phiên", platform)
	}
	cookie = strings.TrimSpace(cookie)
	if cookie == "" && strings.Contains(sessionKey, "=") {
		cookie = strings.TrimSpace(sessionKey)
	}
	if cookie == "" && len(cookies) == 0 {
		return fmt.Errorf("tài khoản chưa có cookie phiên web")
	}
	if len(cookies) == 0 && snapshotUpdater != nil {
		legacyUpdater = func(cookie string) error { return snapshotUpdater(cookie, nil) }
	}
	auth := &WebSessionAuth{
		Platform:               platform,
		SessionValue:           strings.TrimSpace(sessionKey),
		CookieHeader:           cookie,
		CookieJar:              normalizeCookieJar(cookies),
		Email:                  email,
		OnCookieUpdate:         legacyUpdater,
		OnCookieSnapshotUpdate: snapshotUpdater,
	}
	return validateWebSessionAuth(ctx, target, auth)
}

// validateWebSessionAuth returns only after the cookie authenticates and the
// account's real email is known.
func validateWebSessionAuth(ctx context.Context, target WebSessionTarget, auth *WebSessionAuth) error {
	if auth == nil || strings.TrimSpace(auth.CookieHeader) == "" && len(auth.CookieJar) == 0 {
		return fmt.Errorf("cookie phiên trống")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if target.Platform == PlatformGoogleFlow {
		auth.CookieHeader = googleSessionCookie(auth)
	}
	adapter, ok := webSessionValidatorFor(target.Platform)
	if !ok {
		return fmt.Errorf("nền tảng %q chưa hỗ trợ xác thực phiên", target.Platform)
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if len(auth.CookieJar) > 0 {
		updater := auth.OnCookieSnapshotUpdate
		if updater == nil && auth.OnCookieUpdate != nil {
			updater = func(cookie string, _ []BrowserCookie) error { return auth.OnCookieUpdate(cookie) }
		}
		jar, err := newCookieSnapshotJar(target.Platform, auth.CookieHeader, auth.CookieJar, updater)
		if err != nil {
			return fmt.Errorf("không khởi tạo được cookie jar: %w", err)
		}
		auth.cookieSnapshotJar = jar
		auth.CookieJar = jar.Snapshot()
		auth.CookieHeader = jar.CookieHeader()
		client.Jar = jar
	}
	if err := adapter.validateSession(ctx, client, auth); err != nil {
		return err
	}
	email := accountEmail(auth.Email)
	if email == "" && target.Platform != PlatformMuseAI {
		return fmt.Errorf("chưa xác định được email thật của tài khoản")
	}
	auth.Email = email
	return nil
}

func webSessionCookieUpdater(auth *WebSessionAuth) func(string) error {
	return func(cookie string) error {
		if auth == nil {
			return fmt.Errorf("thiếu account cần cập nhật cookie")
		}
		auth.CookieHeader = cookie
		if auth.OnCookieUpdate != nil {
			return auth.OnCookieUpdate(cookie)
		}
		return nil
	}
}

func webSessionCookieSnapshotUpdater(auth *WebSessionAuth) CookieSnapshotUpdater {
	return func(cookie string, cookies []BrowserCookie) error {
		if auth == nil {
			return fmt.Errorf("thiếu account cần cập nhật cookie")
		}
		auth.CookieHeader = cookie
		auth.CookieJar = append([]BrowserCookie(nil), cookies...)
		if auth.OnCookieSnapshotUpdate != nil {
			return auth.OnCookieSnapshotUpdate(cookie, cookies)
		}
		if auth.OnCookieUpdate != nil {
			return auth.OnCookieUpdate(cookie)
		}
		return nil
	}
}

func webSessionProbeCaptured(ctx context.Context, client *http.Client, auth *WebSessionAuth, targetURL, expectedHost, cookie, accept string, headers map[string]string) ([]byte, error) {
	if auth == nil {
		return nil, fmt.Errorf("thiếu account cần xác minh")
	}
	body, refreshedCookie, err := webSessionProbeAndCapture(ctx, client, targetURL, expectedHost, cookie, accept, headers)
	if refreshedCookie != cookie {
		auth.CookieHeader = refreshedCookie
		if auth.cookieSnapshotJar != nil {
			auth.CookieJar = auth.cookieSnapshotJar.Snapshot()
		} else if auth.OnCookieUpdate != nil {
			if persistErr := auth.OnCookieUpdate(refreshedCookie); persistErr != nil {
				return nil, fmt.Errorf("persist refreshed provider cookies: %w", persistErr)
			}
		}
	}
	return body, err
}

func webSessionProbeAndCapture(ctx context.Context, client *http.Client, targetURL, expectedHost, cookie, accept string, headers map[string]string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, cookie, err
	}
	req.Header.Set("User-Agent", webSessionProbeUserAgent)
	if _, hasSnapshotJar := client.Jar.(*cookieSnapshotJar); !hasSnapshotJar {
		req.Header.Set("Cookie", cookie)
	}
	req.Header.Set("Accept", accept)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if jar, ok := client.Jar.(*cookieSnapshotJar); ok {
			if persistErr := jar.takeError(); persistErr != nil {
				return nil, cookie, fmt.Errorf("persist refreshed provider cookies: %w", persistErr)
			}
		}
		return nil, cookie, err
	}
	defer resp.Body.Close()
	if jar, ok := client.Jar.(*cookieSnapshotJar); ok {
		cookie = jar.CookieHeader()
		if err := jar.takeError(); err != nil {
			return nil, cookie, fmt.Errorf("persist refreshed provider cookies: %w", err)
		}
	} else {
		cookie = mergeFlowCookieHeader(cookie, resp.Cookies())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, cookie, &webSessionHTTPStatusError{statusCode: resp.StatusCode}
	}
	if resp.Request == nil || resp.Request.URL == nil || !strings.EqualFold(resp.Request.URL.Hostname(), expectedHost) {
		return nil, cookie, fmt.Errorf("bị chuyển hướng khỏi dịch vụ đăng nhập")
	}
	if path := strings.ToLower(resp.Request.URL.Path); strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/servicelogin") {
		return nil, cookie, fmt.Errorf("dịch vụ vẫn yêu cầu đăng nhập")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, cookie, err
	}
	return body, cookie, nil
}

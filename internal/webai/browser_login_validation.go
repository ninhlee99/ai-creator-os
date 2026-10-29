package webai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const webLoginUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// validateCapturedWebAuth returns only after the captured cookie authenticates
// against the provider and the account's real email is known.
func validateCapturedWebAuth(ctx context.Context, target WebLoginTarget, auth *CapturedWebAuth) error {
	if auth == nil || strings.TrimSpace(auth.CookieHeader) == "" {
		return fmt.Errorf("cookie phiên trống")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	adapter, ok := webLoginAdapterFor(target.Platform)
	if !ok {
		return fmt.Errorf("nền tảng %q chưa hỗ trợ xác thực phiên", target.Platform)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	if err := adapter.validateSession(ctx, client, auth); err != nil {
		return err
	}
	email := accountEmail(auth.Email)
	if email == "" && target.Platform != PlatformMuseAI && target.Platform != PlatformTikTok {
		return fmt.Errorf("chưa xác định được email thật của tài khoản")
	}
	if email != "" {
		auth.Email = email
	} else if auth.Email == "" {
		auth.Email = target.Name + " User"
	}
	return nil
}

func webLoginProbe(ctx context.Context, client *http.Client, targetURL, expectedHost, cookie, accept string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webLoginUserAgent)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", accept)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.Request == nil || resp.Request.URL == nil || !strings.EqualFold(resp.Request.URL.Hostname(), expectedHost) {
		return nil, fmt.Errorf("bị chuyển hướng khỏi dịch vụ đăng nhập")
	}
	if path := strings.ToLower(resp.Request.URL.Path); strings.Contains(path, "/login") || strings.Contains(path, "/signin") || strings.Contains(path, "/servicelogin") {
		return nil, fmt.Errorf("dịch vụ vẫn yêu cầu đăng nhập")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

package webai

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func captureCookieLogin(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(status, msg string)) (*CapturedWebAuth, error) {
	landingSeenCount := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		cookies, err := cdpListCookies(browser.wsURL)
		if err == nil && len(cookies) > 0 {
			if session, email := pickSessionCookieAndEmail(cookies, target); session != "" && cdpHasLandingHost(browser.port, target.LandingHost) {
				landingSeenCount++
				if liveEmail := cdpExtractPageIdentity(browser.port, target); liveEmail != "" {
					email = liveEmail
				}
				auth := &CapturedWebAuth{
					Platform:     target.Platform,
					SessionValue: session,
					CookieHeader: cookieHeaderForHost(cookies, target.CookieHost),
					Email:        email,
					BrowserUsed:  filepath.Base(browser.binary),
				}
				if err := validateCapturedWebAuth(ctx, target, auth); err == nil {
					return auth, nil
				}
				if updateStatus != nil {
					message := fmt.Sprintf("Đã nhận cookie %s nhưng phiên chưa được xác nhận hoạt động hoặc chưa đọc được email thật. Hoàn tất đăng nhập, OTP/2FA nếu trang còn yêu cầu.", target.Name)
					if landingSeenCount < 2 {
						message = fmt.Sprintf("Đang xác minh cookie hoạt động và email tài khoản %s; giữ cửa sổ đăng nhập mở.", target.Name)
					}
					updateStatus("waiting_for_user", message)
				}
			}
		}
		time.Sleep(time.Second)
	}
}

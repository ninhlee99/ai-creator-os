package webai

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func captureCookieLogin(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(status, msg string)) (*CapturedWebAuth, error) {
	landingSeenCount := 0
	accountLookupAttempted := false
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
				liveEmail := cdpExtractPageIdentity(browser.port, target)
				if liveEmail != "" {
					email = liveEmail
				}
				if target.Platform == PlatformGoogleFlow && !accountLookupAttempted {
					accountLookupAttempted = true
					if updateStatus != nil {
						updateStatus("waiting_for_user", "Đã đăng nhập Google Flow. Đang đọc email từ Google Account...")
					}
					// The login tab already lives on Google Account; only open a
					// background lookup tab if its identity is not visible there.
					if liveEmail == "" {
						if accountEmail := cdpLookupAccountEmail(ctx, browser.wsURL, browser.port, "https://myaccount.google.com/"); accountEmail != "" {
							email = accountEmail
						}
					}
				}
				auth := &CapturedWebAuth{
					Platform:                target.Platform,
					SessionValue:            session,
					CookieHeader:            cookieHeaderForTarget(cookies, target),
					CookieJar:               loginCookieJarForTarget(cookies, target),
					Email:                   email,
					BrowserUsed:             filepath.Base(browser.binary),
				}
				if err := validateCapturedWebAuth(ctx, target, auth); err == nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
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

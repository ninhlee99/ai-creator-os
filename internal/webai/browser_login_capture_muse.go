package webai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func captureMuseLogin(ctx context.Context, browser loginBrowserSession, target WebLoginTarget, updateStatus func(status, msg string)) (*CapturedWebAuth, error) {
	port := browser.port
	wsURL := browser.wsURL
	bin := browser.binary
	var musePendingEmail string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

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
				if err := validateCapturedWebAuth(ctx, target, auth); err != nil {
					if updateStatus != nil {
						updateStatus("waiting_for_user", "Đã nhận cookie Muse nhưng phiên chưa xác thực hoạt động. Hoàn tất đăng nhập, OTP/2FA nếu trang còn yêu cầu.")
					}
					time.Sleep(time.Second)
					continue
				}
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
}

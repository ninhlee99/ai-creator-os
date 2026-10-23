package webai

import (
	"context"
	"fmt"
	"net/http"
)

type googleFlowSessionValidator struct{}

func (googleFlowSessionValidator) target() WebSessionTarget {
	return WebSessionTarget{Platform: PlatformGoogleFlow, Name: "Google Flow"}
}

func (googleFlowSessionValidator) validateSession(ctx context.Context, client *http.Client, auth *WebSessionAuth) error {
	cookie := googleSessionCookie(auth)
	auth.CookieHeader = cookie
	if err := ProbeGoogleFlowSessionWithCookieSnapshot(ctx, cookie, auth.CookieJar, webSessionCookieSnapshotUpdater(auth)); err != nil {
		return fmt.Errorf("Google Flow chưa xác thực cookie: %w", err)
	}
	if len(auth.CookieJar) > 0 {
		jar, err := newCookieSnapshotJar(auth.Platform, auth.CookieHeader, auth.CookieJar, webSessionCookieSnapshotUpdater(auth))
		if err != nil {
			return fmt.Errorf("không khởi tạo lại được cookie jar của Flow: %w", err)
		}
		client.Jar = jar
		auth.cookieSnapshotJar = jar
	}
	actualEmail := probeGoogleAccountEmail(ctx, client, auth)
	if actualEmail == "" {
		actualEmail = accountEmail(auth.Email)
	}
	if actualEmail == "" {
		return fmt.Errorf("Google Flow đã xác thực phiên nhưng chưa xác minh được email tài khoản")
	}
	expectedEmail := accountEmail(auth.Email)
	if expectedEmail != "" && expectedEmail != actualEmail {
		return fmt.Errorf("cookie Google Flow thuộc %s, không khớp email đang lưu %s", actualEmail, expectedEmail)
	}
	auth.Email = actualEmail
	return nil
}

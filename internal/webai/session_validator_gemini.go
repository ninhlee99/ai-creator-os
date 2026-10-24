package webai

import (
	"context"
	"fmt"
	"net/http"
)

type geminiSessionValidator struct{}

func (geminiSessionValidator) target() WebSessionTarget {
	return WebSessionTarget{Platform: PlatformGemini, Name: "Gemini"}
}

func (geminiSessionValidator) validateSession(ctx context.Context, client *http.Client, auth *WebSessionAuth) error {
	cookie := googleSessionCookie(auth)
	auth.CookieHeader = cookie
	body, err := webSessionProbeCaptured(ctx, client, auth, "https://gemini.google.com/app", "gemini.google.com", cookie, "text/html,application/xhtml+xml", nil)
	if err != nil {
		return fmt.Errorf("Gemini chưa xác thực cookie: %w", err)
	}
	email := accountEmail(parseGoogleAccountEmail(string(body)))
	if email == "" {
		email = probeGoogleAccountEmail(ctx, client, auth)
	}
	if email == "" {
		return fmt.Errorf("Gemini chưa xác nhận được email tài khoản")
	}
	auth.Email = email
	return nil
}

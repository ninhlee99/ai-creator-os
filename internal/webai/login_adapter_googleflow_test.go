package webai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleFlowLoginAdapter_ValidateSessionWithEmail(t *testing.T) {
	adapter := googleFlowLoginAdapter{}

	auth := &CapturedWebAuth{
		Platform:     PlatformGoogleFlow,
		SessionValue: "test-flow-session",
		CookieHeader: "__Secure-1PSID=test-flow-session",
		Email:        "creator@gmail.com",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><div aria-label="Tài khoản Google: creator@gmail.com"></div></body></html>`))
	}))
	defer ts.Close()

	// Should validate cleanly when email is already captured from CDP
	err := adapter.validateSession(context.Background(), ts.Client(), auth)
	if err != nil {
		t.Fatalf("expected validateSession to accept valid session with email, got: %v", err)
	}
	if auth.Email != "creator@gmail.com" {
		t.Errorf("expected email to be creator@gmail.com, got %s", auth.Email)
	}
}

func TestGoogleFlowLoginAdapter_MatchesIdentityURL(t *testing.T) {
	adapter := googleFlowLoginAdapter{}
	validURLs := []string{
		"https://labs.google/fx/editor",
		"https://flow.google.com/",
		"https://myaccount.google.com/",
		"https://accounts.google.com/signin",
	}
	for _, u := range validURLs {
		if !adapter.matchesIdentityURL(u) {
			t.Errorf("expected %s to match identity URL for Google Flow", u)
		}
	}
	if adapter.matchesIdentityURL("https://evil.com/google") {
		t.Errorf("evil.com should not match identity URL")
	}
}

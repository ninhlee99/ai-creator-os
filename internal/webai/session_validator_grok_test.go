package webai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGrokSessionValidator_EmailResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"email":"creator@x.ai","username":"creator"}`))
	}))
	defer ts.Close()

	validator := grokSessionValidator{}
	auth := &WebSessionAuth{
		Platform:     PlatformGrok,
		SessionValue: "sso-test-token",
		CookieHeader: "sso=sso-test-token",
		Email:        "creator@gmail.com",
	}

	// Directly test validateSession behavior with 403 simulation when email is verified from browser
	ts403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts403.Close()

	err403 := validator.validateSession(context.Background(), ts403.Client(), auth)
	if err403 != nil {
		t.Fatalf("expected 403 with verified email to succeed, got: %v", err403)
	}

	// Test 403 when email is empty - should return error waiting for browser
	authEmptyEmail := &WebSessionAuth{
		Platform:     PlatformGrok,
		SessionValue: "sso-test-token",
		CookieHeader: "sso=sso-test-token",
	}
	errEmpty := validator.validateSession(context.Background(), ts403.Client(), authEmptyEmail)
	if errEmpty == nil {
		t.Fatalf("expected 403 with empty email to fail and wait for browser, got nil")
	}
}

func TestGrokCookieDomainMatches(t *testing.T) {
	target := WebLoginTarget{Platform: PlatformGrok, CookieHost: "grok.com"}
	domains := []string{"grok.com", ".grok.com", "x.ai", ".x.ai", "x.com", ".x.com", "twitter.com", ".twitter.com"}
	for _, d := range domains {
		if !loginCookieDomainMatches(d, target) {
			t.Errorf("expected domain %q to match Grok target", d)
		}
	}
	if loginCookieDomainMatches("attacker.com", target) {
		t.Errorf("attacker.com should not match Grok target")
	}
}

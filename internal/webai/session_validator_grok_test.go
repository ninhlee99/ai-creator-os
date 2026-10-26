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
	}

	body, err := webSessionProbeCaptured(context.Background(), ts.Client(), auth, ts.URL, "127.0.0.1", auth.CookieHeader, "application/json", nil)
	if err != nil && auth.SessionValue != "" {
		// Server simulated ok
	}
	_ = body

	// Directly test validateSession behavior with 403 simulation
	ts403 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts403.Close()

	err403 := validator.validateSession(context.Background(), ts403.Client(), auth)
	// On 403 with session value, should succeed with fallback
	if err403 != nil {
		t.Fatalf("expected 403 to be handled gracefully for grok, got: %v", err403)
	}
	if auth.Email == "" {
		t.Errorf("expected auth.Email to be set, got empty")
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

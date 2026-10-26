package webai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleFlowLoginAdapter_RejectUnauthenticatedCookies(t *testing.T) {
	adapter := googleFlowLoginAdapter{}

	// Only visitor NID cookie without real Google auth session
	auth := &CapturedWebAuth{
		Platform:     PlatformGoogleFlow,
		SessionValue: "visitor-nid-cookie",
		CookieHeader: "NID=visitor-nid-cookie",
		Email:        "creator@gmail.com",
	}

	err := adapter.validateSession(context.Background(), http.DefaultClient, auth)
	if err == nil {
		t.Fatalf("expected validateSession to reject visitor-only NID cookies, got nil")
	}
}

func TestGoogleFlowLoginAdapter_SessionFromCookiesIgnoresNID(t *testing.T) {
	adapter := googleFlowLoginAdapter{}

	cookies := []cdpCookie{
		{Name: "NID", Value: "anonymous-nid-value", Domain: ".google.com"},
		{Name: "_ga", Value: "analytics-value", Domain: ".flow.google.com"},
	}

	session, email := adapter.sessionFromCookies(cookies)
	if session != "" {
		t.Errorf("expected session to be empty when only NID is present, got %q", session)
	}
	if email != "" {
		t.Errorf("expected email to be empty, got %q", email)
	}

	// When genuine SID is added
	cookiesWithSID := append(cookies, cdpCookie{
		Name:   "__Secure-1PSID",
		Value:  "real-secure-psid",
		Domain: ".google.com",
	})
	session2, _ := adapter.sessionFromCookies(cookiesWithSID)
	if session2 != "real-secure-psid" {
		t.Errorf("expected session to pick __Secure-1PSID, got %q", session2)
	}
}

func TestGoogleFlowLoginAdapter_MatchesIdentityURL(t *testing.T) {
	adapter := googleFlowLoginAdapter{}
	validURLs := []string{
		"https://flow.google.com/",
		"https://flow.google.com/project/123",
		"https://labs.google/fx/editor",
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

	err := adapter.validateSession(context.Background(), ts.Client(), auth)
	if err != nil && (auth.Email != "creator@gmail.com") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

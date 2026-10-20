package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsPageRedirectsToAccounts(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/models")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /models = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/accounts" {
		t.Fatalf("GET /models redirects to %q, want /accounts", loc)
	}
}

func TestAffiliateRedirect(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/affiliate", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /affiliate = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/studio" {
		t.Fatalf("GET /affiliate redirects to %q, want /studio", loc)
	}
}

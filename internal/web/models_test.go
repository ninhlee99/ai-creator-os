package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsPageDissolved(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/models")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /models = %d, want 404", rec.Code)
	}
}

func TestAffiliateDissolved(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/affiliate", nil)
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /affiliate = %d, want 404", rec.Code)
	}
}

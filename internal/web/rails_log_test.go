package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/logx"
)

func TestRailsRequestLoggerMiddleware(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	logger := logx.SetupRailsLogging(&consoleBuf, &fileBuf, logx.LevelDebug)
	defer logx.SetupRailsLogging(nil, nil, logx.LevelInfo)
	_ = logger

	s := &Server{}
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<h1>Dashboard</h1>"))
	})

	handler := s.railsRequestLogger(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/dashboard?section=overview", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	fileLog := fileBuf.String()
	if !strings.Contains(fileLog, "Started GET \"/dashboard?section=overview\"") {
		t.Errorf("missing Started line: %s", fileLog)
	}
	if !strings.Contains(fileLog, "Processing by WebServer#GET /dashboard as HTML") {
		t.Errorf("missing Processing line: %s", fileLog)
	}
	if !strings.Contains(fileLog, "Parameters: {\"section\"=>\"overview\"}") {
		t.Errorf("missing Parameters line: %s", fileLog)
	}
	if !strings.Contains(fileLog, "Completed 200 OK") {
		t.Errorf("missing Completed line: %s", fileLog)
	}
}

func TestRailsRequestLoggerRedirect(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	_ = logx.SetupRailsLogging(&consoleBuf, &fileBuf, logx.LevelDebug)
	defer logx.SetupRailsLogging(nil, nil, logx.LevelInfo)

	s := &Server{}
	redirectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/studio", http.StatusSeeOther)
	})

	handler := s.railsRequestLogger(redirectHandler)

	req := httptest.NewRequest(http.MethodGet, "/content", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}

	fileLog := fileBuf.String()
	if !strings.Contains(fileLog, "Started GET \"/content\"") {
		t.Errorf("missing Started line: %s", fileLog)
	}
	if !strings.Contains(fileLog, "Redirected to /studio") {
		t.Errorf("missing Redirected to line: %s", fileLog)
	}
	if !strings.Contains(fileLog, "Completed 303 See Other") {
		t.Errorf("missing Completed line: %s", fileLog)
	}
}

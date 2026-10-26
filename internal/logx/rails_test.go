package logx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"fatal", LevelFatal},
		{"unknown", LevelDebug},
	}
	for _, tc := range tests {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestRailsLoggerFormatting(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	logger := NewRailsLogger(&consoleBuf, &fileBuf, LevelDebug)

	logger.Infof("Server listening on :8080")

	fileOut := fileBuf.String()
	consoleOut := consoleBuf.String()

	// Verify file log contains Rails format: I, [YYYY-MM-DD... #PID] INFO  -- : message
	if !strings.HasPrefix(fileOut, "I, [") {
		t.Fatalf("fileOut should start with 'I, [', got: %s", fileOut)
	}
	if !strings.Contains(fileOut, "INFO  -- : Server listening on :8080") {
		t.Fatalf("fileOut missing Rails info signature, got: %s", fileOut)
	}
	if strings.Contains(fileOut, "\033[") {
		t.Fatalf("fileOut must not contain ANSI escape codes, got: %s", fileOut)
	}

	// Verify console output has ANSI color styling
	if !strings.Contains(consoleOut, "\033[") {
		t.Fatalf("consoleOut must contain ANSI escape codes for Rails debug console, got: %s", consoleOut)
	}
	if !strings.Contains(consoleOut, "Server listening on :8080") {
		t.Fatalf("consoleOut missing message, got: %s", consoleOut)
	}
}

func TestRailsLoggerFiltering(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	logger := NewRailsLogger(&consoleBuf, &fileBuf, LevelWarn)

	logger.Debugf("this debug should be filtered")
	logger.Infof("this info should be filtered")
	logger.Warnf("this warning should be logged")

	out := fileBuf.String()
	if strings.Contains(out, "this debug") || strings.Contains(out, "this info") {
		t.Fatalf("filtered messages appeared in log: %s", out)
	}
	if !strings.Contains(out, "this warning should be logged") {
		t.Fatalf("warning missing from log: %s", out)
	}
	if !strings.HasPrefix(out, "W, [") {
		t.Fatalf("warning must start with 'W, [', got: %s", out)
	}
}

func TestRailsRequestLogging(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	logger := NewRailsLogger(&consoleBuf, &fileBuf, LevelDebug)

	req := httptest.NewRequest(http.MethodGet, "/studio?tab=jobs&status=done", nil)
	logger.LogRequest(req, http.StatusOK, 15*time.Millisecond, "")

	fileOut := fileBuf.String()
	if !strings.Contains(fileOut, "Started GET \"/studio?tab=jobs&status=done\"") {
		t.Errorf("missing Started line: %s", fileOut)
	}
	if !strings.Contains(fileOut, "Processing by WebServer#GET /studio as HTML") {
		t.Errorf("missing Processing line: %s", fileOut)
	}
	if !strings.Contains(fileOut, "Parameters: {") || !strings.Contains(fileOut, "\"tab\"=>\"jobs\"") {
		t.Errorf("missing Parameters line: %s", fileOut)
	}
	if !strings.Contains(fileOut, "Completed 200 OK in 15.0ms") {
		t.Errorf("missing Completed line: %s", fileOut)
	}
}

func TestRailsRequestRedirectLogging(t *testing.T) {
	var consoleBuf, fileBuf bytes.Buffer
	logger := NewRailsLogger(&consoleBuf, &fileBuf, LevelDebug)

	req := httptest.NewRequest(http.MethodPost, "/accounts/1/delete", nil)
	logger.LogRequest(req, http.StatusSeeOther, 2*time.Millisecond, "/accounts")

	fileOut := fileBuf.String()
	if !strings.Contains(fileOut, "Redirected to /accounts") {
		t.Errorf("missing Redirected to line: %s", fileOut)
	}
	if !strings.Contains(fileOut, "Completed 303 See Other in 2.0ms") {
		t.Errorf("missing Completed line: %s", fileOut)
	}
}

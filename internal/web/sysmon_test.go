package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSystemMetricsEndpoint(t *testing.T) {
	s := newTestServer(t)

	// 1. GET /api/system/metrics
	rec := get(t, s, "/api/system/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/system/metrics = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "id=\"system-metrics-panel\"") {
		t.Fatalf("expected response to contain system-metrics-panel, got: %s", body)
	}
	if !strings.Contains(body, "RAM Hệ Thống") {
		t.Fatalf("expected response to contain RAM metrics, got: %s", body)
	}
	if !strings.Contains(body, "CPU Load Avg") {
		t.Fatalf("expected response to contain CPU metrics, got: %s", body)
	}

	// 2. GET /api/system/sidebar-metrics
	recSidebar := get(t, s, "/api/system/sidebar-metrics")
	if recSidebar.Code != http.StatusOK {
		t.Fatalf("GET /api/system/sidebar-metrics = %d, want 200", recSidebar.Code)
	}
	sidebarBody := recSidebar.Body.String()
	if !strings.Contains(sidebarBody, "id=\"sidebar-sysmon\"") {
		t.Fatalf("expected response to contain sidebar-sysmon, got: %s", sidebarBody)
	}
	if !strings.Contains(sidebarBody, "Tài nguyên") {
		t.Fatalf("expected response to contain Tài nguyên, got: %s", sidebarBody)
	}

	// 3. Verify that non-dashboard screens (/accounts, /studio, /settings) also render sidebar-sysmon
	recAccounts := get(t, s, "/accounts")
	if recAccounts.Code != http.StatusOK {
		t.Fatalf("GET /accounts = %d, want 200", recAccounts.Code)
	}
	if !strings.Contains(recAccounts.Body.String(), "id=\"sidebar-sysmon\"") {
		t.Fatalf("expected /accounts to render sidebar-sysmon at bottom of sidebar")
	}
}

func TestSystemKillProcessEndpoint(t *testing.T) {
	s := newTestServer(t)

	// 1. Rejects non-POST
	recGet := get(t, s, "/api/system/kill-process")
	if recGet.Code == http.StatusOK || recGet.Code == http.StatusSeeOther {
		t.Fatalf("GET /api/system/kill-process unexpectedly succeeded: %d", recGet.Code)
	}

	// 2. Rejects invalid PID
	recInvalid := postForm(t, s, "/api/system/kill-process", url.Values{
		"pid": {"abc"},
	})
	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("POST with invalid pid = %d, want 400", recInvalid.Code)
	}

	// 3. Rejects PID <= 0
	recZero := postForm(t, s, "/api/system/kill-process", url.Values{
		"pid": {"0"},
	})
	if recZero.Code != http.StatusBadRequest {
		t.Fatalf("POST with pid=0 = %d, want 400", recZero.Code)
	}

	// 4. Safe kill attempt on non-existent process
	recValid := postForm(t, s, "/api/system/kill-process", url.Values{
		"pid": {"9999999"},
	})
	if recValid.Code != http.StatusSeeOther {
		t.Fatalf("POST with valid pid = %d, want 303 redirect", recValid.Code)
	}
}

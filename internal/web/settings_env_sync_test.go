package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "aicos-env-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")
	l, err := ledger.New(dbPath)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	mgr, err := network.NewAccountManager(l, dbPath)
	if err != nil {
		t.Fatalf("create mgr: %v", err)
	}
	cfg := LoadConfig()
	cfg.GeminiAPIKeys = []string{"AIzaSyTestKey1111111111111111111111111111111"}
	s, err := NewServer(cfg, l, mgr, dbPath)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	cleanup := func() {
		_ = s.Close()
		_ = l.Close()
		_ = os.RemoveAll(tmpDir)
	}
	return s, cleanup
}

func TestEnvExportAndImportEnv(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Set some initial env vars in Ledger
	_ = s.Ledger.SetSetting("env:OPENAI_API_KEY", "sk-proj-test123456789")
	_ = s.Ledger.SetSetting("env:YOUTUBE_DEFAULT_PRIVACY", "public")
	_ = s.Ledger.SetSetting(accesstrade.KeySetting, "at-test-access-token-999")

	// 2. Test handleEnvExport (.env format)
	req := httptest.NewRequest("GET", "/settings/env/export", nil)
	rec := httptest.NewRecorder()
	s.handleEnvExport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export returned status %d, expected 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "OPENAI_API_KEY=\"sk-proj-test123456789\"") {
		t.Errorf("expected OPENAI_API_KEY in export, got:\n%s", body)
	}
	if !strings.Contains(body, "YOUTUBE_DEFAULT_PRIVACY=\"public\"") {
		t.Errorf("expected YOUTUBE_DEFAULT_PRIVACY in export, got:\n%s", body)
	}
	if !strings.Contains(body, "ACCESSTRADE_API_KEY=\"at-test-access-token-999\"") {
		t.Errorf("expected ACCESSTRADE_API_KEY in export, got:\n%s", body)
	}

	// 3. Test handleEnvImport on a fresh server or with new values
	newEnvText := `
# Cấu hình chuyển từ máy khác
GEMINI_API_KEYS="AIzaSyImportedKey222222222222222222222222222"
ANTHROPIC_API_KEY="sk-ant-test-imported-888"
TIKTOK_CLIENT_KEY="tt-client-key-333"
ACCESSTRADE_API_KEY="at-new-imported-key"
`
	importBody := strings.NewReader("env_content=" + strings.ReplaceAll(newEnvText, "\n", "%0A"))
	importReq := httptest.NewRequest("POST", "/settings/env/import", importBody)
	importReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	importRec := httptest.NewRecorder()
	s.handleEnvImport(importRec, importReq)

	if importRec.Code != http.StatusSeeOther {
		t.Fatalf("import returned status %d, expected 303 redirect", importRec.Code)
	}

	// Verify values applied in ledger & environment
	val, ok := s.EffectiveEnv("ANTHROPIC_API_KEY")
	if !ok || val != "sk-ant-test-imported-888" {
		t.Errorf("expected ANTHROPIC_API_KEY='sk-ant-test-imported-888', got %q (ok=%v)", val, ok)
	}
	ttVal, ok := s.EffectiveEnv("TIKTOK_CLIENT_KEY")
	if !ok || ttVal != "tt-client-key-333" {
		t.Errorf("expected TIKTOK_CLIENT_KEY='tt-client-key-333', got %q", ttVal)
	}
	atKey, ok, _ := s.Ledger.GetSetting(accesstrade.KeySetting)
	if !ok || atKey != "at-new-imported-key" {
		t.Errorf("expected ACCESSTRADE key 'at-new-imported-key', got %q", atKey)
	}
}

func TestEnvExportAndImportJSON(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	_ = s.Ledger.SetSetting("env:ELEVENLABS_API_KEY", "el-test-key-555")

	// 1. Export JSON
	req := httptest.NewRequest("GET", "/settings/env/export-json", nil)
	rec := httptest.NewRecorder()
	s.handleEnvExportJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export json returned status %d, expected 200", rec.Code)
	}

	var exp EnvExportJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &exp); err != nil {
		t.Fatalf("unmarshal export json: %v", err)
	}
	if exp.EnvVars["ELEVENLABS_API_KEY"] != "el-test-key-555" {
		t.Errorf("expected ELEVENLABS_API_KEY in json export, got %q", exp.EnvVars["ELEVENLABS_API_KEY"])
	}

	// 2. Import multipart JSON file
	newJSON := EnvExportJSON{
		Version:       "1.0",
		GeminiAPIKeys: []string{"AIzaSyImportedKeyJSON99999999999999999999"},
		EnvVars: map[string]string{
			"FB_PAGE_ID": "meta-page-imported-777",
		},
	}
	jsonData, _ := json.Marshal(newJSON)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, _ := w.CreateFormFile("env_file", "backup.json")
	_, _ = part.Write(jsonData)
	_ = w.Close()

	uploadReq := httptest.NewRequest("POST", "/settings/env/import", &b)
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	s.handleEnvImport(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusSeeOther {
		t.Fatalf("import json returned status %d, expected 303", uploadRec.Code)
	}

	pageID, ok := s.EffectiveEnv("FB_PAGE_ID")
	if !ok || pageID != "meta-page-imported-777" {
		t.Errorf("expected FB_PAGE_ID='meta-page-imported-777', got %q", pageID)
	}
}

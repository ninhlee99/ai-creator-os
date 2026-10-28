package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

func TestConfigJSONExportAndImport(t *testing.T) {
	s, cleanup := setupTestServer(t)
	defer cleanup()

	_ = s.Ledger.SetSetting("env:ELEVENLABS_API_KEY", "el-test-key-555")
	_ = s.Ledger.SetSetting("accesstrade.api_key", "at-secret-444")
	_ = s.Ledger.SetSetting("notify.telegram_bot", "telegram-secret-333")

	// 1. Export JSON
	req := httptest.NewRequest("GET", "/settings/config/export-json", nil)
	rec := httptest.NewRecorder()
	s.handleConfigJSONExport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export json returned status %d, expected 200", rec.Code)
	}

	var exp ConfigExportJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &exp); err != nil {
		t.Fatalf("unmarshal export json: %v", err)
	}
	if len(exp.EnvVars) != 0 || len(exp.GeminiAPIKeys) != 0 {
		t.Errorf("secret fields must be absent from export: env=%v gemini=%v", exp.EnvVars, exp.GeminiAPIKeys)
	}
	for _, secret := range []string{"el-test-key-555", "at-secret-444", "telegram-secret-333", "AIzaSyTestKey"} {
		if bytes.Contains(rec.Body.Bytes(), []byte(secret)) {
			t.Fatalf("plain JSON export leaked secret %q", secret)
		}
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("web_ai_accounts")) || bytes.Contains(rec.Body.Bytes(), []byte("cookie")) || bytes.Contains(rec.Body.Bytes(), []byte("session_key")) {
		t.Fatal("plain JSON export must not include provider session credentials")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, private" {
		t.Fatalf("Cache-Control = %q; want no-store, private", got)
	}

	// 2. Import multipart JSON file
	newJSON := ConfigExportJSON{
		Version:       "1.0",
		GeminiAPIKeys: []string{"AIzaSyImportedKeyJSON99999999999999999999"},
		EnvVars: map[string]string{
			"FB_PAGE_ID": "meta-page-imported-777",
		},
	}
	jsonData, _ := json.Marshal(newJSON)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, _ := w.CreateFormFile("json_file", "backup.json")
	_, _ = part.Write(jsonData)
	_ = w.Close()

	uploadReq := httptest.NewRequest("POST", "/settings/config/import-json", &b)
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	s.handleConfigJSONImport(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusSeeOther {
		t.Fatalf("import json returned status %d, expected 303", uploadRec.Code)
	}

	pageID, ok := s.EffectiveEnv("FB_PAGE_ID")
	if !ok || pageID != "meta-page-imported-777" {
		t.Errorf("expected FB_PAGE_ID='meta-page-imported-777', got %q", pageID)
	}
}

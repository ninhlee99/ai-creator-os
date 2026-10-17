package webai

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestWebAIStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "webai_test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Initial count should be 0
	if count := store.CountConnected(); count != 0 {
		t.Errorf("expected 0 connected, got %d", count)
	}

	// Save Claude account
	err = store.SaveAccount(Account{
		Platform:    PlatformClaude,
		AccountName: "Ninh Claude",
		Email:       "ninh@example.com",
		SessionKey:  "sk-ant-test12345",
		Status:      StatusConnected,
		Tier:        "pro",
	})
	if err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	// Verify Claude account
	acc, err := store.GetAccount(PlatformClaude)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.AccountName != "Ninh Claude" || acc.Status != StatusConnected || acc.Tier != "pro" {
		t.Errorf("unexpected account data: %+v", acc)
	}

	// Save ChatGPT account
	err = store.SaveAccount(Account{
		Platform:    PlatformChatGPT,
		AccountName: "Ninh GPT",
		Email:       "gpt@example.com",
		SessionKey:  "session-token-xyz",
		Status:      StatusConnected,
	})
	if err != nil {
		t.Fatalf("SaveAccount ChatGPT failed: %v", err)
	}

	if count := store.CountConnected(); count != 2 {
		t.Errorf("expected 2 connected, got %d", count)
	}

	all, err := store.GetAllAccounts()
	if err != nil {
		t.Fatalf("GetAllAccounts failed: %v", err)
	}
	if len(all) != len(SupportedPlatforms()) {
		t.Errorf("expected %d platforms in map, got %d", len(SupportedPlatforms()), len(all))
	}
	if claudeAccs := all[PlatformClaude]; len(claudeAccs) == 0 || claudeAccs[0].Status != StatusConnected {
		t.Errorf("expected claude connected: %+v", claudeAccs)
	}
	if geminiAccs := all[PlatformGemini]; len(geminiAccs) != 0 {
		t.Errorf("expected gemini empty: %+v", geminiAccs)
	}

	// Disconnect Claude
	err = store.DisconnectAccount(PlatformClaude)
	if err != nil {
		t.Fatalf("DisconnectAccount failed: %v", err)
	}
	acc, err = store.GetAccount(PlatformClaude)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.Status != StatusDisconnected || acc.SessionKey != "" {
		t.Errorf("expected disconnected and empty session key: %+v", acc)
	}
	if count := store.CountConnected(); count != 1 {
		t.Errorf("expected 1 connected, got %d", count)
	}
}

func TestMultiAccountRotation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_rotation.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Add 2 accounts for Gemini
	_, err = store.AddAccount(Account{
		Platform:    PlatformGemini,
		AccountName: "Gemini Acc 1",
		Email:       "g1@example.com",
		SessionKey:  "tok-gemini-1",
	})
	if err != nil {
		t.Fatalf("AddAccount 1 failed: %v", err)
	}

	_, err = store.AddAccount(Account{
		Platform:    PlatformGemini,
		AccountName: "Gemini Acc 2",
		Email:       "g2@example.com",
		SessionKey:  "tok-gemini-2",
	})
	if err != nil {
		t.Fatalf("AddAccount 2 failed: %v", err)
	}

	// First pick
	acc1, err := store.PickActiveAccount(PlatformGemini)
	if err != nil {
		t.Fatalf("expected account picked, got err: %v", err)
	}

	// Rate limit the picked account for 1 hour
	if err := store.MarkRateLimited(acc1.ID, 1*time.Hour); err != nil {
		t.Fatalf("MarkRateLimited failed: %v", err)
	}

	// Next pick should automatically rotate to the other account
	acc2, err := store.PickActiveAccount(PlatformGemini)
	if err != nil {
		t.Fatalf("expected auto-rotation to other account, got err: %v", err)
	}
	if acc2.ID == acc1.ID {
		t.Fatalf("expected different account picked, got same: %s", acc1.ID)
	}

	// Reset cooldown on acc1
	if err := store.ResetCooldown(acc1.ID); err != nil {
		t.Fatalf("ResetCooldown failed: %v", err)
	}

	ready, cooldown, total := store.CountByPlatform(PlatformGemini)
	if ready != 2 || cooldown != 0 || total != 2 {
		t.Errorf("expected 2 ready, 0 cooldown, 2 total, got ready=%d cool=%d total=%d", ready, cooldown, total)
	}
}

func TestPromptSynthesizer(t *testing.T) {
	pkg := GenerateAdScriptPackage(
		"Serum Dưỡng Trắng",
		"Mỹ phẩm",
		"problem_solution",
		"Nữ 20-35 tuổi",
		"Làm sáng da sau 7 ngày, mờ thâm nám",
		"Bấm vào giỏ hàng ngay",
	)

	if pkg.ProductName != "Serum Dưỡng Trắng" {
		t.Errorf("unexpected product name: %s", pkg.ProductName)
	}
	if len(pkg.Scenes) != 4 {
		t.Errorf("expected 4 scenes, got %d", len(pkg.Scenes))
	}
	if pkg.Scenes[0].GeminiPrompt == "" {
		t.Errorf("expected Gemini prompt to be generated")
	}
	if pkg.Scenes[0].MuseAIPrompt == "" {
		t.Errorf("expected Muse AI prompt to be generated")
	}
	if pkg.ChatGPTPrompt == "" || pkg.ClaudePrompt == "" {
		t.Errorf("expected ChatGPT and Claude prompts")
	}

	// Test three_reasons framework
	pkg2 := GenerateAdScriptPackage("Bình Giữ Nhiệt Cao Cấp", "Gia dụng", "three_reasons", "", "", "")
	if len(pkg2.Scenes) != 4 {
		t.Errorf("expected 4 scenes in three_reasons, got %d", len(pkg2.Scenes))
	}

	// Test transformation framework
	pkg3 := GenerateAdScriptPackage("Tai Nghe Chống Ồn", "Công nghệ", "transformation", "", "", "")
	if len(pkg3.Scenes) != 4 {
		t.Errorf("expected 4 scenes in transformation, got %d", len(pkg3.Scenes))
	}
}

func TestProxyRelayHeaders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "webai_proxy_test.db")
	store, _ := NewStore(dbPath)
	defer store.Close()

	relay := NewProxyRelay(store)

	// Test invalid platform
	req := httptest.NewRequest("GET", "/web-ai/proxy/unknown", nil)
	rr := httptest.NewRecorder()
	relay.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown platform, got %d", rr.Code)
	}
}

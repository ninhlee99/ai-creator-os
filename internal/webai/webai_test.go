package webai

import (
	"context"
	"database/sql"
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

func TestAccountIdentityUsesCanonicalEmail(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "webai_identity.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	id1, err := store.UpsertAccountSession(PlatformGemini, "session-one", "cookie-one", "Google", " Creator@Example.com ", "free")
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	id2, err := store.UpsertAccountSession(PlatformGemini, "session-two", "cookie-two", "Google creator", "creator@example.com", "free")
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("same platform email created two accounts: %q != %q", id1, id2)
	}
	accounts, err := store.GetAccounts(PlatformGemini)
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("same email should map to one account, got %d", len(accounts))
	}
	if accounts[0].Email != "creator@example.com" || accounts[0].SessionKey != "session-two" || accounts[0].Cookie != "cookie-two" {
		t.Fatalf("canonical account did not receive latest session: %+v", accounts[0])
	}

	addID, err := store.AddAccount(Account{
		Platform: PlatformGemini, AccountName: "Updated", Email: "CREATOR@example.com",
		SessionKey: "session-three", Cookie: "cookie-three", Status: StatusConnected,
	})
	if err != nil {
		t.Fatalf("AddAccount duplicate email should update existing account: %v", err)
	}
	if addID != id1 {
		t.Fatalf("AddAccount changed account ID for same email: got %q want %q", addID, id1)
	}
	accounts, err = store.GetAccounts(PlatformGemini)
	if err != nil || len(accounts) != 1 || accounts[0].SessionKey != "session-three" {
		t.Fatalf("AddAccount failed to update canonical identity: accounts=%+v err=%v", accounts, err)
	}
}

func TestStoreMigrationCollapsesDuplicateEmails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "webai_migrate.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX idx_webai_accounts_platform_email`); err != nil {
		t.Fatalf("drop identity index: %v", err)
	}
	for _, row := range []struct {
		id, email, session, cookie, updated string
	}{
		{"old", "Creator@Example.com", "old-session", "active-cookie", "2026-01-01 00:00:00"},
		{"new", "creator@example.com", "new-session", "", "2026-01-02 00:00:00"},
	} {
		_, err = db.Exec(`
			INSERT INTO webai_accounts (id, platform, account_name, email, session_key, cookie, status, tier, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, row.id, PlatformGemini, "Google", row.email, row.session, row.cookie, StatusConnected, "free", row.updated)
		if err != nil {
			_ = db.Close()
			t.Fatalf("insert legacy duplicate %s: %v", row.id, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	store, err = NewStore(dbPath)
	if err != nil {
		t.Fatalf("reopen and migrate store: %v", err)
	}
	defer store.Close()
	accounts, err := store.GetAccounts(PlatformGemini)
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "new" || accounts[0].Email != "creator@example.com" || accounts[0].Cookie != "active-cookie" {
		t.Fatalf("migration did not retain newest canonical identity: %+v", accounts)
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

	// Test AI Engine Personas
	pkgGrok := GenerateAdScriptPackageWithEngine(context.Background(), nil, EngineGrok, "Camera Giám Sát", "Công nghệ", "problem_solution", "", "", "")
	if pkgGrok.AIEngine != EngineGrok || pkgGrok.AIEngineBadge == "" {
		t.Errorf("expected Grok engine with badge, got engine=%s badge=%s", pkgGrok.AIEngine, pkgGrok.AIEngineBadge)
	}

	pkgClaude := GenerateAdScriptPackageWithEngine(context.Background(), nil, EngineClaude, "Đầm Lụa Thiết Kế", "Thời trang", "transformation", "", "", "")
	if pkgClaude.AIEngine != EngineClaude || pkgClaude.AIEngineBadge == "" {
		t.Errorf("expected Claude engine with badge, got engine=%s badge=%s", pkgClaude.AIEngine, pkgClaude.AIEngineBadge)
	}

	pkgChatGPT := GenerateAdScriptPackageWithEngine(context.Background(), nil, EngineChatGPT, "Máy Lọc Không Khí", "Gia dụng", "problem_solution", "", "", "")
	if pkgChatGPT.AIEngine != EngineChatGPT || pkgChatGPT.AIEngineBadge == "" {
		t.Errorf("expected ChatGPT engine with badge, got engine=%s badge=%s", pkgChatGPT.AIEngine, pkgChatGPT.AIEngineBadge)
	}
}

func TestMarkSessionExpiredStopsAccountRotationAndReconnectRestoresIt(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "webai_session_state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id, err := store.UpsertAccountSession(PlatformMuseAI, "session-old", "hatch_sess=old", "Muse", "creator@example.com", "free")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSessionExpired(PlatformMuseAI, "creator@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PickActiveAccount(PlatformMuseAI); err == nil {
		t.Fatal("expired account remained eligible for rotation")
	}
	if _, err := store.UpsertAccountSession(PlatformMuseAI, "session-new", "hatch_sess=new", "Muse", "creator@example.com", "free"); err != nil {
		t.Fatal(err)
	}
	account, err := store.GetAccount(id)
	if err != nil || account.Status != "ok" || !account.IsActive || account.Cookie != "hatch_sess=new" {
		t.Fatalf("reconnected account = %+v, err %v", account, err)
	}
}

func TestLegacyAccountMigrationFailurePreservesCookies(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "webai_legacy_failure.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE webai_accounts (platform TEXT PRIMARY KEY, cookie TEXT); INSERT INTO webai_accounts VALUES ('museai', 'hatch_sess=keep-me')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := NewStore(dbPath); err == nil {
		_ = store.Close()
		t.Fatal("migration unexpectedly accepted incomplete legacy schema")
	}
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var cookie string
	if err := db.QueryRow(`SELECT cookie FROM webai_accounts WHERE platform='museai'`).Scan(&cookie); err != nil {
		t.Fatalf("legacy account table lost after failed migration: %v", err)
	}
	if cookie != "hatch_sess=keep-me" {
		t.Fatalf("legacy cookie = %q, want preserved cookie", cookie)
	}
	var staged int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='webai_accounts_v2'`).Scan(&staged); err != nil || staged != 0 {
		t.Fatalf("failed migration left staged table: count=%d err=%v", staged, err)
	}
}

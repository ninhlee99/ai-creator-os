package webai

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountJSONKeepsSessionFieldsForLegacyConfigImport(t *testing.T) {
	account := Account{SessionKey: "session-secret", Cookie: "sid=cookie-secret", CookieJar: []BrowserCookie{{Name: "sid", Value: "cookie-secret"}}}
	encoded, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Account
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionKey != account.SessionKey || decoded.Cookie != account.Cookie || len(decoded.CookieJar) != 1 {
		t.Fatalf("legacy account import lost credentials: %+v", decoded)
	}
}

func TestBrowserHeapDefaultAndBounds(t *testing.T) {
	t.Setenv("AICOS_BROWSER_MAX_HEAP_MB", "")
	if got := browserMaxHeapMB(); got != "512" {
		t.Fatalf("default browser heap = %s, want 512 MB", got)
	}
	t.Setenv("AICOS_BROWSER_MAX_HEAP_MB", "2048")
	if got := browserMaxHeapMB(); got != "2048" {
		t.Fatalf("configured browser heap = %s, want 2048 MB", got)
	}
	for _, invalid := range []string{"not-a-number", "255", "4097"} {
		t.Setenv("AICOS_BROWSER_MAX_HEAP_MB", invalid)
		if got := browserMaxHeapMB(); got != "512" {
			t.Errorf("invalid heap %q resolved to %s, want default 512 MB", invalid, got)
		}
	}
}

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

	// Save Muse AI account
	err = store.SaveAccount(Account{
		Platform:    PlatformMuseAI,
		AccountName: "Ninh Muse",
		Email:       "ninh@example.com",
		Cookie:      "hatch_sess=muse-session",
		Status:      StatusConnected,
		Tier:        "free",
	})
	if err != nil {
		t.Fatalf("SaveAccount failed: %v", err)
	}

	// Verify Muse AI account
	acc, err := store.GetAccount(PlatformMuseAI)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acc.AccountName != "Ninh Muse" || acc.Status != StatusConnected || acc.Tier != "free" {
		t.Errorf("unexpected account data: %+v", acc)
	}

	// Save Google Flow account
	err = store.SaveAccount(Account{
		Platform:    PlatformGoogleFlow,
		AccountName: "Ninh Flow",
		Email:       "flow@example.com",
		Cookie:      "SID=flow-session",
		Status:      StatusConnected,
	})
	if err != nil {
		t.Fatalf("SaveAccount Google Flow failed: %v", err)
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
	if museAccs := all[PlatformMuseAI]; len(museAccs) == 0 || museAccs[0].Status != StatusConnected {
		t.Errorf("expected Muse connected: %+v", museAccs)
	}
	if flowAccs := all[PlatformGoogleFlow]; len(flowAccs) == 0 || flowAccs[0].Status != StatusConnected {
		t.Errorf("expected Flow connected: %+v", flowAccs)
	}

	// Disconnect Muse AI
	err = store.DisconnectAccount(PlatformMuseAI)
	if err != nil {
		t.Fatalf("DisconnectAccount failed: %v", err)
	}
	acc, err = store.GetAccount(PlatformMuseAI)
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

	// Add 2 accounts for Google Flow
	_, err = store.AddAccount(Account{
		Platform:    PlatformGoogleFlow,
		AccountName: "Flow Acc 1",
		Email:       "g1@example.com",
		Cookie:      "SID=flow-1",
	})
	if err != nil {
		t.Fatalf("AddAccount 1 failed: %v", err)
	}

	_, err = store.AddAccount(Account{
		Platform:    PlatformGoogleFlow,
		AccountName: "Flow Acc 2",
		Email:       "g2@example.com",
		Cookie:      "SID=flow-2",
	})
	if err != nil {
		t.Fatalf("AddAccount 2 failed: %v", err)
	}

	// First pick
	acc1, err := store.PickActiveAccount(PlatformGoogleFlow)
	if err != nil {
		t.Fatalf("expected account picked, got err: %v", err)
	}

	// Rate limit the picked account for 1 hour
	if err := store.MarkRateLimited(acc1.ID, 1*time.Hour); err != nil {
		t.Fatalf("MarkRateLimited failed: %v", err)
	}

	// Next pick should automatically rotate to the other account
	acc2, err := store.PickActiveAccount(PlatformGoogleFlow)
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

	ready, cooldown, total := store.CountByPlatform(PlatformGoogleFlow)
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

	id1, err := store.UpsertAccountSession(PlatformGoogleFlow, "session-one", "SID=cookie-one", "Google", " Creator@Example.com ", "free")
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	id2, err := store.UpsertAccountSession(PlatformGoogleFlow, "session-two", "SID=cookie-two", "Google creator", "creator@example.com", "free")
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("same platform email created two accounts: %q != %q", id1, id2)
	}
	accounts, err := store.GetAccounts(PlatformGoogleFlow)
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("same email should map to one account, got %d", len(accounts))
	}
	if accounts[0].Email != "creator@example.com" || accounts[0].SessionKey != "session-two" || accounts[0].Cookie != "SID=cookie-two" {
		t.Fatalf("canonical account did not receive latest session: %+v", accounts[0])
	}

	addID, err := store.AddAccount(Account{
		Platform: PlatformGoogleFlow, AccountName: "Updated", Email: "CREATOR@example.com",
		SessionKey: "session-three", Cookie: "SID=cookie-three", Status: StatusConnected,
	})
	if err != nil {
		t.Fatalf("AddAccount duplicate email should update existing account: %v", err)
	}
	if addID != id1 {
		t.Fatalf("AddAccount changed account ID for same email: got %q want %q", addID, id1)
	}
	accounts, err = store.GetAccounts(PlatformGoogleFlow)
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
		`, row.id, PlatformGoogleFlow, "Google", row.email, row.session, row.cookie, StatusConnected, "free", row.updated)
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
	accounts, err := store.GetAccounts(PlatformGoogleFlow)
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != "new" || accounts[0].Email != "creator@example.com" || accounts[0].Cookie != "active-cookie" {
		t.Fatalf("migration did not retain newest canonical identity: %+v", accounts)
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

package ledger

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// openRaw opens the DB file with the sqlite driver without running the
// schema/migrations, to simulate a pre-migration database.
func openRaw(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

func newTestLedger(t *testing.T) *Ledger {
	t.Helper()
	l, err := New(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func strp(s string) *string { return &s }

func TestAddGetListAccount(t *testing.T) {
	l := newTestLedger(t)

	id1, err := l.AddAccount("alice", "cooking", "ALICE_RTMP")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	id2, err := l.AddAccount("bob", "fitness", "BOB_RTMP")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected distinct ids, got %d and %d", id1, id2)
	}

	a, err := l.GetAccount(id1)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if a.Username != "alice" || a.Status != "onboarding" {
		t.Fatalf("unexpected account: %+v", a)
	}
	if a.RestWeekday != 0 || a.Followers != 0 {
		t.Fatalf("unexpected defaults: %+v", a)
	}

	all, err := l.ListAccounts(nil)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(all) != 2 || all[0].Username != "alice" || all[1].Username != "bob" {
		t.Fatalf("unexpected list: %+v", all)
	}

	if err := l.SetAccountStatus(id1, "live", map[string]any{"persona": "teacher"}); err != nil {
		t.Fatalf("SetAccountStatus: %v", err)
	}
	live, err := l.ListAccounts([]string{"live"})
	if err != nil {
		t.Fatalf("ListAccounts(live): %v", err)
	}
	if len(live) != 1 || live[0].Username != "alice" || !live[0].Persona.Valid || live[0].Persona.String != "teacher" {
		t.Fatalf("unexpected filtered list: %+v", live)
	}
	// status filter with no matches
	none, err := l.ListAccounts([]string{"retired"})
	if err != nil || len(none) != 0 {
		t.Fatalf("expected empty, got %v, %v", none, err)
	}

	if err := l.SetFollowers(id1, 12345); err != nil {
		t.Fatalf("SetFollowers: %v", err)
	}
	a, _ = l.GetAccount(id1)
	if a.Followers != 12345 {
		t.Fatalf("followers not updated: %d", a.Followers)
	}
}

func TestAddAccountDuplicate(t *testing.T) {
	l := newTestLedger(t)
	id1, err := l.AddAccount("alice", "", "")
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	id2, err := l.AddAccount("alice", "other hint", "OTHER_RTMP")
	if err != nil {
		t.Fatalf("AddAccount duplicate: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("duplicate username must return existing id: %d != %d", id1, id2)
	}
	all, _ := l.ListAccounts(nil)
	if len(all) != 1 {
		t.Fatalf("expected 1 account, got %d", len(all))
	}
}

func TestRecordOrderIdempotent(t *testing.T) {
	l := newTestLedger(t)
	pid, err := l.AddProduct("pid-1", "Widget", 100.0, 0.1, "gadgets")
	if err != nil {
		t.Fatalf("AddProduct: %v", err)
	}
	id, err := l.RecordOrder("oid-1", map[string]any{
		"product_id": pid, "amount": 100.0, "commission": 10.0,
		"ordered_at": "2026-10-01T10:00:00",
	})
	if err != nil || id == 0 {
		t.Fatalf("RecordOrder: id=%d err=%v", id, err)
	}
	// duplicate platform_oid -> (0, nil)
	id2, err := l.RecordOrder("oid-1", map[string]any{
		"product_id": pid, "amount": 100.0, "commission": 10.0,
		"ordered_at": "2026-10-01T10:00:00",
	})
	if err != nil {
		t.Fatalf("duplicate RecordOrder must not error: %v", err)
	}
	if id2 != 0 {
		t.Fatalf("duplicate RecordOrder must return 0 id, got %d", id2)
	}
	st, err := l.ProductStats(pid)
	if err != nil {
		t.Fatalf("ProductStats: %v", err)
	}
	if st.Orders != 1 || st.Revenue != 100.0 || st.Commission != 10.0 {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

func TestRecordCommissionAndTotalRevenue(t *testing.T) {
	l := newTestLedger(t)
	if _, err := l.RecordCommission("2026-10", 150.5, "tiktok_shop_api"); err != nil {
		t.Fatalf("RecordCommission: %v", err)
	}
	if _, err := l.RecordCommission("2026-10", 49.5, "tiktok_shop_api"); err != nil {
		t.Fatalf("RecordCommission: %v", err)
	}
	rev, err := l.TotalRevenue()
	if err != nil {
		t.Fatalf("TotalRevenue: %v", err)
	}
	if rev != 200.0 {
		t.Fatalf("expected 200.0, got %f", rev)
	}
}

func TestRecordGiftAccumulates(t *testing.T) {
	l := newTestLedger(t)
	aid, _ := l.AddAccount("alice", "", "")
	sid, err := l.StartSession(&aid)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, err := l.RecordGift(aid, 100, 0.5, &sid); err != nil {
		t.Fatalf("RecordGift: %v", err)
	}
	if _, err := l.RecordGift(aid, 200, 1.0, nil); err != nil {
		t.Fatalf("RecordGift: %v", err)
	}
	sum, err := l.AccountGiftUSD(aid)
	if err != nil {
		t.Fatalf("AccountGiftUSD: %v", err)
	}
	if sum != 1.5 {
		t.Fatalf("expected 1.5, got %f", sum)
	}
	a, _ := l.GetAccount(aid)
	if a.GiftUSD != 1.5 {
		t.Fatalf("accounts.gift_usd not accumulated: %f", a.GiftUSD)
	}
	gs, err := l.GetGiftSummary()
	if err != nil {
		t.Fatalf("GetGiftSummary: %v", err)
	}
	if len(gs) != 1 || gs[0].Username != "alice" || gs[0].USD != 1.5 {
		t.Fatalf("unexpected gift summary: %+v", gs)
	}
}

func TestSlots(t *testing.T) {
	l := newTestLedger(t)
	aid, _ := l.AddAccount("alice", "", "")
	if err := l.SaveSlots([]Slot{
		{AccountID: aid, SlotDate: "2026-10-02", StartMin: 600, DurationMin: 60, Status: "planned"},
		{AccountID: aid, SlotDate: "2026-10-02", StartMin: 1200, DurationMin: 60, Status: "planned"},
	}); err != nil {
		t.Fatalf("SaveSlots: %v", err)
	}
	slots, err := l.GetSlots("2026-10-02")
	if err != nil {
		t.Fatalf("GetSlots: %v", err)
	}
	if len(slots) != 2 || slots[0].StartMin != 600 || slots[1].StartMin != 1200 {
		t.Fatalf("unexpected slots: %+v", slots)
	}
	// only the 10:00 slot is due at 11:00
	due, err := l.DueSlots("2026-10-02", 660)
	if err != nil {
		t.Fatalf("DueSlots: %v", err)
	}
	if len(due) != 1 || due[0].StartMin != 600 {
		t.Fatalf("unexpected due slots: %+v", due)
	}
	if err := l.SetSlotStatus(due[0].ID, "started"); err != nil {
		t.Fatalf("SetSlotStatus: %v", err)
	}
	due, _ = l.DueSlots("2026-10-02", 660)
	if len(due) != 0 {
		t.Fatalf("started slot must not be due: %+v", due)
	}
}

func TestSessionEventDecideUsage(t *testing.T) {
	l := newTestLedger(t)

	// session without account
	sid, err := l.StartSession(nil)
	if err != nil || sid == 0 {
		t.Fatalf("StartSession(nil): id=%d err=%v", sid, err)
	}
	if err := l.LogEvent(sid, "segment", map[string]any{"n": 1}); err != nil {
		t.Fatalf("LogEvent: %v", err)
	}
	if err := l.EndSession(sid, map[string]any{
		"status": "ended", "duration_min": 45, "peak_viewers": 100,
	}); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	if err := l.Decide("governor", "scale_product", strp("pid-1"), "good ctr",
		map[string]any{"ctr": 0.12}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := l.Decide("governor", "deny", nil, "no reason", nil); err != nil {
		t.Fatalf("Decide nil target: %v", err)
	}

	if err := l.LogUsage("llm", "openai", 100, 0.25); err != nil {
		t.Fatalf("LogUsage: %v", err)
	}
	if err := l.LogUsage("tts", "elevenlabs", 50, 0.10); err != nil {
		t.Fatalf("LogUsage: %v", err)
	}
	spend, err := l.DailySpendUSD()
	if err != nil {
		t.Fatalf("DailySpendUSD: %v", err)
	}
	if spend != 0.35 {
		t.Fatalf("expected 0.35, got %f", spend)
	}
	// a usage row from another day must not count
	if _, err := l.db.Exec(
		"INSERT INTO api_usage (engine, provider, units, cost_usd, created_at) " +
			"VALUES ('llm','x',1,99,'2000-01-01')"); err != nil {
		t.Fatalf("insert old usage: %v", err)
	}
	spend, _ = l.DailySpendUSD()
	if spend != 0.35 {
		t.Fatalf("old usage leaked into daily spend: %f", spend)
	}
}

func TestGetUsage(t *testing.T) {
	l := newTestLedger(t)
	if err := l.LogUsage("llm", "openai", 10, 1.0); err != nil {
		t.Fatal(err)
	}
	if err := l.LogUsage("tts", "elevenlabs", 5, 0.5); err != nil {
		t.Fatal(err)
	}
	rows, err := l.GetUsage("2000-01-01", 20)
	if err != nil || len(rows) != 0 {
		t.Fatalf("expected no rows for old day: %v %v", rows, err)
	}
	// today's date in SQLite terms
	var today string
	if err := l.db.QueryRow("SELECT date('now')").Scan(&today); err != nil {
		t.Fatal(err)
	}
	rows, err = l.GetUsage(today, 20)
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if len(rows) != 2 || rows[0].Engine != "tts" || rows[1].Engine != "llm" {
		t.Fatalf("unexpected usage rows: %+v", rows)
	}
	if rows[0].CostUSD != 0.5 || rows[1].Units != 10 {
		t.Fatalf("unexpected usage values: %+v", rows)
	}
}

func TestUpsertProductShelf(t *testing.T) {
	l := newTestLedger(t)
	id, err := l.UpsertProduct("pid-9", map[string]any{
		"title": "Gadget", "price": 50.0, "commission_rate": 0.2,
		"commission_value": 10.0, "score": 3.0, "status": "candidate",
	})
	if err != nil || id == 0 {
		t.Fatalf("UpsertProduct insert: id=%d err=%v", id, err)
	}
	id2, err := l.UpsertProduct("pid-9", map[string]any{
		"title": "Gadget v2", "score": 9.0, "status": "shelf",
	})
	if err != nil {
		t.Fatalf("UpsertProduct update: %v", err)
	}
	if id != id2 {
		t.Fatalf("upsert must return same id: %d != %d", id, id2)
	}
	shelf, err := l.ShelfProducts(20)
	if err != nil {
		t.Fatalf("ShelfProducts: %v", err)
	}
	if len(shelf) != 1 || shelf[0].Title != "Gadget v2" || shelf[0].Score != 9.0 {
		t.Fatalf("unexpected shelf: %+v", shelf)
	}
	if err := l.SetProductStatus(id, "killed"); err != nil {
		t.Fatalf("SetProductStatus: %v", err)
	}
	shelf, _ = l.ShelfProducts(20)
	if len(shelf) != 0 {
		t.Fatalf("killed product must not be on shelf: %+v", shelf)
	}
	prods, err := l.GetProducts()
	if err != nil || len(prods) != 1 || prods[0].Status != "killed" {
		t.Fatalf("GetProducts: %+v %v", prods, err)
	}
	// AddProduct computes commission_value
	pid, err := l.AddProduct("pid-10", "Thing", 19.99, 0.15, "home")
	if err != nil {
		t.Fatalf("AddProduct: %v", err)
	}
	prods, _ = l.GetProducts()
	var cv float64
	for _, p := range prods {
		if p.ID == pid {
			cv = p.CommissionValue
		}
	}
	if cv != 3.0 { // round(19.99*0.15, 2) = 3.0
		t.Fatalf("commission_value: expected 3.0, got %f", cv)
	}
}

// TestMigrationTwice: opening an existing DB re-runs schema + migrations
// without error (duplicate columns are ignored).
func TestMigrationTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	l1, err := New(path)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if _, err := l1.AddAccount("alice", "", ""); err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if err := l1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	l2, err := New(path)
	if err != nil {
		t.Fatalf("second New (migration re-run): %v", err)
	}
	defer l2.Close()
	all, err := l2.ListAccounts(nil)
	if err != nil || len(all) != 1 || all[0].Username != "alice" {
		t.Fatalf("data lost after re-open: %+v %v", all, err)
	}
	// migration columns usable on the re-opened DB
	if err := l2.SetAccountStatus(all[0].ID, "live", map[string]any{
		"topics_json": `["a","b"]`, "youtube_channel": "chan",
		"youtube_content_types": `["short_video"]`,
	}); err != nil {
		t.Fatalf("SetAccountStatus with migration columns: %v", err)
	}
	a, _ := l2.GetAccount(all[0].ID)
	if a.TopicsJSON != `["a","b"]` || !a.YoutubeChannel.Valid || a.YoutubeChannel.String != "chan" {
		t.Fatalf("migration columns not persisted: %+v", a)
	}
}

// TestMigrationOnLegacyDB: a DB whose accounts table predates the three
// migration columns must gain them via ALTER TABLE.
func TestMigrationOnLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	// faithful pre-migration accounts table: full current DDL minus the
	// three columns that the migrations add
	ddl := legacyAccountsDDL(t)
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create legacy accounts table: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO accounts (username) VALUES ('legacy')"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	l, err := New(path)
	if err != nil {
		t.Fatalf("New on legacy DB: %v", err)
	}
	defer l.Close()
	a, err := l.GetAccount(1)
	if err != nil {
		t.Fatalf("GetAccount on migrated DB: %v", err)
	}
	if a.Username != "legacy" || a.TopicsJSON != "[]" || a.YoutubeContentTypes != "[]" {
		t.Fatalf("migration did not add columns with defaults: %+v", a)
	}
	if a.YoutubeChannel.Valid {
		t.Fatalf("youtube_channel should be NULL: %+v", a)
	}
}

// legacyAccountsDDL returns the accounts CREATE TABLE from the embedded
// schema with the three migration-added columns stripped out.
func legacyAccountsDDL(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	for _, line := range strings.Split(schemaSQL, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, stmt := range strings.Split(sb.String(), ";") {
		trimmed := strings.TrimSpace(stmt)
		if !strings.HasPrefix(trimmed, "CREATE TABLE") ||
			!strings.Contains(trimmed, "accounts (") {
			continue
		}
		var out []string
		for _, l := range strings.Split(trimmed, "\n") {
			if strings.Contains(l, "topics_json") ||
				strings.Contains(l, "youtube_channel") ||
				strings.Contains(l, "youtube_content_types") {
				continue
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n")
	}
	t.Fatal("accounts DDL not found in embedded schema")
	return ""
}

func TestSettingsRoundTrip(t *testing.T) {
	l := newTestLedger(t)

	if err := l.SetSetting(SettingTTSChain, `{"chain":["a","b"]}`); err != nil {
		t.Fatalf("SetSetting tts.chain: %v", err)
	}
	if err := l.SetSetting(SettingLLMChain, `{"chain":["x"]}`); err != nil {
		t.Fatalf("SetSetting llm.chain: %v", err)
	}
	if err := l.SetSetting("ui.theme", "dark"); err != nil {
		t.Fatalf("SetSetting custom key: %v", err)
	}

	// overwrite an existing key: must follow the UPSERT conflict path
	if err := l.SetSetting("ui.theme", "light"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}

	v, ok, err := l.GetSetting(SettingTTSChain)
	if err != nil || !ok || v != `{"chain":["a","b"]}` {
		t.Fatalf("GetSetting tts.chain = %q, %v, %v", v, ok, err)
	}
	v, ok, err = l.GetSetting("ui.theme")
	if err != nil || !ok || v != "light" {
		t.Fatalf("GetSetting ui.theme = %q, %v, %v", v, ok, err)
	}

	all, err := l.AllSettings()
	if err != nil {
		t.Fatalf("AllSettings: %v", err)
	}
	want := map[string]string{
		SettingTTSChain: `{"chain":["a","b"]}`,
		SettingLLMChain: `{"chain":["x"]}`,
		"ui.theme":      "light",
	}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("AllSettings = %v, want %v", all, want)
	}
}

func TestSettingsMissingKey(t *testing.T) {
	l := newTestLedger(t)
	v, ok, err := l.GetSetting("no.such.key")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if ok || v != "" {
		t.Fatalf("expected (\"\", false, nil), got (%q, %v, %v)", v, ok, err)
	}
}

// TestSettingsOnLegacyDB: a DB whose schema predates the settings table must
// gain it automatically when opened with New().
func TestSettingsOnLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	// faithful old schema: the embedded schema with the settings CREATE
	// TABLE stripped out
	if err := applySchemaWithoutSettings(t, db); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	db.Close()

	l, err := New(path)
	if err != nil {
		t.Fatalf("New on legacy DB: %v", err)
	}
	defer l.Close()

	if err := l.SetSetting("k", "v"); err != nil {
		t.Fatalf("SetSetting on migrated DB: %v", err)
	}
	v, ok, err := l.GetSetting("k")
	if err != nil || !ok || v != "v" {
		t.Fatalf("GetSetting = %q, %v, %v", v, ok, err)
	}
	all, err := l.AllSettings()
	if err != nil || len(all) != 1 || all["k"] != "v" {
		t.Fatalf("AllSettings = %v, %v", all, err)
	}
}

// applySchemaWithoutSettings runs the embedded schema with the settings
// CREATE TABLE statement stripped out, simulating a pre-settings database.
func applySchemaWithoutSettings(t *testing.T, db *sql.DB) error {
	t.Helper()
	var sb strings.Builder
	for _, line := range strings.Split(schemaSQL, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, stmt := range strings.Split(sb.String(), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS settings") {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema statement: %w", err)
		}
	}
	return nil
}

func TestSessionsFeatured(t *testing.T) {
	l := newTestLedger(t)
	pid, err := l.AddProduct("pid-1", "Túi kem quilted cao cấp chính hãng", 500000, 0.18, "fashion")
	if err != nil {
		t.Fatalf("AddProduct: %v", err)
	}
	other, err := l.AddProduct("pid-2", "Ví da mini", 200000, 0.2, "fashion")
	if err != nil {
		t.Fatalf("AddProduct: %v", err)
	}
	s1, err := l.StartSession(nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	s2, err := l.StartSession(nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	// Exact product_moment events in two sessions + one duplicate in s1.
	for _, sid := range []int64{s1, s1, s2} {
		if err := l.LogEvent(sid, "product_moment", map[string]any{"product_id": pid, "product_title": "Túi kem quilted cao cấp chính hãng"}); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}
	// Legacy segment event that only embeds the title.
	if err := l.LogEvent(s1, "segment", map[string]any{"script": "Giới thiệu Túi kem quilted cao cấp hôm nay"}); err != nil {
		t.Fatalf("LogEvent legacy: %v", err)
	}
	// A different product must not be counted.
	if err := l.LogEvent(s2, "product_moment", map[string]any{"product_id": other}); err != nil {
		t.Fatalf("LogEvent other: %v", err)
	}
	n, err := l.SessionsFeatured(pid)
	if err != nil {
		t.Fatalf("SessionsFeatured: %v", err)
	}
	if n != 2 {
		t.Fatalf("SessionsFeatured = %d, want 2 distinct sessions", n)
	}
}

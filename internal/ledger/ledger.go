// Package ledger is the Go port of the Python ledger module
// (ledger/store.py + ledger/schema.sql).
//
// SQLite (WAL mode) ledger: money tables (orders, commissions, gifts) are
// append-only. Writes are serialized through a mutex, mirroring the Python
// _LockedConnection; concurrent reads are served by database/sql's pool
// (pinned to a single connection so the PRAGMAs always apply).
package ledger

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
)

//go:embed schema.sql
var schemaSQL string

// Reference embed so the import is marked used on toolchains where the
// //go:embed directive alone does not count as a use.
var _ embed.FS

// migrations are additive-only: new columns for DBs created before they
// existed. Duplicate-column errors are ignored so they are safe to re-run.
var migrations = []string{
	"ALTER TABLE accounts ADD COLUMN topics_json TEXT NOT NULL DEFAULT '[]'",
	"ALTER TABLE accounts ADD COLUMN youtube_channel TEXT",
	"ALTER TABLE accounts ADD COLUMN youtube_content_types TEXT NOT NULL DEFAULT '[]'",
}

// Ledger is a thread-safe SQLite-backed ledger. See package doc.
type Ledger struct {
	db *sql.DB
	mu sync.Mutex
}

// New opens (creating parent dirs) or creates the ledger DB at path,
// applies the schema, journal/busy-timeout pragmas and additive migrations.
func New(path string) (*Ledger, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open ledger db: %w", err)
	}
	// Mirror Python's single locked connection: one underlying connection
	// means the per-connection pragmas below always hold.
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		db.Close()
		return nil, err
	}
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if err := stampUserVersion(db, 1); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma user_version: %w", err)
	}
	l := &Ledger{db: db}
	if err := l.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return l, nil
}

// Close closes the underlying database.
func (l *Ledger) Close() error { return l.db.Close() }

// migrate runs additive migrations, ignoring "already exists" errors.
func (l *Ledger) migrate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ddl := range migrations {
		if _, err := l.db.Exec(ddl); err != nil {
			if isDuplicateColumn(err) {
				continue // column already exists
			}
			return fmt.Errorf("migration %q: %w", ddl, err)
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

// applySchema runs the embedded schema.sql statement by statement
// (the driver does not accept multi-statement scripts via Exec).
func applySchema(db *sql.DB) error {
	var sb strings.Builder
	for _, line := range strings.Split(schemaSQL, "\n") {
		// strip "--" comments (none of the DDL strings contain "--")
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
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema statement: %w", err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ---- internal helpers ----

// insertLocked builds and runs "INSERT INTO table (cols...) VALUES (?...)".
// Caller must hold l.mu.
func (l *Ledger) insertLocked(table string, data map[string]any) (int64, error) {
	cols := make([]string, 0, len(data))
	vals := make([]any, 0, len(data))
	for k, v := range data {
		cols = append(cols, k)
		vals = append(vals, normalize(v))
	}
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	res, err := l.db.Exec(
		fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			table, strings.Join(cols, ", "), strings.Join(ph, ", ")),
		vals...,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// insert is the mutex-guarded variant of insertLocked.
func (l *Ledger) insert(table string, data map[string]any) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.insertLocked(table, data)
}

// normalize converts Go values into driver values (dereferences pointers,
// widens ints to int64 / floats to float64).
func normalize(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return rv.Bytes()
		}
	}
	if s, ok := v.(string); ok {
		return s
	}
	return v
}

// toJSON mirrors Python's json.dumps(payload or {}): nil/empty -> "{}".
func toJSON(v map[string]any) (string, error) {
	if len(v) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func scanAccount(row *sql.Row) (*Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Username, &a.Status, &a.Persona, &a.Niche,
		&a.NicheHint, &a.TopicsJSON, &a.YoutubeChannel, &a.YoutubeContentTypes,
		&a.Followers, &a.RtmpKeyRef, &a.RestWeekday, &a.GiftUSD,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- domain structs ----

// Account maps the accounts table. Nullable text columns use sql.NullString.
type Account struct {
	ID                  int64
	Username            string
	Status              string
	Persona             sql.NullString
	Niche               sql.NullString
	NicheHint           sql.NullString
	TopicsJSON          string
	YoutubeChannel      sql.NullString
	YoutubeContentTypes string
	Followers           int64
	RtmpKeyRef          sql.NullString
	RestWeekday         int64
	GiftUSD             float64
	CreatedAt           string
	UpdatedAt           string
}

// Product maps the products table.
type Product struct {
	ID              int64
	PlatformPID     string
	Title           string
	Category        sql.NullString
	Price           float64
	CommissionRate  float64
	CommissionValue float64
	SellerRating    sql.NullFloat64
	Score           float64
	Status          string
	CreatedAt       string
	UpdatedAt       string
}

// Slot maps the live_slots table.
type Slot struct {
	ID          int64
	AccountID   int64
	SlotDate    string // YYYY-MM-DD
	StartMin    int    // minutes since midnight ICT
	DurationMin int
	Status      string // planned | started | done | skipped
	CreatedAt   string
}

// Decision maps the decisions table.
type Decision struct {
	ID         int64
	Agent      string
	Action     string
	Target     sql.NullString
	Reason     sql.NullString
	InputsJSON sql.NullString
	CreatedAt  string
}

// GiftSummary is one row of GetGiftSummary.
type GiftSummary struct {
	Username string
	USD      float64
}

// UsageRow is one row of GetUsage.
type UsageRow struct {
	Engine   string
	Provider string
	Units    float64
	CostUSD  float64
}

// ProductStats aggregates orders + content views for a product.
type ProductStats struct {
	Orders     int64
	Revenue    float64
	Commission float64
	Views      float64
}

// ---- products ----

// UpsertProduct inserts or updates a product by platform_pid, returning its id.
func (l *Ledger) UpsertProduct(platformPID string, fields map[string]any) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var id int64
	err := l.db.QueryRow(
		"SELECT id FROM products WHERE platform_pid = ?", platformPID).Scan(&id)
	switch {
	case err == nil:
		var sets []string
		var vals []any
		for k, v := range fields {
			if k == "platform_pid" {
				continue
			}
			sets = append(sets, k+" = ?")
			vals = append(vals, normalize(v))
		}
		vals = append(vals, platformPID)
		_, err := l.db.Exec(
			"UPDATE products SET "+strings.Join(sets, ", ")+
				", updated_at = datetime('now') WHERE platform_pid = ?",
			vals...,
		)
		if err != nil {
			return 0, err
		}
		return id, nil
	case errors.Is(err, sql.ErrNoRows):
		fields["platform_pid"] = platformPID
		return l.insertLocked("products", fields)
	default:
		return 0, err
	}
}

// SetProductStatus updates a product's status.
func (l *Ledger) SetProductStatus(productID int64, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE products SET status = ?, updated_at = datetime('now') WHERE id = ?",
		status, productID)
	return err
}

// ShelfProducts returns up to limit products with status shelf/scaled,
// best score first.
func (l *Ledger) ShelfProducts(limit int) ([]Product, error) {
	rows, err := l.db.Query(
		"SELECT id, platform_pid, title, category, price, commission_rate, "+
			"commission_value, seller_rating, score, status, created_at, updated_at "+
			"FROM products WHERE status IN ('shelf','scaled') "+
			"ORDER BY score DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

// AddProduct inserts a new product with status 'shelf', computing
// commission_value = round(price * commission_rate, 2).
func (l *Ledger) AddProduct(platformPID, title string, price, commissionRate float64, category string) (int64, error) {
	// Python: round(price * commission_rate, 2)
	cv := float64(int(price*commissionRate*100+0.5)) / 100
	return l.insert("products", map[string]any{
		"platform_pid":     platformPID,
		"title":            title,
		"category":         category,
		"price":            price,
		"commission_rate":  commissionRate,
		"commission_value": cv,
		"status":           "shelf",
	})
}

// GetProducts returns all products ordered by score DESC, id.
func (l *Ledger) GetProducts() ([]Product, error) {
	rows, err := l.db.Query(
		"SELECT id, platform_pid, title, category, price, commission_rate, " +
			"commission_value, seller_rating, score, status, created_at, updated_at " +
			"FROM products ORDER BY score DESC, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

func scanProducts(rows *sql.Rows) ([]Product, error) {
	var out []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.PlatformPID, &p.Title, &p.Category,
			&p.Price, &p.CommissionRate, &p.CommissionValue, &p.SellerRating,
			&p.Score, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- append-only money ----

// RecordOrder records an order. Idempotent: a duplicate platform_oid
// returns (0, nil) instead of an error.
func (l *Ledger) RecordOrder(platformOID string, fields map[string]any) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fields["platform_oid"] = platformOID
	id, err := l.insertLocked("orders", fields)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, nil // duplicate platform_oid: ignored, like Python
		}
		return 0, err
	}
	return id, nil
}

// RecordCommission records a commission settlement.
func (l *Ledger) RecordCommission(period string, amount float64, source string) (int64, error) {
	return l.insert("commissions", map[string]any{
		"period": period, "amount": amount, "source": source,
	})
}

// ---- sessions / events / decisions / usage ----

// StartSession opens a live session, optionally bound to an account.
func (l *Ledger) StartSession(accountID *int64) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var res sql.Result
	var err error
	if accountID == nil {
		res, err = l.db.Exec("INSERT INTO live_sessions DEFAULT VALUES")
	} else {
		res, err = l.db.Exec(
			"INSERT INTO live_sessions (account_id) VALUES (?)", *accountID)
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndSession updates arbitrary session fields (e.g. ended_at,
// duration_min, peak_viewers, status, notes).
func (l *Ledger) EndSession(sessionID int64, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	sets := make([]string, 0, len(fields))
	vals := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		sets = append(sets, k+" = ?")
		vals = append(vals, normalize(v))
	}
	vals = append(vals, sessionID)
	_, err := l.db.Exec(
		"UPDATE live_sessions SET "+strings.Join(sets, ", ")+" WHERE id = ?",
		vals...)
	return err
}

// LogEvent appends a live event with a JSON payload.
func (l *Ledger) LogEvent(sessionID int64, kind string, payload map[string]any) error {
	pj, err := toJSON(payload)
	if err != nil {
		return err
	}
	_, err = l.insert("live_events", map[string]any{
		"session_id": sessionID, "kind": kind, "payload": pj,
	})
	return err
}

// Decide records a governance decision with JSON inputs (audit trail).
func (l *Ledger) Decide(agent, action string, target *string, reason string, inputs map[string]any) error {
	ij, err := toJSON(inputs)
	if err != nil {
		return err
	}
	_, err = l.insert("decisions", map[string]any{
		"agent": agent, "action": action, "target": target,
		"reason": reason, "inputs_json": ij,
	})
	return err
}

// LogUsage meters engine usage (free-tier caps).
func (l *Ledger) LogUsage(engine, provider string, units, costUSD float64) error {
	_, err := l.insert("api_usage", map[string]any{
		"engine": engine, "provider": provider,
		"units": units, "cost_usd": costUSD,
	})
	return err
}

// DailySpendUSD returns today's total API spend in USD.
func (l *Ledger) DailySpendUSD() (float64, error) {
	var s float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd),0) AS s FROM api_usage " +
			"WHERE date(created_at) = date('now')").Scan(&s)
	return s, err
}

// ---- analyst queries ----

// ProductStats aggregates orders and content views for a product.
func (l *Ledger) ProductStats(productID int64) (ProductStats, error) {
	var st ProductStats
	err := l.db.QueryRow(
		"SELECT COUNT(*) AS orders, COALESCE(SUM(amount),0) AS revenue, "+
			"COALESCE(SUM(commission),0) AS commission "+
			"FROM orders WHERE product_id = ?", productID).
		Scan(&st.Orders, &st.Revenue, &st.Commission)
	if err != nil {
		return st, err
	}
	err = l.db.QueryRow(
		"SELECT COALESCE(SUM(views),0) AS v FROM content_items WHERE product_id = ?",
		productID).Scan(&st.Views)
	return st, err
}

// SessionsFeatured counts distinct live sessions in which a product was
// featured. A session counts when the streamer logged a product_moment
// event whose payload carries this product_id (exact JSON match); legacy
// events that only embedded the product title in the payload are matched
// on the first 20 runes of the title, mirroring the original Python query.
func (l *Ledger) SessionsFeatured(productID int64) (int, error) {
	var title string
	if err := l.db.QueryRow("SELECT title FROM products WHERE id = ?", productID).Scan(&title); err != nil {
		return 0, err
	}
	runes := []rune(title)
	if len(runes) > 20 {
		runes = runes[:20]
	}
	var n int
	err := l.db.QueryRow(
		"SELECT COUNT(DISTINCT session_id) FROM live_events WHERE "+
			"(kind = 'product_moment' AND json_extract(payload, '$.product_id') = ?) "+
			"OR payload LIKE ?",
		productID, "%"+string(runes)+"%").Scan(&n)
	return n, err
}

// ---- accounts (AI Creator Network) ----

// AddAccount registers a TikTok account. A duplicate username returns the
// existing id. The RTMP key itself is NEVER stored — only the env var name.
func (l *Ledger) AddAccount(username, nicheHint, rtmpKeyRef string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var count int64
	if err := l.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count); err != nil {
		return 0, err
	}
	id, err := l.insertLocked("accounts", map[string]any{
		"username":     username,
		"niche_hint":   nicheHint,
		"rtmp_key_ref": rtmpKeyRef,
		"rest_weekday": count % 7,
	})
	if err != nil {
		if isUniqueViolation(err) {
			var existing int64
			if qerr := l.db.QueryRow(
				"SELECT id FROM accounts WHERE username = ?", username,
			).Scan(&existing); qerr != nil {
				return 0, qerr
			}
			return existing, nil
		}
		return 0, err
	}
	return id, nil
}

// GetAccount returns one account by id (sql.ErrNoRows if missing).
func (l *Ledger) GetAccount(accountID int64) (*Account, error) {
	return scanAccount(l.db.QueryRow(
		"SELECT id, username, status, persona, niche, niche_hint, topics_json, "+
			"youtube_channel, youtube_content_types, followers, rtmp_key_ref, "+
			"rest_weekday, gift_usd, created_at, updated_at "+
			"FROM accounts WHERE id = ?", accountID))
}

// ListAccounts lists accounts, optionally filtered by status.
func (l *Ledger) ListAccounts(statuses []string) ([]Account, error) {
	var rows *sql.Rows
	var err error
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		args := make([]any, len(statuses))
		for i, s := range statuses {
			ph[i] = "?"
			args[i] = s
		}
		rows, err = l.db.Query(
			"SELECT id, username, status, persona, niche, niche_hint, topics_json, "+
				"youtube_channel, youtube_content_types, followers, rtmp_key_ref, "+
				"rest_weekday, gift_usd, created_at, updated_at "+
				"FROM accounts WHERE status IN ("+strings.Join(ph, ",")+") ORDER BY id",
			args...)
	} else {
		rows, err = l.db.Query(
			"SELECT id, username, status, persona, niche, niche_hint, topics_json, " +
				"youtube_channel, youtube_content_types, followers, rtmp_key_ref, " +
				"rest_weekday, gift_usd, created_at, updated_at " +
				"FROM accounts ORDER BY id")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Username, &a.Status, &a.Persona, &a.Niche,
			&a.NicheHint, &a.TopicsJSON, &a.YoutubeChannel, &a.YoutubeContentTypes,
			&a.Followers, &a.RtmpKeyRef, &a.RestWeekday, &a.GiftUSD,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAccountStatus updates an account's status plus any extra fields.
func (l *Ledger) SetAccountStatus(accountID int64, status string, fields map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	fields["status"] = status
	sets := make([]string, 0, len(fields))
	vals := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		sets = append(sets, k+" = ?")
		vals = append(vals, normalize(v))
	}
	vals = append(vals, accountID)
	_, err := l.db.Exec(
		"UPDATE accounts SET "+strings.Join(sets, ", ")+
			", updated_at = datetime('now') WHERE id = ?", vals...)
	return err
}

// SetFollowers updates an account's follower count.
func (l *Ledger) SetFollowers(accountID int64, followers int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE accounts SET followers = ?, updated_at = datetime('now') WHERE id = ?",
		followers, accountID)
	return err
}

// ---- characters (avatar identities) ----

// Character is one AI persona's visual identity. It mirrors
// internal/engines/avatar.Character (the web package must not import
// internal/engines); the adapter in cmd/aicos converts between them.
type Character struct {
	ID             int64
	Name           string
	ReferenceImage string
	Seed           int64
	IdentityLock   string
	VoicePreset    string
	Notes          string
	CreatedAt      string
}

const characterColumns = "id, name, reference_image, seed, identity_lock, voice_preset, notes, created_at"

func scanCharacter(row *sql.Row) (*Character, error) {
	var c Character
	if err := row.Scan(&c.ID, &c.Name, &c.ReferenceImage, &c.Seed,
		&c.IdentityLock, &c.VoicePreset, &c.Notes, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func scanCharacterRows(rows *sql.Rows) ([]Character, error) {
	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(&c.ID, &c.Name, &c.ReferenceImage, &c.Seed,
			&c.IdentityLock, &c.VoicePreset, &c.Notes, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCharacter stores a new avatar identity. The identity lock must
// already be computed by the caller (sha256 of image bytes + seed).
func (l *Ledger) CreateCharacter(name, referenceImage string, seed int64, identityLock, voicePreset, notes string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.insertLocked("characters", map[string]any{
		"name":            name,
		"reference_image": referenceImage,
		"seed":            seed,
		"identity_lock":   identityLock,
		"voice_preset":    voicePreset,
		"notes":           notes,
	})
}

// GetCharacter returns one character by id (sql.ErrNoRows if missing).
func (l *Ledger) GetCharacter(id int64) (*Character, error) {
	return scanCharacter(l.db.QueryRow(
		"SELECT "+characterColumns+" FROM characters WHERE id = ?", id))
}

// ListCharacters returns all characters in creation order.
func (l *Ledger) ListCharacters() ([]Character, error) {
	rows, err := l.db.Query("SELECT " + characterColumns + " FROM characters ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCharacterRows(rows)
}

// UpdateCharacter updates a character's editable fields.
func (l *Ledger) UpdateCharacter(id int64, name string, seed int64, identityLock, voicePreset, notes string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE characters SET name = ?, seed = ?, identity_lock = ?, "+
			"voice_preset = ?, notes = ? WHERE id = ?",
		name, seed, identityLock, voicePreset, notes, id)
	return err
}

// DeleteCharacter removes a character. The reference image file on disk is
// left alone (the caller may clean it up).
func (l *Ledger) DeleteCharacter(id int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

// ---- gifts (append-only, provider evidence) ----

// RecordGift records gift revenue and accumulates accounts.gift_usd.
func (l *Ledger) RecordGift(accountID int64, diamonds int64, usd float64, sessionID *int64) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var sess any
	if sessionID != nil {
		sess = *sessionID
	}
	res, err := tx.Exec(
		"INSERT INTO gifts (account_id, session_id, diamonds, usd) VALUES (?,?,?,?)",
		accountID, sess, diamonds, usd)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		"UPDATE accounts SET gift_usd = gift_usd + ? WHERE id = ?", usd, accountID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AccountGiftUSD returns lifetime gift USD summed from the gifts table.
func (l *Ledger) AccountGiftUSD(accountID int64) (float64, error) {
	var s float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(usd),0) AS s FROM gifts WHERE account_id = ?",
		accountID).Scan(&s)
	return s, err
}

// GetGiftSummary returns per-account gift revenue, richest first.
func (l *Ledger) GetGiftSummary() ([]GiftSummary, error) {
	rows, err := l.db.Query(
		"SELECT a.username AS username, COALESCE(SUM(g.usd), 0) AS usd " +
			"FROM accounts a LEFT JOIN gifts g ON g.account_id = a.id " +
			"GROUP BY a.id ORDER BY usd DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GiftSummary
	for rows.Next() {
		var g GiftSummary
		if err := rows.Scan(&g.Username, &g.USD); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---- live slots ----

// SaveSlots persists scheduler-produced slots.
func (l *Ledger) SaveSlots(slots []Slot) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range slots {
		if _, err := tx.Exec(
			"INSERT INTO live_slots (account_id, slot_date, start_min, duration_min, status) "+
				"VALUES (?,?,?,?,?)",
			s.AccountID, s.SlotDate, s.StartMin, s.DurationMin, s.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSlots returns all slots for a date, ordered by start_min.
func (l *Ledger) GetSlots(slotDate string) ([]Slot, error) {
	rows, err := l.db.Query(
		"SELECT id, account_id, slot_date, start_min, duration_min, status, created_at "+
			"FROM live_slots WHERE slot_date = ? ORDER BY start_min", slotDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSlots(rows)
}

// DueSlots returns planned slots for a date whose start time has arrived.
func (l *Ledger) DueSlots(slotDate string, nowMin int) ([]Slot, error) {
	rows, err := l.db.Query(
		"SELECT id, account_id, slot_date, start_min, duration_min, status, created_at "+
			"FROM live_slots WHERE slot_date = ? AND status = 'planned' "+
			"AND start_min <= ? ORDER BY start_min", slotDate, nowMin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSlots(rows)
}

// SetSlotStatus updates a slot's status.
func (l *Ledger) SetSlotStatus(slotID int64, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"UPDATE live_slots SET status = ? WHERE id = ?", status, slotID)
	return err
}

func scanSlots(rows *sql.Rows) ([]Slot, error) {
	var out []Slot
	for rows.Next() {
		var s Slot
		if err := rows.Scan(&s.ID, &s.AccountID, &s.SlotDate, &s.StartMin,
			&s.DurationMin, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- dashboard read helpers ----

// TotalRevenue returns total settled commission revenue.
func (l *Ledger) TotalRevenue() (float64, error) {
	var r float64
	err := l.db.QueryRow(
		"SELECT COALESCE(SUM(amount), 0) FROM commissions").Scan(&r)
	return r, err
}

// GetUsage returns api_usage rows for a day (YYYY-MM-DD), newest first.
func (l *Ledger) GetUsage(day string, limit int) ([]UsageRow, error) {
	rows, err := l.db.Query(
		"SELECT engine, provider, units, cost_usd FROM api_usage "+
			"WHERE date(created_at) = ? ORDER BY id DESC LIMIT ?", day, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		var u UsageRow
		if err := rows.Scan(&u.Engine, &u.Provider, &u.Units, &u.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- settings (key-value dashboard config) ----

// Settings key conventions (used by the web worker / dashboard):
//
//	tts.chain, llm.chain — JSON-serialized ChainConfig of the respective
//	engine: the ordered provider chain used when the engine runs.
//	Any other key is free-form configuration storage.
const (
	SettingTTSChain    = "tts.chain"
	SettingLLMChain    = "llm.chain"
	SettingAvatarChain = "avatar.chain"
)

// GetSetting returns the value for key. The second return is false (not an
// error) when the key does not exist yet.
func (l *Ledger) GetSetting(key string) (string, bool, error) {
	var v string
	err := l.db.QueryRow(
		"SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	switch {
	case err == nil:
		return v, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	default:
		return "", false, err
	}
}

// SetSetting upserts a key/value pair, refreshing updated_at on conflict.
func (l *Ledger) SetSetting(key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value, "+
			"updated_at = datetime('now')",
		key, value)
	return err
}

// AllSettings returns every stored key/value pair (for the dashboard).
func (l *Ledger) AllSettings() (map[string]string, error) {
	rows, err := l.db.Query("SELECT key, value FROM settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// stampUserVersion records the schema generation for future migrations
// (R2-W7): a fresh database is marked with v, an existing stamp is never
// overwritten here.
func stampUserVersion(db *sql.DB, v int) error {
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		return err
	}
	if cur == 0 {
		_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
		return err
	}
	return nil
}

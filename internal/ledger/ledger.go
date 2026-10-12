package ledger

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

// DeleteSetting xóa một key khỏi bảng settings. Không có key thì no-op
// (không báo lỗi) — dùng để dọn các marker tạm như story.publishing.*.
func (l *Ledger) DeleteSetting(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.db.Exec("DELETE FROM settings WHERE key = ?", key)
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

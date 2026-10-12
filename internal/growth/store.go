package growth

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// schemaDDL creates the growth tables (docs/CHANNEL_GROWTH.md §8, with the
// account-level snapshot table named metric_snapshots). Executed at store
// construction, mirroring how internal/studio creates its own tables.
var schemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS growth_profiles (
		account_id INTEGER PRIMARY KEY,
		growth_stage TEXT NOT NULL DEFAULT 'cold_start',
		cadence_per_day REAL NOT NULL DEFAULT 1.0,
		plan_horizon_days INTEGER NOT NULL DEFAULT 30,
		started_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS growth_targets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		metric TEXT NOT NULL,
		threshold REAL NOT NULL,
		label TEXT NOT NULL,
		deadline TEXT,
		reached_at TEXT,
		UNIQUE(account_id, metric, threshold)
	)`,
	`CREATE TABLE IF NOT EXISTS content_plans (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		phase TEXT NOT NULL DEFAULT 'd30',
		status TEXT NOT NULL DEFAULT 'active',
		rationale TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS content_plan_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		plan_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		planned_for TEXT NOT NULL,
		format_id TEXT NOT NULL,
		platform_variant TEXT NOT NULL,
		topic TEXT,
		hook TEXT,
		series_ep INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'planned',
		content_item_id INTEGER,
		published_ref TEXT,
		UNIQUE(account_id, planned_for, format_id, platform_variant)
	)`,
	`CREATE TABLE IF NOT EXISTS format_stats (
		account_id INTEGER NOT NULL,
		format_id TEXT NOT NULL,
		videos INTEGER NOT NULL DEFAULT 0,
		median_completion REAL,
		median_proxy_rate REAL,
		median_views REAL,
		avg_views REAL,
		follows_per_1k REAL,
		verdict TEXT NOT NULL DEFAULT 'testing',
		verdict_reason TEXT,
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (account_id, format_id)
	)`,
	`CREATE TABLE IF NOT EXISTS metric_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		taken_at TEXT NOT NULL DEFAULT (datetime('now')),
		followers INTEGER,
		views_30d REAL,
		videos INTEGER,
		extra_json TEXT,
		source TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_metric_snapshots_acct ON metric_snapshots(account_id, taken_at)`,
	`CREATE TABLE IF NOT EXISTS video_metric_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		platform_video_id TEXT NOT NULL,
		taken_at TEXT NOT NULL DEFAULT (datetime('now')),
		views INTEGER, likes INTEGER, comments INTEGER, shares INTEGER, saves INTEGER,
		completion REAL,
		extra_json TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_video_snapshots_acct ON video_metric_snapshots(account_id, platform_video_id)`,
	`CREATE TABLE IF NOT EXISTS growth_alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER,
		severity TEXT NOT NULL,
		kind TEXT NOT NULL,
		message TEXT NOT NULL,
		action_taken TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
}

// Phase-2 columns on content_plan_items (additive migration, duplicate
// column tolerated like studio's own migrations).
var itemColumnDDL = []string{
	`ALTER TABLE content_plan_items ADD COLUMN studio_job_id TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN concept_text TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN concept_hash TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN variant_group TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN dedup_action TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE content_plan_items ADD COLUMN pub_title TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN pub_caption TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN pub_note TEXT`,
	`ALTER TABLE content_plan_items ADD COLUMN related_item_id INTEGER`,
	`ALTER TABLE content_plan_items ADD COLUMN produced_at TEXT`,
}

// Store persists growth state in the app's shared SQLite database.
type Store struct {
	db *sql.DB
}

// NewStore creates the growth tables if needed and returns the store.
func NewStore(db *sql.DB) (*Store, error) {
	for _, ddl := range schemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("growth schema: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS growth_youtube_quota (
		day TEXT PRIMARY KEY,
		units INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return nil, fmt.Errorf("growth schema: %w", err)
	}
	for _, ddl := range itemColumnDDL {
		if _, err := db.Exec(ddl); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("growth schema: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func nowStr() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

// -------------------------------------------------------------- profiles

// EnsureProfile returns the account's growth profile, creating the default
// cold_start profile (and the platform KPI ladder) on first sight. This is
// the zero-touch bootstrap: an account handed to the app starts growing
// without any human configuration step.
func (s *Store) EnsureProfile(accountID int64, platform string, hasYouTube bool) (*Profile, error) {
	p, err := s.GetProfile(accountID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	if _, err := s.db.Exec(
		`INSERT INTO growth_profiles (account_id, growth_stage) VALUES (?, ?)`,
		accountID, StageColdStart); err != nil {
		return nil, err
	}
	for _, t := range DefaultTargets(platform) {
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO growth_targets (account_id, metric, threshold, label, deadline)
			 VALUES (?, ?, ?, ?, ?)`,
			accountID, t.Metric, t.Threshold, t.Label, nullStr(t.Deadline)); err != nil {
			return nil, err
		}
	}
	if hasYouTube {
		for _, t := range DefaultTargets("youtube") {
			if _, err := s.db.Exec(
				`INSERT OR IGNORE INTO growth_targets (account_id, metric, threshold, label, deadline)
				 VALUES (?, ?, ?, ?, ?)`,
				accountID, t.Metric, t.Threshold, t.Label, nullStr(t.Deadline)); err != nil {
				return nil, err
			}
		}
	}
	return s.GetProfile(accountID)
}

// GetProfile returns nil (no error) when the account has no profile yet.
func (s *Store) GetProfile(accountID int64) (*Profile, error) {
	row := s.db.QueryRow(
		`SELECT account_id, growth_stage, cadence_per_day, plan_horizon_days, started_at, updated_at
		 FROM growth_profiles WHERE account_id = ?`, accountID)
	var p Profile
	err := row.Scan(&p.AccountID, &p.Stage, &p.CadenceDay, &p.HorizonDays, &p.StartedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetStage updates the growth stage (growth-owned table; account lifecycle
// status stays with the account manager).
func (s *Store) SetStage(accountID int64, stage string) error {
	_, err := s.db.Exec(
		`UPDATE growth_profiles SET growth_stage = ?, updated_at = ? WHERE account_id = ?`,
		stage, nowStr(), accountID)
	return err
}

// SetCadence records the system's current cadence decision.
func (s *Store) SetCadence(accountID int64, perDay float64) error {
	_, err := s.db.Exec(
		`UPDATE growth_profiles SET cadence_per_day = ?, updated_at = ? WHERE account_id = ?`,
		perDay, nowStr(), accountID)
	return err
}

// --------------------------------------------------------------- targets

// ListTargets returns the account's KPI ladder in threshold order.

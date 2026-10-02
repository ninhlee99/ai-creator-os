package growth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
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
func (s *Store) ListTargets(accountID int64) ([]Target, error) {
	rows, err := s.db.Query(
		`SELECT id, account_id, metric, threshold, label, COALESCE(deadline,''), COALESCE(reached_at,'')
		 FROM growth_targets WHERE account_id = ? ORDER BY metric, threshold`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Target
	for rows.Next() {
		var t Target
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Metric, &t.Threshold, &t.Label, &t.Deadline, &t.ReachedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkTargetsReached stamps reached_at on every milestone the given metric
// readings satisfy. Returns the labels newly reached (for milestone alerts).
func (s *Store) MarkTargetsReached(accountID int64, readings map[string]float64) ([]string, error) {
	targets, err := s.ListTargets(accountID)
	if err != nil {
		return nil, err
	}
	var reached []string
	for _, t := range targets {
		if t.ReachedAt != "" {
			continue
		}
		v, ok := readings[t.Metric]
		if !ok || v < t.Threshold {
			continue
		}
		if _, err := s.db.Exec(
			`UPDATE growth_targets SET reached_at = ? WHERE id = ?`, nowStr(), t.ID); err != nil {
			return reached, err
		}
		reached = append(reached, t.Label)
	}
	return reached, nil
}

// ----------------------------------------------------------------- plans

// InsertPlan supersedes the account's previous active plan and stores the
// new rolling plan + items (history is preserved, never rewritten).
func (s *Store) InsertPlan(accountID int64, phase, rationale string, drafts []PlanDraft) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE content_plans SET status = 'superseded' WHERE account_id = ? AND status = 'active'`,
		accountID); err != nil {
		return 0, err
	}
	// Discard the superseded plan's unexecuted items so the new plan can
	// schedule the same (day, format, variant) slots — the UNIQUE index
	// guards the ACTIVE plan against duplicate scheduling, and published
	// history is kept untouched.
	if _, err := tx.Exec(
		`DELETE FROM content_plan_items
		 WHERE account_id = ? AND status = 'planned'
		   AND plan_id IN (SELECT id FROM content_plans WHERE account_id = ? AND status = 'superseded')`,
		accountID, accountID); err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO content_plans (account_id, phase, rationale) VALUES (?, ?, ?)`,
		accountID, phase, rationale)
	if err != nil {
		return 0, err
	}
	planID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, d := range drafts {
		group := d.Group
		if group == "" {
			group = d.Date + "|" + d.FormatID
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO content_plan_items
			 (plan_id, account_id, planned_for, format_id, platform_variant, topic, hook, series_ep, status, variant_group)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'planned', ?)`,
			planID, accountID, d.Date, d.FormatID, d.Variant, d.Topic, d.Hook, d.SeriesEp, group); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return planID, nil
}

// ActivePlan returns the current plan and its items (nil plan when none).
func (s *Store) ActivePlan(accountID int64) (*Plan, []PlanItem, error) {
	row := s.db.QueryRow(
		`SELECT id, account_id, phase, status, COALESCE(rationale,''), created_at
		 FROM content_plans WHERE account_id = ? AND status = 'active' ORDER BY id DESC LIMIT 1`, accountID)
	var p Plan
	err := row.Scan(&p.ID, &p.AccountID, &p.Phase, &p.Status, &p.Rationale, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	items, err := s.planItems(p.ID)
	return &p, items, err
}

const planItemCols = `id, plan_id, account_id, planned_for, format_id, platform_variant,
	COALESCE(topic,''), COALESCE(hook,''), series_ep, status, COALESCE(content_item_id,0),
	COALESCE(published_ref,''), COALESCE(studio_job_id,''), COALESCE(concept_text,''),
	COALESCE(concept_hash,''), COALESCE(variant_group,''), COALESCE(dedup_action,''),
	attempts, COALESCE(pub_title,''), COALESCE(pub_caption,''), COALESCE(pub_note,''),
	COALESCE(related_item_id,0), COALESCE(produced_at,'')`

func scanPlanItem(rows *sql.Rows) (PlanItem, error) {
	var it PlanItem
	err := rows.Scan(&it.ID, &it.PlanID, &it.AccountID, &it.PlannedFor, &it.FormatID,
		&it.Variant, &it.Topic, &it.Hook, &it.SeriesEp, &it.Status, &it.ContentItemID,
		&it.PublishedRef, &it.StudioJobID, &it.ConceptText, &it.ConceptHash,
		&it.VariantGroup, &it.DedupAction, &it.Attempts, &it.PubTitle, &it.PubCaption,
		&it.PubNote, &it.RelatedItemID, &it.ProducedAt)
	return it, err
}

func (s *Store) planItems(planID int64) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items WHERE plan_id = ? ORDER BY planned_for, id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpcomingItems returns the next planned items from today (inclusive).
func (s *Store) UpcomingItems(accountID int64, today string, limit int) ([]PlanItem, error) {
	_, items, err := s.ActivePlan(accountID)
	if err != nil {
		return nil, err
	}
	var out []PlanItem
	for _, it := range items {
		if it.Status == ItemPlanned && it.PlannedFor >= today {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// --------------------------------------------------- production (ph. 2)

// ActivePlanItems returns every item of the account's active plan (any
// status), oldest scheduled first.
func (s *Store) ActivePlanItems(accountID int64) ([]PlanItem, error) {
	_, items, err := s.ActivePlan(accountID)
	return items, err
}

// PlannedItemsDue returns still-planned items scheduled on or before
// today (the caller applies the per-variant schedule lag and caps).
func (s *Store) PlannedItemsDue(accountID int64, today string, limit int) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE account_id = ? AND status = 'planned' AND planned_for <= ?
		 ORDER BY planned_for, id LIMIT ?`, accountID, today, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ItemsByStatus returns the account's items in any of the given statuses,
// across all plans (production history included), newest activity last.
func (s *Store) ItemsByStatus(accountID int64, statuses []string, limit int) ([]PlanItem, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	ph := make([]string, len(statuses))
	args := []any{accountID}
	for i, st := range statuses {
		ph[i] = "?"
		args = append(args, st)
	}
	args = append(args, limit)
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE account_id = ? AND status IN (`+strings.Join(ph, ",")+`)
		 ORDER BY planned_for, id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetItem returns one plan item (nil when absent).
func (s *Store) GetItem(id int64) (*PlanItem, error) {
	rows, err := s.db.Query(`SELECT `+planItemCols+` FROM content_plan_items WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	it, err := scanPlanItem(rows)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

// SetItemConcept stamps the dedup fingerprint on an item.
func (s *Store) SetItemConcept(id int64, conceptText, conceptHash string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET concept_text = ?, concept_hash = ? WHERE id = ?`,
		conceptText, conceptHash, id)
	return err
}

// SetItemDedup records a forced re-generation: new topic + concept and the
// dedup_action marker for the UI lineage display.
func (s *Store) SetItemDedup(id int64, topic, conceptText, conceptHash string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET topic = ?, concept_text = ?, concept_hash = ?,
		   dedup_action = 'angle_regenerated' WHERE id = ?`,
		topic, conceptText, conceptHash, id)
	return err
}

// SetItemProduction links the Studio job and the per-variant publish
// metadata, moving the item to producing.
func (s *Store) SetItemProduction(id int64, jobID, pubTitle, pubCaption string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'producing', studio_job_id = ?,
		   pub_title = ?, pub_caption = ? WHERE id = ?`,
		jobID, pubTitle, pubCaption, id)
	return err
}

// NoteItemFailure records a failed enqueue attempt (item stays planned and
// will be retried next tick). Returns the new attempt count.
func (s *Store) NoteItemFailure(id int64, note string) (int, error) {
	if _, err := s.db.Exec(
		`UPDATE content_plan_items SET attempts = attempts + 1, pub_note = ? WHERE id = ?`,
		note, id); err != nil {
		return 0, err
	}
	it, err := s.GetItem(id)
	if err != nil || it == nil {
		return 0, err
	}
	return it.Attempts, nil
}

// SetItemStatus moves an item to a status with an honest state note.
func (s *Store) SetItemStatus(id int64, status, note string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = ?, pub_note = ? WHERE id = ?`,
		status, note, id)
	return err
}

// SetItemProduced marks the render finished (produced_at stamped).
func (s *Store) SetItemProduced(id int64, note string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'produced', produced_at = ?, pub_note = ? WHERE id = ?`,
		nowStr(), note, id)
	return err
}

// SetItemPublished records the platform video id after a real upload.
func (s *Store) SetItemPublished(id int64, ref string) error {
	_, err := s.db.Exec(
		`UPDATE content_plan_items SET status = 'published', published_ref = ?, pub_note = '' WHERE id = ?`,
		ref, id)
	return err
}

// LinkRelatedItems records the sibling relation (Short <-> long-form)
// both ways.
func (s *Store) LinkRelatedItems(aID, bID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE content_plan_items SET related_item_id = ? WHERE id = ?`, bID, aID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE content_plan_items SET related_item_id = ? WHERE id = ?`, aID, bID); err != nil {
		return err
	}
	return tx.Commit()
}

// RecentNetworkItems returns recently produced/published items across the
// whole network (the dedup guard's comparison set). produced_at falls back
// to the plan date for items still rendering.
func (s *Store) RecentNetworkItems(sinceDate string, limit int) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE COALESCE(concept_hash,'') != ''
		   AND status IN ('producing','produced','waiting_connect','waiting_quota','published')
		   AND COALESCE(NULLIF(produced_at,''), planned_for) >= ?
		 ORDER BY id DESC LIMIT ?`, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		it, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ------------------------------------------------------------- YT quota

// QuotaUsed returns the YouTube Data API units charged on a day
// (YYYY-MM-DD, ICT).
func (s *Store) QuotaUsed(day string) (int, error) {
	var units int
	err := s.db.QueryRow(
		`SELECT COALESCE(units,0) FROM growth_youtube_quota WHERE day = ?`, day).Scan(&units)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return units, err
}

// AddQuota charges units to a day's YouTube quota counter.
func (s *Store) AddQuota(day string, units int) error {
	_, err := s.db.Exec(
		`INSERT INTO growth_youtube_quota (day, units) VALUES (?, ?)
		 ON CONFLICT(day) DO UPDATE SET units = units + excluded.units`, day, units)
	return err
}

// ------------------------------------------------------------- snapshots

// InsertSnapshot appends one provider reading. Append-only: old readings
// are never updated (doc §8 data principle).
func (s *Store) InsertSnapshot(snap Snapshot) error {
	var followers, videos any
	var views30d any
	if snap.Followers != nil {
		followers = *snap.Followers
	}
	if snap.Videos != nil {
		videos = *snap.Videos
	}
	if snap.Views30d != nil {
		views30d = *snap.Views30d
	}
	var extra any
	if len(snap.Extra) > 0 {
		b, err := json.Marshal(snap.Extra)
		if err != nil {
			return err
		}
		extra = string(b)
	}
	taken := snap.TakenAt
	if taken.IsZero() {
		taken = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO metric_snapshots (account_id, taken_at, followers, views_30d, videos, extra_json, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		snap.AccountID, taken.UTC().Format("2006-01-02 15:04:05"),
		followers, views30d, videos, extra, snap.Source)
	return err
}

func scanSnapshot(rows *sql.Rows) (Snapshot, bool, error) {
	var snap Snapshot
	var followers, videos sql.NullInt64
	var views30d sql.NullFloat64
	var extra sql.NullString
	var taken string
	if err := rows.Scan(&snap.AccountID, &taken, &followers, &views30d, &videos, &extra, &snap.Source); err != nil {
		return snap, false, err
	}
	if t, err := time.Parse("2006-01-02 15:04:05", taken); err == nil {
		snap.TakenAt = t
	}
	if followers.Valid {
		v := followers.Int64
		snap.Followers = &v
	}
	if videos.Valid {
		v := videos.Int64
		snap.Videos = &v
	}
	if views30d.Valid {
		v := views30d.Float64
		snap.Views30d = &v
	}
	if extra.Valid && extra.String != "" {
		_ = json.Unmarshal([]byte(extra.String), &snap.Extra)
	}
	return snap, true, nil
}

const snapshotCols = `account_id, taken_at, followers, views_30d, videos, extra_json, source`

// LatestSnapshot returns the account's newest reading (nil when none).
func (s *Store) LatestSnapshot(accountID int64) (*Snapshot, error) {
	rows, err := s.db.Query(
		`SELECT `+snapshotCols+` FROM metric_snapshots WHERE account_id = ? ORDER BY id DESC LIMIT 1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	snap, _, err := scanSnapshot(rows)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// SnapshotSeries returns recent snapshots oldest-first (sparkline input).
func (s *Store) SnapshotSeries(accountID int64, limit int) ([]Snapshot, error) {
	rows, err := s.db.Query(
		`SELECT `+snapshotCols+` FROM metric_snapshots WHERE account_id = ? ORDER BY id DESC LIMIT ?`,
		accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		snap, _, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Fetched newest-first by id; reverse into oldest-first. (taken_at has
	// one-second resolution, so id order is the reliable sequence.)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ------------------------------------------------------------ video stats

// latestVideoRows returns the newest metric row per published video of the
// account, joined to the plan item that published it (for its format).
type videoRow struct {
	VideoID    string
	FormatID   string
	Views      int64
	Shares     int64
	Saves      int64
	Completion *float64
	TakenAt    time.Time
}

func (s *Store) latestVideoRows(accountID int64) ([]videoRow, error) {
	rows, err := s.db.Query(
		`SELECT v.platform_video_id, COALESCE(i.format_id,''), v.views, v.shares, v.saves, v.completion, v.taken_at
		 FROM video_metric_snapshots v
		 LEFT JOIN content_plan_items i
		   ON i.account_id = v.account_id AND i.published_ref = v.platform_video_id
		 WHERE v.id IN (SELECT MAX(id) FROM video_metric_snapshots WHERE account_id = ? GROUP BY platform_video_id)
		 ORDER BY v.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []videoRow
	for rows.Next() {
		var r videoRow
		var views, shares, saves sql.NullInt64
		var completion sql.NullFloat64
		var taken string
		if err := rows.Scan(&r.VideoID, &r.FormatID, &views, &shares, &saves, &completion, &taken); err != nil {
			return nil, err
		}
		r.Views = views.Int64
		r.Shares = shares.Int64
		r.Saves = saves.Int64
		if completion.Valid {
			v := completion.Float64
			r.Completion = &v
		}
		if t, err := time.Parse("2006-01-02 15:04:05", taken); err == nil {
			r.TakenAt = t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// accountMedianViews is the median views across the account's videos.
func accountMedianViews(rows []videoRow) float64 {
	xs := make([]float64, 0, len(rows))
	for _, r := range rows {
		xs = append(xs, float64(r.Views))
	}
	return Median(xs)
}

// ------------------------------------------------------------ format stats

// ListFormatStats returns stored format scores for the scoreboard.
func (s *Store) ListFormatStats(accountID int64) ([]FormatStat, error) {
	rows, err := s.db.Query(
		`SELECT account_id, format_id, videos, median_completion, median_proxy_rate,
		        COALESCE(median_views,0), COALESCE(avg_views,0), follows_per_1k, verdict,
		        COALESCE(verdict_reason,''), updated_at
		 FROM format_stats WHERE account_id = ? ORDER BY videos DESC, format_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FormatStat
	for rows.Next() {
		var f FormatStat
		var mc, mp, fp sql.NullFloat64
		if err := rows.Scan(&f.AccountID, &f.FormatID, &f.Videos, &mc, &mp,
			&f.MedianViews, &f.AvgViews, &fp, &f.Verdict, &f.VerdictReason, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if mc.Valid {
			v := mc.Float64
			f.MedianCompletion = &v
		}
		if mp.Valid {
			v := mp.Float64
			f.MedianProxyRate = &v
		}
		if fp.Valid {
			v := fp.Float64
			f.FollowsPer1k = &v
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) upsertFormatStat(f FormatStat) error {
	var mc, mp, fp any
	if f.MedianCompletion != nil {
		mc = *f.MedianCompletion
	}
	if f.MedianProxyRate != nil {
		mp = *f.MedianProxyRate
	}
	if f.FollowsPer1k != nil {
		fp = *f.FollowsPer1k
	}
	_, err := s.db.Exec(
		`INSERT INTO format_stats (account_id, format_id, videos, median_completion, median_proxy_rate,
		        median_views, avg_views, follows_per_1k, verdict, verdict_reason, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, format_id) DO UPDATE SET
		   videos = excluded.videos, median_completion = excluded.median_completion,
		   median_proxy_rate = excluded.median_proxy_rate, median_views = excluded.median_views,
		   avg_views = excluded.avg_views, follows_per_1k = excluded.follows_per_1k,
		   verdict = excluded.verdict, verdict_reason = excluded.verdict_reason,
		   updated_at = excluded.updated_at`,
		f.AccountID, f.FormatID, f.Videos, mc, mp, f.MedianViews, f.AvgViews, fp,
		f.Verdict, f.VerdictReason, nowStr())
	return err
}

// RecomputeFormatStats rebuilds format_stats from the latest per-video
// metric rows. Videos with no format link (published outside a plan) are
// skipped — unattributed stats would poison kill decisions.
func (s *Store) RecomputeFormatStats(accountID int64) ([]FormatStat, error) {
	rows, err := s.latestVideoRows(accountID)
	if err != nil {
		return nil, err
	}
	byFormat := map[string][]videoRow{}
	for _, r := range rows {
		if r.FormatID == "" {
			continue
		}
		byFormat[r.FormatID] = append(byFormat[r.FormatID], r)
	}
	var out []FormatStat
	for fid, vids := range byFormat {
		var completions, proxies, views []float64
		var sum float64
		for _, v := range vids {
			views = append(views, float64(v.Views))
			sum += float64(v.Views)
			if v.Completion != nil {
				completions = append(completions, *v.Completion)
			}
			if v.Views > 0 {
				proxies = append(proxies, float64(v.Shares+v.Saves)/float64(v.Views))
			}
		}
		existing, _ := s.formatStat(accountID, fid)
		stat := FormatStat{
			AccountID:   accountID,
			FormatID:    fid,
			Videos:      len(vids),
			MedianViews: Median(views),
		}
		if len(vids) > 0 {
			stat.AvgViews = sum / float64(len(vids))
		}
		if len(completions) > 0 {
			m := Median(completions)
			stat.MedianCompletion = &m
		}
		if len(proxies) > 0 {
			m := Median(proxies)
			stat.MedianProxyRate = &m
		}
		if existing != nil {
			stat.Verdict = existing.Verdict
			stat.VerdictReason = existing.VerdictReason
			stat.FollowsPer1k = existing.FollowsPer1k
		}
		if stat.Verdict == "" {
			stat.Verdict = VerdictTesting
		}
		if err := s.upsertFormatStat(stat); err != nil {
			return nil, err
		}
		out = append(out, stat)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FormatID < out[j].FormatID })
	return out, nil
}

func (s *Store) formatStat(accountID int64, formatID string) (*FormatStat, error) {
	stats, err := s.ListFormatStats(accountID)
	if err != nil {
		return nil, err
	}
	for i := range stats {
		if stats[i].FormatID == formatID {
			return &stats[i], nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------- alerts

// InsertAlert records one notify-only alert with the action already taken.
func (s *Store) InsertAlert(a Alert) error {
	var acct any
	if a.AccountID != 0 {
		acct = a.AccountID
	}
	_, err := s.db.Exec(
		`INSERT INTO growth_alerts (account_id, severity, kind, message, action_taken)
		 VALUES (?, ?, ?, ?, ?)`,
		acct, a.Severity, a.Kind, a.Message, a.ActionTaken)
	return err
}

// ListAlerts returns recent alerts, newest first (accountID 0 = all).
func (s *Store) ListAlerts(accountID int64, limit int) ([]Alert, error) {
	q := `SELECT id, COALESCE(account_id,0), severity, kind, message, COALESCE(action_taken,''), created_at
	      FROM growth_alerts`
	var args []any
	if accountID != 0 {
		q += ` WHERE account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Severity, &a.Kind, &a.Message, &a.ActionTaken, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Decide writes one row into the shared decisions audit trail (same table
// the ledger's Decide writes), so growth decisions are auditable next to
// money decisions.
func (s *Store) Decide(agent, action, target, reason string, inputs map[string]any) error {
	var raw any
	if inputs != nil {
		b, err := json.Marshal(inputs)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO decisions (agent, action, target, reason, inputs_json) VALUES (?, ?, ?, ?, ?)`,
		agent, action, target, reason, raw)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var _ = strings.TrimSpace // keep strings import if helpers shrink

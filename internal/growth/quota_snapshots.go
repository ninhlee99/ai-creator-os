package growth

import (
	"database/sql"
	"encoding/json"
	"time"
)

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

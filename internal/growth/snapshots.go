package growth

import (
	"database/sql"
	"encoding/json"
	"time"
)

// RecentNetworkItems returns recent produced items for the shared duplicate guard.
func (s *Store) RecentNetworkItems(sinceDate string, limit int) ([]PlanItem, error) {
	rows, err := s.db.Query(
		`SELECT `+planItemCols+` FROM content_plan_items
		 WHERE COALESCE(concept_hash,'') != ''
		   AND status IN ('producing','produced','published')
		   AND COALESCE(NULLIF(produced_at,''), planned_for) >= ?
		 ORDER BY id DESC LIMIT ?`, sinceDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanItem
	for rows.Next() {
		item, err := scanPlanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// InsertSnapshot appends one provider reading; absent metrics remain NULL.
func (s *Store) InsertSnapshot(snapshot Snapshot) error {
	var followers, videos, views30d any
	if snapshot.Followers != nil {
		followers = *snapshot.Followers
	}
	if snapshot.Videos != nil {
		videos = *snapshot.Videos
	}
	if snapshot.Views30d != nil {
		views30d = *snapshot.Views30d
	}
	var extra any
	if len(snapshot.Extra) > 0 {
		encoded, err := json.Marshal(snapshot.Extra)
		if err != nil {
			return err
		}
		extra = string(encoded)
	}
	taken := snapshot.TakenAt
	if taken.IsZero() {
		taken = time.Now()
	}
	_, err := s.db.Exec(`INSERT INTO metric_snapshots
		(account_id, taken_at, followers, views_30d, videos, extra_json, source)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, snapshot.AccountID,
		taken.UTC().Format("2006-01-02 15:04:05"), followers, views30d, videos, extra, snapshot.Source)
	return err
}

func scanSnapshot(rows *sql.Rows) (Snapshot, error) {
	var snapshot Snapshot
	var followers, videos sql.NullInt64
	var views30d sql.NullFloat64
	var extra sql.NullString
	var taken string
	if err := rows.Scan(&snapshot.AccountID, &taken, &followers, &views30d, &videos, &extra, &snapshot.Source); err != nil {
		return snapshot, err
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", taken); err == nil {
		snapshot.TakenAt = parsed
	}
	if followers.Valid {
		snapshot.Followers = &followers.Int64
	}
	if videos.Valid {
		snapshot.Videos = &videos.Int64
	}
	if views30d.Valid {
		snapshot.Views30d = &views30d.Float64
	}
	if extra.Valid && extra.String != "" {
		_ = json.Unmarshal([]byte(extra.String), &snapshot.Extra)
	}
	return snapshot, nil
}

const snapshotCols = `account_id, taken_at, followers, views_30d, videos, extra_json, source`

// LatestSnapshot returns the account's newest reading.
func (s *Store) LatestSnapshot(accountID int64) (*Snapshot, error) {
	rows, err := s.db.Query(`SELECT `+snapshotCols+` FROM metric_snapshots
		WHERE account_id = ? ORDER BY id DESC LIMIT 1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	snapshot, err := scanSnapshot(rows)
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// SnapshotSeries returns recent readings oldest-first for sparklines.
func (s *Store) SnapshotSeries(accountID int64, limit int) ([]Snapshot, error) {
	rows, err := s.db.Query(`SELECT `+snapshotCols+` FROM metric_snapshots
		WHERE account_id = ? ORDER BY id DESC LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []Snapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(snapshots)-1; i < j; i, j = i+1, j-1 {
		snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
	}
	return snapshots, nil
}

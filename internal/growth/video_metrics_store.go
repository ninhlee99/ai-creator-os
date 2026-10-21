package growth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InsertVideoMetricSnapshots appends provider readings without filling absent
// metrics with zero. AverageViewPercentage is stored as a fraction in the
// legacy completion column and in its original percent form in extra_json.
func (s *Store) InsertVideoMetricSnapshots(accountID int64, metrics []VideoMetric) error {
	if len(metrics) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO video_metric_snapshots
		(account_id, platform_video_id, taken_at, views, likes, comments, shares, saves, completion, extra_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, metric := range metrics {
		videoID := strings.TrimSpace(metric.PlatformVideoID)
		if videoID == "" {
			return fmt.Errorf("growth: video metric thiếu platform video ID")
		}
		taken := metric.TakenAt
		if taken.IsZero() {
			taken = time.Now()
		}
		var completion any
		extra := map[string]any{}
		if metric.AverageViewPercentage != nil {
			completion = *metric.AverageViewPercentage / 100
			extra["average_view_percentage"] = *metric.AverageViewPercentage
		}
		if metric.SubscribersGained != nil {
			extra["subscribers_gained"] = *metric.SubscribersGained
		}
		var extraJSON any
		if len(extra) > 0 {
			encoded, err := json.Marshal(extra)
			if err != nil {
				return err
			}
			extraJSON = string(encoded)
		}
		if _, err := stmt.Exec(accountID, videoID, taken.UTC().Format("2006-01-02 15:04:05"),
			metricInt(metric.Views), metricInt(metric.Likes), metricInt(metric.Comments),
			metricInt(metric.Shares), metricInt(metric.Saves), completion, extraJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecentVideoMetricSummary reports latest, non-null view readings refreshed
// inside the requested window. Missing views stay missing, never become zero.
type RecentVideoMetricSummary struct {
	VideoCount   int
	AverageViews *float64
	LatestAt     time.Time
}

func (s *Store) RecentVideoMetrics(ctx context.Context, accountID int64, since time.Time) (RecentVideoMetricSummary, error) {
	var summary RecentVideoMetricSummary
	if accountID <= 0 {
		return summary, fmt.Errorf("growth: account ID không hợp lệ")
	}
	cutoff := since.UTC().Format("2006-01-02 15:04:05")
	var average sql.NullFloat64
	var latest sql.NullString
	err := s.db.QueryRowContext(ctx, `
		WITH latest AS (
			SELECT MAX(id) AS id
			FROM video_metric_snapshots
			WHERE account_id = ?
			GROUP BY platform_video_id
		)
		SELECT COUNT(*), AVG(CAST(v.views AS REAL)), MAX(v.taken_at)
		FROM video_metric_snapshots v
		JOIN latest l ON l.id = v.id
		WHERE v.account_id = ? AND v.taken_at >= ? AND v.views IS NOT NULL`,
		accountID, accountID, cutoff).Scan(&summary.VideoCount, &average, &latest)
	if err != nil {
		return RecentVideoMetricSummary{}, err
	}
	if average.Valid {
		v := average.Float64
		summary.AverageViews = &v
	}
	if latest.Valid {
		summary.LatestAt, _ = time.Parse("2006-01-02 15:04:05", latest.String)
	}
	return summary, nil
}

func metricInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

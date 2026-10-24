package growth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type VideoMetricRow struct {
	PlatformVideoID string
	Title           string
	ShareURL        string
	PublishedAt     time.Time
	TakenAt         time.Time
	Views           *int64
	Likes           *int64
	Comments        *int64
	Shares          *int64
}

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
		if title := strings.TrimSpace(metric.Title); title != "" {
			extra["video_title"] = title
		}
		if shareURL := strings.TrimSpace(metric.ShareURL); shareURL != "" {
			extra["share_url"] = shareURL
		}
		if !metric.PublishedAt.IsZero() {
			extra["published_at"] = metric.PublishedAt.UTC().Format(time.RFC3339)
		}
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

// LatestVideoMetricRows returns one newest provider reading per public video.
func (s *Store) LatestVideoMetricRows(accountID int64, limit int) ([]VideoMetricRow, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("growth: account ID không hợp lệ")
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(`
		SELECT v.platform_video_id, v.taken_at, v.views, v.likes, v.comments, v.shares, COALESCE(v.extra_json,'')
		FROM video_metric_snapshots v
		JOIN (
			SELECT account_id, platform_video_id, MAX(id) AS id
			FROM video_metric_snapshots WHERE account_id=? GROUP BY account_id, platform_video_id
		) latest ON latest.id=v.id
		WHERE v.account_id=?
		ORDER BY v.taken_at DESC, v.id DESC LIMIT ?`, accountID, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VideoMetricRow
	for rows.Next() {
		var row VideoMetricRow
		var taken, extraJSON string
		var views, likes, comments, shares sql.NullInt64
		if err := rows.Scan(&row.PlatformVideoID, &taken, &views, &likes, &comments, &shares, &extraJSON); err != nil {
			return nil, err
		}
		row.TakenAt, _ = time.Parse("2006-01-02 15:04:05", taken)
		row.Views = nullableInt64(views)
		row.Likes = nullableInt64(likes)
		row.Comments = nullableInt64(comments)
		row.Shares = nullableInt64(shares)
		var extra struct {
			Title       string `json:"video_title"`
			ShareURL    string `json:"share_url"`
			PublishedAt string `json:"published_at"`
		}
		if extraJSON != "" {
			_ = json.Unmarshal([]byte(extraJSON), &extra)
		}
		row.Title, row.ShareURL = extra.Title, extra.ShareURL
		if extra.PublishedAt != "" {
			row.PublishedAt, _ = time.Parse(time.RFC3339, extra.PublishedAt)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// LatestVideoMetric returns the newest known TikTok counters for one public
// video. Missing snapshots are reported as found=false, never as zero.
func (s *Store) LatestVideoMetric(accountID int64, videoID string) (VideoMetricRow, bool, error) {
	if accountID <= 0 || strings.TrimSpace(videoID) == "" {
		return VideoMetricRow{}, false, fmt.Errorf("growth: account ID hoặc video ID không hợp lệ")
	}
	var row VideoMetricRow
	var taken, extraJSON string
	var views, likes, comments, shares sql.NullInt64
	err := s.db.QueryRow(`SELECT platform_video_id,taken_at,views,likes,comments,shares,COALESCE(extra_json,'')
		FROM video_metric_snapshots WHERE account_id=? AND platform_video_id=? ORDER BY id DESC LIMIT 1`,
		accountID, strings.TrimSpace(videoID)).Scan(&row.PlatformVideoID, &taken, &views, &likes, &comments, &shares, &extraJSON)
	if err == sql.ErrNoRows {
		return VideoMetricRow{}, false, nil
	}
	if err != nil {
		return VideoMetricRow{}, false, err
	}
	row.TakenAt, _ = time.Parse("2006-01-02 15:04:05", taken)
	row.Views, row.Likes = nullableInt64(views), nullableInt64(likes)
	row.Comments, row.Shares = nullableInt64(comments), nullableInt64(shares)
	var extra struct {
		Title       string `json:"video_title"`
		ShareURL    string `json:"share_url"`
		PublishedAt string `json:"published_at"`
	}
	if extraJSON != "" {
		_ = json.Unmarshal([]byte(extraJSON), &extra)
	}
	row.Title, row.ShareURL = extra.Title, extra.ShareURL
	if extra.PublishedAt != "" {
		row.PublishedAt, _ = time.Parse(time.RFC3339, extra.PublishedAt)
	}
	return row, true, nil
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

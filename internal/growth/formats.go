package growth

import (
	"database/sql"
	"encoding/json"
	"sort"
	"time"
)

type videoRow struct {
	VideoID           string
	FormatID          string
	Views             int64
	Shares            *int64
	Saves             *int64
	Completion        *float64
	SubscribersGained *int64
	TakenAt           time.Time
}

func (s *Store) latestVideoRows(accountID int64) ([]videoRow, error) {
	rows, err := s.db.Query(
		`SELECT v.platform_video_id, COALESCE(i.format_id,''), v.views, v.shares, v.saves, v.completion, v.taken_at, COALESCE(v.extra_json,'')
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
		var taken, extraJSON string
		if err := rows.Scan(&r.VideoID, &r.FormatID, &views, &shares, &saves, &completion, &taken, &extraJSON); err != nil {
			return nil, err
		}
		if !views.Valid {
			continue
		}
		r.Views = views.Int64
		if shares.Valid {
			v := shares.Int64
			r.Shares = &v
		}
		if saves.Valid {
			v := saves.Int64
			r.Saves = &v
		}
		if completion.Valid {
			v := completion.Float64
			r.Completion = &v
		}
		var extra struct {
			SubscribersGained *int64 `json:"subscribers_gained"`
		}
		if extraJSON != "" {
			_ = json.Unmarshal([]byte(extraJSON), &extra)
		}
		r.SubscribersGained = extra.SubscribersGained
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
		var completions, proxies, views, followsPer1k []float64
		var sum float64
		for _, v := range vids {
			views = append(views, float64(v.Views))
			sum += float64(v.Views)
			if v.Completion != nil {
				completions = append(completions, *v.Completion)
			}
			if v.Views > 0 && v.Shares != nil && v.Saves != nil {
				proxies = append(proxies, float64(*v.Shares+*v.Saves)/float64(v.Views))
			}
			if v.Views > 0 {
				if v.SubscribersGained != nil {
					followsPer1k = append(followsPer1k, float64(*v.SubscribersGained)/float64(v.Views)*1000)
				}
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
		if len(followsPer1k) > 0 {
			m := Median(followsPer1k)
			stat.FollowsPer1k = &m
		}
		if existing != nil {
			stat.Verdict = existing.Verdict
			stat.VerdictReason = existing.VerdictReason
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

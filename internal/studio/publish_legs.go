package studio

import "time"

// Chân đăng (publish legs) — idempotency cho auto-publish affiliate
// (Đợt Y).
//
// AutoPublishAffiliate records TikTok delivery per job. The
// studio_publish_legs key prevents duplicate drafts when hooks retry:
// trước khi đăng mỗi chân, check done → bỏ qua; sau đăng ghi done/failed;
// chân failed được tick thử lại.

// PublishLeg là một chân đăng của job affiliate.
type PublishLeg struct {
	JobID     string
	Title     string
	Platform  string // "tiktok"
	RemoteID  string
	Status    string
	UpdatedAt string
}

// PublishLeg statuses.
const (
	LegDone        = "done"
	LegFailed      = "failed"
	LegPublishing  = "publishing"
	LegNeedsReview = "needs_review"
)

// RecoverInterruptedPublishLegs runs at app startup. A process may have
// stopped after the platform accepted a request but before the result was
// persisted; never retry those legs automatically.
func (s *Studio) RecoverInterruptedPublishLegs() (int, error) {
	res, err := s.db.Exec(`UPDATE studio_publish_legs SET status=?, updated_at=? WHERE status=?`,
		LegNeedsReview, time.Now().Format("2006-01-02T15:04:05"), LegPublishing)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// PublishLegStatus trả về trạng thái chân đăng ("" khi chưa từng đăng).
func (s *Studio) PublishLegStatus(jobID, platform string) string {
	var st string
	_ = s.db.QueryRow(`SELECT status FROM studio_publish_legs WHERE job_id=? AND platform=?`,
		jobID, platform).Scan(&st)
	return st
}

// PublishLegRemoteID trả về remote_id của chân đăng ("" khi chưa có).
// Dùng khi growth phát hiện chân đã done để đánh dấu item đã đăng.
func (s *Studio) PublishLegRemoteID(jobID, platform string) string {
	var rid string
	_ = s.db.QueryRow(`SELECT remote_id FROM studio_publish_legs WHERE job_id=? AND platform=?`,
		jobID, platform).Scan(&rid)
	return rid
}

// SetPublishLeg ghi (upsert) trạng thái chân đăng.
func (s *Studio) SetPublishLeg(jobID, platform, status, remoteID string) error {
	_, err := s.db.Exec(`INSERT INTO studio_publish_legs(job_id,platform,remote_id,status,updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(job_id,platform) DO UPDATE SET
			remote_id=excluded.remote_id, status=excluded.status,
			updated_at=excluded.updated_at`,
		jobID, platform, remoteID, status,
		time.Now().Format("2006-01-02T15:04:05"))
	return err
}

// FailedPublishLegs liệt kê các chân đăng lỗi tạm thời để tick thử lại.
func (s *Studio) FailedPublishLegs() []PublishLeg {
	rows, err := s.db.Query(`SELECT job_id, platform FROM studio_publish_legs WHERE status=?`,
		LegFailed)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []PublishLeg
	for rows.Next() {
		var l PublishLeg
		if rows.Scan(&l.JobID, &l.Platform) == nil {
			out = append(out, l)
		}
	}
	return out
}

// RecentPublishLegs returns recorded automatic publish outcomes, newest first.
func (s *Studio) RecentPublishLegs(limit int) ([]PublishLeg, error) {
	rows, err := s.db.Query(`SELECT p.job_id, COALESCE(j.title,''), p.platform,
		p.remote_id, p.status, p.updated_at
		FROM studio_publish_legs p
		LEFT JOIN studio_jobs j ON j.id = p.job_id
		WHERE p.status IN (?, ?, ?, ?)
		ORDER BY p.updated_at DESC, p.job_id DESC LIMIT ?`, LegDone, LegFailed, LegNeedsReview, LegPublishing, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublishLeg
	for rows.Next() {
		var leg PublishLeg
		if err := rows.Scan(&leg.JobID, &leg.Title, &leg.Platform, &leg.RemoteID, &leg.Status, &leg.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, leg)
	}
	return out, rows.Err()
}

// RecentFailedPublishLegs returns current failed autopilot legs newest first.
func (s *Studio) RecentFailedPublishLegs(limit int) ([]PublishLeg, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT p.job_id, COALESCE(j.title,''), p.platform,
		p.remote_id, p.status, p.updated_at
		FROM studio_publish_legs p
		LEFT JOIN studio_jobs j ON j.id = p.job_id
		WHERE p.status=?
		ORDER BY p.updated_at DESC, p.job_id DESC LIMIT ?`, LegFailed, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublishLeg
	for rows.Next() {
		var leg PublishLeg
		if err := rows.Scan(&leg.JobID, &leg.Title, &leg.Platform, &leg.RemoteID, &leg.Status, &leg.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, leg)
	}
	return out, rows.Err()
}

// RecentPublishLegIssues returns failed or ambiguous automatic publish legs.
func (s *Studio) RecentPublishLegIssues(limit int) ([]PublishLeg, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT p.job_id, COALESCE(j.title,''), p.platform,
		p.remote_id, p.status, p.updated_at
		FROM studio_publish_legs p
		LEFT JOIN studio_jobs j ON j.id = p.job_id
		WHERE p.status IN (?, ?)
		ORDER BY p.updated_at DESC, p.job_id DESC LIMIT ?`, LegFailed, LegNeedsReview, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PublishLeg
	for rows.Next() {
		var leg PublishLeg
		if err := rows.Scan(&leg.JobID, &leg.Title, &leg.Platform, &leg.RemoteID, &leg.Status, &leg.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, leg)
	}
	return out, rows.Err()
}

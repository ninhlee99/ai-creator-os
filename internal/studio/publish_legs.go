package studio

import "time"

// Chân đăng (publish legs) — idempotency cho auto-publish affiliate
// (Đợt Y).
//
// Vấn đề: AutoPublishAffiliate không có marker nào ghi "job X đã đăng lên
// TikTok/YouTube". TikTok OK + YouTube lỗi → chân YouTube mất vĩnh viễn
// (job đã done, OnDone không bao giờ refire). Ngược lại hook fire 2 lần
// → đăng trùng. Bảng studio_publish_legs khóa theo (job_id, platform):
// trước khi đăng mỗi chân, check done → bỏ qua; sau đăng ghi done/failed;
// chân failed được tick thử lại.

// PublishLeg là một chân đăng của job affiliate.
type PublishLeg struct {
	JobID    string
	Platform string // "tiktok" | "youtube"
}

// PublishLeg statuses.
const (
	LegDone   = "done"
	LegFailed = "failed"
)

// PublishLegStatus trả về trạng thái chân đăng ("" khi chưa từng đăng).
func (s *Studio) PublishLegStatus(jobID, platform string) string {
	var st string
	_ = s.db.QueryRow(`SELECT status FROM studio_publish_legs WHERE job_id=? AND platform=?`,
		jobID, platform).Scan(&st)
	return st
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

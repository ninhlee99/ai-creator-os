package studio

import (
	"strings"
	"time"
)

type OnDoneFunc func(jobID string)

// SetOnDone registers the completion hook (nil clears it).
func (s *Studio) SetOnDone(fn OnDoneFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDone = fn
}

func (s *Studio) fireOnDone(id string) {
	s.mu.Lock()
	fn := s.onDone
	s.mu.Unlock()
	if fn != nil {
		fn(id)
	}
}

func (s *Studio) appendLog(id, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur string
	_ = s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&cur)
	if cur != "" {
		cur += "\n"
	}
	cur += time.Now().Format("15:04:05") + " " + line
	// Đợt V: cắt log giữ 200 dòng cuối — job lắm log không làm cột phình
	// vô hạn (write amplification mỗi lần append đọc-cộng-ghi cả cột).
	if lines := strings.Split(cur, "\n"); len(lines) > maxJobLogLines {
		cur = strings.Join(lines[len(lines)-maxJobLogLines:], "\n")
	}
	s.db.Exec(`UPDATE studio_jobs SET log=? WHERE id=?`, cur, id)
}

// maxJobLogLines: số dòng log tối đa giữ lại mỗi job (Đợt V).
const maxJobLogLines = 200

// PruneOldJobs xóa job đã xong (done/failed) quá maxAge (Đợt V — máy 24/7
// không để bảng phình mãi). Trả về số job đã xóa.
func (s *Studio) PruneOldJobs(maxAge time.Duration) int {
	// finished_at lưu dạng "2006-01-02T15:04:05" nên dùng strftime cùng định dạng.
	cutoff := time.Now().Add(-maxAge).Format("2006-01-02T15:04:05")
	res, err := s.db.Exec(`DELETE FROM studio_jobs
		WHERE status IN ('done','failed') AND finished_at != '' AND finished_at < ?`, cutoff)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

func (s *Studio) addAsset(jobID string, idx int, kind, prompt string) int64 {
	res, _ := s.db.Exec(`INSERT INTO studio_assets(job_id,idx,kind,prompt,status) VALUES(?,?,?,?,?)`,
		jobID, idx, kind, prompt, StatusQueued)
	id, _ := res.LastInsertId()
	return id
}

func (s *Studio) setAsset(id int64, status, path string) {
	s.db.Exec(`UPDATE studio_assets SET status=?, path=? WHERE id=?`, status, path, id)
}

// GetJob returns one job.
func (s *Studio) GetJob(id string) (Job, bool) {
	var j Job
	err := s.db.QueryRow(`SELECT id,kind,title,status,progress,log,output,caption,params,created_at,finished_at FROM studio_jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Log, &j.Output, &j.Caption, &j.Params, &j.CreatedAt, &j.FinishedAt)
	return j, err == nil
}

// ListJobs returns jobs newest first.
func (s *Studio) ListJobs(limit int) []Job {
	rows, err := s.db.Query(`SELECT id,kind,title,status,progress,CASE WHEN status = 'failed' THEN log ELSE '' END,output,created_at,finished_at FROM studio_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Log, &j.Output, &j.CreatedAt, &j.FinishedAt); err == nil {
			out = append(out, j)
		}
	}
	return out
}

// ListAssets returns a job's storyboard assets in order.
func (s *Studio) ListAssets(jobID string) []Asset {
	rows, err := s.db.Query(`SELECT id,job_id,idx,kind,path,prompt,status,method FROM studio_assets WHERE job_id=? ORDER BY idx`, jobID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.JobID, &a.Idx, &a.Kind, &a.Path, &a.Prompt, &a.Status, &a.Method); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// affiliate jobs
// ---------------------------------------------------------------------------

// CreateAffiliateJob queues an affiliate video job and starts it.

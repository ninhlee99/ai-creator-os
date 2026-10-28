package studio

import (
	"os"
	"path/filepath"
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
	// vô hạn (write amplification mỗi lần append đọc-cộng-ghi cả cột). Giữ
	// marker pipeline mới nhất để log dài vẫn chứng minh được các bước QC.
	cur = trimJobLog(cur)
	s.db.Exec(`UPDATE studio_jobs SET log=? WHERE id=?`, cur, id)
}

// maxJobLogLines: số dòng log gần nhất giữ lại ngoài pipeline evidence.
const maxJobLogLines = 200

var jobLogEvidenceMarkers = []string{
	"PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:", "PRODUCT_ANALYSIS_FAIL:",
	"DIRECTOR_START:", "DIRECTOR_PASS:", "DIRECTOR_FAIL:",
	"WORKFLOW_START:", "WORKFLOW_PASS:", "WORKFLOW_OUTPUT_PASS:", "WORKFLOW_FAIL:",
	"MOTION_QC_START:", "MOTION_QC_PASS:", "MOTION_QC_FAIL:",
	"VISUAL_QC_START:", "VISUAL_QC_PASS:", "VISUAL_QC_FAIL:", "LỖI:",
}

func trimJobLog(log string) string {
	lines := strings.Split(log, "\n")
	if len(lines) <= maxJobLogLines {
		return log
	}
	keepFrom := len(lines) - maxJobLogLines
	lastEvidence := make(map[string]int, len(jobLogEvidenceMarkers))
	for i, line := range lines {
		for _, marker := range jobLogEvidenceMarkers {
			if strings.Contains(line, marker) {
				lastEvidence[marker] = i
			}
		}
	}
	preserve := make(map[int]bool, len(lastEvidence))
	for _, i := range lastEvidence {
		if i < keepFrom {
			preserve[i] = true
		}
	}
	trimmed := make([]string, 0, maxJobLogLines+len(preserve))
	for i := 0; i < keepFrom; i++ {
		if preserve[i] {
			trimmed = append(trimmed, lines[i])
		}
	}
	trimmed = append(trimmed, lines[keepFrom:]...)
	return strings.Join(trimmed, "\n")
}

// PruneOldJobs xóa job đã xong (done/failed) quá maxAge (Đợt V — máy 24/7
// không để bảng phình mãi), đồng thời dọn studio_workflow_runs và cache radar_videos cũ.
// Trả về tổng số bản ghi job/workflow đã xóa.
func (s *Studio) PruneOldJobs(maxAge time.Duration) int {
	totalPruned := 0
	if fStore, err := s.GetFlowStore(); err == nil && fStore != nil {
		if prunedRunIDs, err := fStore.PruneOldRuns(maxAge); err == nil {
			runsDir := filepath.Join(s.workRoot, "runs")
			for _, runID := range prunedRunIDs {
				if runDir, ok := safeJobWorkDir(runsDir, runID); ok {
					_ = os.RemoveAll(runDir)
				}
			}
			totalPruned += len(prunedRunIDs)
		}
	}
	s.pruneRadarVideoCache(7 * 24 * time.Hour)

	// finished_at lưu dạng "2006-01-02T15:04:05" nên dùng strftime cùng định dạng.
	cutoff := time.Now().Add(-maxAge).Format("2006-01-02T15:04:05")
	rows, err := s.db.Query(`SELECT id,kind,title,status,progress,log,output,caption,params,created_at,finished_at
		FROM studio_jobs WHERE status IN ('done','failed') AND finished_at != '' AND finished_at < ?`, cutoff)
	if err != nil {
		return totalPruned
	}
	type prunedJob struct {
		job Job
	}
	var candidates []prunedJob
	for rows.Next() {
		var candidate prunedJob
		if err := rows.Scan(&candidate.job.ID, &candidate.job.Kind, &candidate.job.Title,
			&candidate.job.Status, &candidate.job.Progress, &candidate.job.Log,
			&candidate.job.Output, &candidate.job.Caption, &candidate.job.Params,
			&candidate.job.CreatedAt, &candidate.job.FinishedAt); err != nil {
			rows.Close()
			return totalPruned
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return totalPruned
	}
	rows.Close()
	activeFiltered := candidates[:0]
	for _, candidate := range candidates {
		if !s.jobActive(candidate.job.ID) {
			activeFiltered = append(activeFiltered, candidate)
		}
	}
	candidates = activeFiltered
	if len(candidates) == 0 {
		if totalPruned > 0 {
			_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
			_, _ = s.db.Exec(`PRAGMA optimize`)
		}
		return totalPruned
	}

	tx, err := s.db.Begin()
	if err != nil {
		return totalPruned
	}
	defer tx.Rollback()
	var pruned []prunedJob
	for _, candidate := range candidates {
		res, err := tx.Exec(`DELETE FROM studio_jobs WHERE id=? AND status IN ('done','failed')
			AND finished_at != '' AND finished_at < ?`, candidate.job.ID, cutoff)
		if err != nil {
			return totalPruned
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return totalPruned
		}
		if deleted == 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM studio_tiktok_publications WHERE job_id=?`, candidate.job.ID); err != nil {
			return totalPruned
		}
		if _, err := tx.Exec(`DELETE FROM studio_assets WHERE job_id=?`, candidate.job.ID); err != nil {
			return totalPruned
		}
		pruned = append(pruned, candidate)
	}
	if err := tx.Commit(); err != nil {
		return totalPruned
	}
	for _, candidate := range pruned {
		jobDir, safe := safeJobWorkDir(s.workRoot, candidate.job.ID)
		if safe {
			finalPath := s.OutputPath(candidate.job.Output)
			if !pathWithinDir(jobDir, finalPath) {
				finalPath = ""
			}
			removeJobWorkDir(jobDir, finalPath)
		}
		s.removeJobInputUploads(candidate.job)
	}
	totalPruned += len(pruned)
	if totalPruned > 0 {
		_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
		_, _ = s.db.Exec(`PRAGMA optimize`)
	}
	return totalPruned
}

func (s *Studio) pruneRadarVideoCache(maxAge time.Duration) {
	if s.workRoot == "" {
		return
	}
	cacheDir := filepath.Join(s.workRoot, "cache", "radar_videos")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(cacheDir, entry.Name()))
		}
	}
}

func sameJobFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	canonical := func(path string) (string, bool) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", false
		}
		clean := filepath.Clean(absolute)
		if resolved, err := filepath.EvalSymlinks(clean); err == nil {
			clean = filepath.Clean(resolved)
		}
		return clean, true
	}
	canonicalA, okA := canonical(a)
	canonicalB, okB := canonical(b)
	return okA && okB && canonicalA == canonicalB
}

func (s *Studio) jobActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, active := s.running[id]
	return active
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

// RecentFailedJobs returns failed jobs newest first, independently of the
// general recent-job window used by the dashboard's video cards.
func (s *Studio) RecentFailedJobs(limit int) ([]Job, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT id,kind,title,status,progress,log,output,caption,params,created_at,finished_at
		FROM studio_jobs WHERE status=?
		ORDER BY finished_at DESC, created_at DESC LIMIT ?`, StatusFailed, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.Title, &j.Status, &j.Progress, &j.Log, &j.Output,
			&j.Caption, &j.Params, &j.CreatedAt, &j.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// CountActiveJobs returns an exact count without limiting it to recent jobs.
func (s *Studio) CountActiveJobs() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM studio_jobs WHERE status IN (?, ?)`, StatusQueued, StatusRunning).Scan(&n)
	return n, err
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

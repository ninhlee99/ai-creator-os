package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Studio) insertJob(kind, title string, params any) (string, error) {
	return s.insertJobWithID(newID(), kind, title, params)
}

func (s *Studio) insertJobWithID(id, kind, title string, params any) (string, error) {
	if err := s.checkDiskCapacity(); err != nil {
		return "", err
	}
	pj, _ := json.Marshal(params)
	_, err := s.db.Exec(
		`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		id, kind, title, StatusQueued, string(pj), time.Now().Format("2006-01-02T15:04:05"))
	return id, err
}

func (s *Studio) checkDiskCapacity() error {
	// Đợt V: pre-flight đĩa — từ chối job mới khi đĩa gần đầy thay vì để
	// job render dở rồi kẹt (video + backup zip có thể lấp đĩa rất nhanh).
	if s.DiskGuard != nil {
		limit := s.MaxDiskPct
		if limit <= 0 {
			limit = 95
		}
		if pct, err := s.DiskGuard(); err == nil && pct >= limit {
			return fmt.Errorf("đĩa đã dùng %d%% (ngưỡng %d%%) — dọn đĩa rồi tạo job mới", pct, limit)
		}
	}
	return nil
}

// startTrackedJob registers a job before launching its goroutine. Shutdown
// cannot miss work that has been accepted but has not started running yet.
func (s *Studio) startTrackedJob(id string, ctx context.Context, cancel context.CancelFunc, run func(context.Context)) error {
	if ctx == nil || cancel == nil || run == nil {
		return fmt.Errorf("job context, cancel, and runner are required")
	}
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("studio is shutting down")
	}
	if s.running == nil {
		s.running = make(map[string]context.CancelFunc)
	}
	if _, exists := s.running[id]; exists {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("studio job %q is already active", id)
	}
	s.running[id] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
			cancel()
		}()
		run(ctx)
	}()
	return nil
}

// insertAffiliateJobWithPlan atomically consumes the one-use plan and creates
// its job. A disk/database failure leaves the plan available for retry.
func (s *Studio) insertAffiliateJobWithPlan(id, title string, params any) error {
	if err := s.checkDiskCapacity(); err != nil {
		return err
	}
	pj, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal affiliate job params: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM studio_affiliate_plans WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("shot plan đã được dùng hoặc không còn tồn tại; hãy lập plan mới")
	}
	if _, err := tx.Exec(`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		id, KindAffiliate, title, StatusQueued, string(pj), time.Now().Format("2006-01-02T15:04:05")); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Studio) setStatus(id, status string, progress int) {
	fin := ""
	if status == StatusDone || status == StatusFailed {
		fin = time.Now().Format("2006-01-02T15:04:05")
	}
	s.db.Exec(`UPDATE studio_jobs SET status=?, progress=?, finished_at=CASE WHEN ? != '' THEN ? ELSE finished_at END WHERE id=?`,
		status, progress, fin, fin, id)
}

func (s *Studio) setOutput(id, output string) {
	s.db.Exec(`UPDATE studio_jobs SET output=? WHERE id=?`, output, id)
}

func (s *Studio) setCaption(id, caption string) {
	s.db.Exec(`UPDATE studio_jobs SET caption=? WHERE id=?`, caption, id)
}

// AppendLog adds a line to a job's log from outside the studio package
// (e.g. Studio completion notifications in cmd/aicos).
func (s *Studio) AppendLog(id, line string) { s.appendLog(id, line) }

// PendingJobCount đếm job đang chờ hoặc đang chạy (đợt AA — trang growth
// hiện số này để user biết hàng đợi dài bao nhiêu).
func (s *Studio) PendingJobCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM studio_jobs WHERE status IN (?,?)`,
		StatusQueued, StatusRunning).Scan(&n)
	return n
}

// OnDoneFunc runs after a studio job finishes successfully. It is called
// from the job's goroutine; keep it quick or spawn its own goroutine.

// Shutdown drains jobs and detached flow runs before app exit, then cancels
// work still active after timeout.
func (s *Studio) Shutdown(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()
	for {
		s.mu.Lock()
		n := len(s.running) + len(s.flowRuns)
		s.mu.Unlock()
		if n == 0 {
			return
		}
		if !time.Now().Before(deadline) {
			// Cancel after the drain window, then allow workers to finish their
			// final state writes before callers close the SQLite connection.
			s.mu.Lock()
			type pendingCancel struct {
				cancel func()
				jobID  string
			}
			pending := make([]pendingCancel, 0, n)
			for id, cancel := range s.running {
				pending = append(pending, pendingCancel{cancel: cancel, jobID: id})
			}
			for _, cancel := range s.flowRuns {
				pending = append(pending, pendingCancel{cancel: cancel})
			}
			s.mu.Unlock()
			for _, item := range pending {
				item.cancel()
				if item.jobID != "" {
					s.appendLog(item.jobID, "App đang tắt — tác vụ bị hủy.")
				}
			}
			graceDeadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(graceDeadline) {
				s.mu.Lock()
				active := len(s.running) + len(s.flowRuns)
				s.mu.Unlock()
				if active == 0 {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

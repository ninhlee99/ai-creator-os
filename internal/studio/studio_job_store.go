package studio

import (
	"encoding/json"
	"fmt"
	"time"
)

func newID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Studio) insertJob(kind, title string, params any) (string, error) {
	// Đợt V: pre-flight đĩa — từ chối job mới khi đĩa gần đầy thay vì để
	// job render dở rồi kẹt (video + backup zip có thể lấp đĩa rất nhanh).
	if s.DiskGuard != nil {
		limit := s.MaxDiskPct
		if limit <= 0 {
			limit = 95
		}
		if pct, err := s.DiskGuard(); err == nil && pct >= limit {
			return "", fmt.Errorf("đĩa đã dùng %d%% (ngưỡng %d%%) — dọn đĩa rồi tạo job mới", pct, limit)
		}
	}
	pj, _ := json.Marshal(params)
	id := newID()
	_, err := s.db.Exec(
		`INSERT INTO studio_jobs(id,kind,title,status,params,created_at) VALUES(?,?,?,?,?,?)`,
		id, kind, title, StatusQueued, string(pj), time.Now().Format("2006-01-02T15:04:05"))
	return id, err
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
// (e.g. the auto-publish hook in cmd/aicos).
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

// Shutdown chờ các job đang chạy xong (tối đa timeout) trước khi app thoát.
// Đợt AM: graceful drain — SIGTERM không giết job đang render giữa chừng.
func (s *Studio) Shutdown(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		n := len(s.running)
		s.mu.Unlock()
		if n == 0 || time.Now().After(deadline) {
			if n > 0 {
				// Hết timeout: hủy các job còn lại để không treo shutdown.
				s.mu.Lock()
				cancels := make([]func(), 0, len(s.running))
				ids := make([]string, 0, len(s.running))
				for id, cancel := range s.running {
					cancels = append(cancels, cancel)
					ids = append(ids, id)
				}
				s.mu.Unlock()
				for i, cancel := range cancels {
					cancel()
					s.appendLog(ids[i], "App đang tắt — job bị hủy.")
				}
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

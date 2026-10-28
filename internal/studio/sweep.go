package studio

// Self-healing cho máy chạy 24/7 không người trông (Đợt L).
//
// MarkInterruptedJobs (có sẵn) xử lý restart: mọi job "running" đều chết
// cùng process cũ. Còn thiếu 2 lưới:
//  1. SweepHungJobs (định kỳ): job kẹt ở "running" quá lâu mà app KHÔNG
//     restart — ffmpeg treo, goroutine chết lặng. Không job nào hợp lệ
//     chạy 12h.
//  2. PruneFailedWork: dọn file tạm của job lỗi; output hoàn tất được giữ.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SweepHungJobs đánh dấu failed các job kẹt ở "running" quá maxAge.
// Trả về số job đã xử lý. Không đụng queued/done/failed.
//
// Đợt S: trước khi đánh failed, hủy context thật của job (CancelRunning) —
// ffmpeg/ffprobe treo bị kill, slot render được giải phóng. Trước đây sweep
// chỉ sửa DB trong khi process treo vẫn ngốn CPU/đĩa và giữ slot tới khi
// restart app.
func (s *Studio) SweepHungJobs(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge).Format("2006-01-02T15:04:05")
	rows, err := s.db.Query(
		`SELECT id FROM studio_jobs WHERE status=? AND created_at < ? AND kind<>?`,
		StatusRunning, cutoff, KindTikTokRadarRefresh)
	if err != nil {
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	swept := 0
	for _, id := range ids {
		killed := s.CancelRunning(id)
		updated, err := s.failHungJob(id, cutoff)
		if err != nil {
			continue
		}
		if updated {
			if killed {
				s.appendLog(id, fmt.Sprintf("Tự chữa: treo quá %s — đã kill process, đánh dấu thất bại", maxAge))
			} else {
				s.appendLog(id, fmt.Sprintf("Tự chữa: treo quá %s — đánh dấu thất bại", maxAge))
			}
			swept++
			continue
		}
		// The canceled worker may have recorded its own failure before this
		// conditional update. Count that terminal transition, but never replace
		// a job that completed successfully after the initial query.
		if job, ok := s.GetJob(id); ok && job.Status == StatusFailed {
			swept++
		}
	}
	return swept
}

func (s *Studio) failHungJob(id, cutoff string) (bool, error) {
	finishedAt := time.Now().Format("2006-01-02T15:04:05")
	result, err := s.db.Exec(`UPDATE studio_jobs SET status=?,progress=100,finished_at=?
		WHERE id=? AND status=? AND created_at < ? AND kind<>?`,
		StatusFailed, finishedAt, id, StatusRunning, cutoff, KindTikTokRadarRefresh)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// PruneFailedWork xóa thư mục work + file asset của các job đã failed.
// Giữ nguyên DB row (lịch sử + log còn đó). Trả về (số job, bytes giải phóng).
func (s *Studio) PruneFailedWork() (int, int64) {
	rows, err := s.db.Query(`SELECT id,output FROM studio_jobs WHERE status=?`, StatusFailed)
	if err != nil {
		return 0, 0
	}
	type failedJob struct{ id, output string }
	var jobs []failedJob
	for rows.Next() {
		var job failedJob
		if rows.Scan(&job.id, &job.output) == nil {
			jobs = append(jobs, job)
		}
	}
	rows.Close()
	var freed int64
	pruned := 0
	for _, job := range jobs {
		if s.jobActive(job.id) {
			continue
		}
		jobDir, safe := safeJobWorkDir(s.workRoot, job.id)
		if !safe {
			continue
		}
		finalPath := s.OutputPath(job.output)
		if !pathWithinDir(jobDir, finalPath) {
			finalPath = ""
		}
		freed += removeJobWorkDir(jobDir, finalPath)
		s.appendLog(job.id, "Tự chữa đĩa: đã dọn file tạm của job lỗi")
		pruned++
	}
	return pruned, freed
}

func safeJobWorkDir(workRoot, jobID string) (string, bool) {
	if jobID == "" || jobID == "." || jobID == ".." || filepath.Base(jobID) != jobID {
		return "", false
	}
	root, err := filepath.Abs(workRoot)
	if err != nil {
		return "", false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	dir := filepath.Join(root, jobID)
	relative, err := filepath.Rel(root, dir)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() {
			return "", false
		}
	} else if !os.IsNotExist(err) {
		return "", false
	}
	return dir, true
}

func pathWithinDir(root, path string) bool {
	if path == "" {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	pathResolved, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootResolved, pathResolved)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func removeJobWorkDir(jobDir, keepPath string) int64 {
	if keepPath == "" || !pathWithinDir(jobDir, keepPath) {
		return removeAll(jobDir)
	}
	type entry struct {
		path string
		info os.FileInfo
	}
	var entries []entry
	if err := filepath.Walk(jobDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries = append(entries, entry{path: path, info: info})
		return nil
	}); err != nil {
		return 0
	}
	var freed int64
	for i := len(entries) - 1; i >= 0; i-- {
		item := entries[i]
		if item.info.IsDir() {
			if item.path != jobDir {
				_ = os.Remove(item.path)
			}
			continue
		}
		if !sameJobFile(item.path, keepPath) {
			freed += removeFile(item.path)
		}
	}
	return freed
}

// removeFile xóa 1 file, trả về bytes đã giải phóng (0 nếu không xóa được).
func removeFile(path string) int64 {
	if path == "" {
		return 0
	}
	var size int64
	if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
	}
	if err := os.Remove(path); err != nil {
		return 0
	}
	return size
}

// removeAll xóa cả thư mục, trả về tổng bytes (ước tính qua filepath.Walk).
func removeAll(dir string) int64 {
	var size int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			size += fi.Size()
		}
		return nil
	})
	if err := os.RemoveAll(dir); err != nil {
		return 0
	}
	return size
}

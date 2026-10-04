package studio

// Self-healing cho máy chạy 24/7 không người trông (Đợt L).
//
// MarkInterruptedJobs (có sẵn) xử lý restart: mọi job "running" đều chết
// cùng process cũ. Còn thiếu 2 lưới:
//  1. SweepHungJobs (định kỳ): job kẹt ở "running" quá lâu mà app KHÔNG
//     restart — ffmpeg treo, goroutine chết lặng. Không job nào hợp lệ
//     chạy 12h.
//  2. PruneFailedWork / PruneOldPublishedOutputs: dọn đĩa tự động, chỉ
//     chạm những gì an toàn (job lỗi, video đã đăng xong).

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SweepHungJobs đánh dấu failed các job kẹt ở "running" quá maxAge.
// Trả về số job đã xử lý. Không đụng queued/done/failed.
func (s *Studio) SweepHungJobs(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge).Format("2006-01-02T15:04:05")
	rows, err := s.db.Query(
		`SELECT id FROM studio_jobs WHERE status=? AND created_at < ?`,
		StatusRunning, cutoff)
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
	for _, id := range ids {
		s.appendLog(id, fmt.Sprintf("Tự chữa: treo quá %s — đánh dấu thất bại", maxAge))
		s.setStatus(id, StatusFailed, 100)
	}
	return len(ids)
}

// PruneFailedWork xóa thư mục work + file asset của các job đã failed.
// Giữ nguyên DB row (lịch sử + log còn đó). Trả về (số job, bytes giải phóng).
func (s *Studio) PruneFailedWork() (int, int64) {
	rows, err := s.db.Query(`SELECT id FROM studio_jobs WHERE status=?`, StatusFailed)
	if err != nil {
		return 0, 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var freed int64
	for _, id := range ids {
		for _, a := range s.ListAssets(id) {
			freed += removeFile(a.Path)
		}
		freed += removeAll(filepath.Join(s.workRoot, id))
		s.appendLog(id, "Tự chữa đĩa: đã dọn file tạm của job lỗi")
	}
	return len(ids), freed
}

// PruneOldPublishedOutputs xóa file output local của truyện đã đăng
// YouTube (private) từ quá maxAge — bản đăng vẫn còn trên YouTube,
// log ghi rõ. Chỉ chạm KindStory có marker published (callback).
// Trả về (số video, bytes giải phóng).
func (s *Studio) PruneOldPublishedOutputs(maxAge time.Duration, isPublished func(id string) bool) (int, int64) {
	if isPublished == nil {
		return 0, 0
	}
	cutoff := time.Now().Add(-maxAge).Format("2006-01-02T15:04:05")
	rows, err := s.db.Query(
		`SELECT id, output FROM studio_jobs WHERE kind=? AND status=? AND output != '' AND finished_at != '' AND finished_at < ?`,
		KindStory, StatusDone, cutoff)
	if err != nil {
		return 0, 0
	}
	type target struct{ id, output string }
	var targets []target
	for rows.Next() {
		var t target
		if rows.Scan(&t.id, &t.output) == nil && isPublished(t.id) {
			targets = append(targets, t)
		}
	}
	rows.Close()
	var freed int64
	for _, t := range targets {
		freed += removeFile(t.output)
		s.db.Exec(`UPDATE studio_jobs SET output='' WHERE id=?`, t.id)
		s.appendLog(t.id, "Tự chữa đĩa: đã dọn file video local (bản đăng vẫn trên YouTube)")
	}
	return len(targets), freed
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

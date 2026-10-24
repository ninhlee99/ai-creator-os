package studio

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Studio) CancelRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel, ok := s.running[id]
	if !ok {
		return false
	}
	cancel()
	return true
}

func copyStudioFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// DeleteJob cancels a running job (if any), waits for its goroutine to exit,
// then removes its assets, output, work dir and DB rows. Works for any job
// kind.
func (s *Studio) DeleteJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s", id)
	}
	s.CancelRunning(id)
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		_, stillRunning := s.running[id]
		s.mu.Unlock()
		if !stillRunning || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, a := range s.ListAssets(id) {
		os.Remove(a.Path)
	}
	if j.Output != "" {
		os.Remove(s.OutputPath(j.Output))
	}
	os.RemoveAll(filepath.Join(s.workRoot, id))
	if _, err := s.db.Exec(`DELETE FROM studio_tiktok_publications WHERE job_id=?`, id); err != nil {
		return fmt.Errorf("xoá liên kết TikTok: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM studio_assets WHERE job_id=?`, id); err != nil {
		return fmt.Errorf("xoá assets: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM studio_jobs WHERE id=?`, id); err != nil {
		return fmt.Errorf("xoá job: %w", err)
	}
	return nil
}

// MarkInterruptedJobs fails every job left "running" from a previous process
// (their render goroutines died with it). Call once at startup; returns the
// number of jobs swept.
//
// Đợt Y: also fails jobs left "queued" — a queued job's goroutine died with
// the old process too, and nothing ever re-dispatches it, so it would sit
// at "đang chờ" forever while the UI pretends work is happening.
func (s *Studio) MarkInterruptedJobs() int {
	rows, err := s.db.Query(`SELECT id, status FROM studio_jobs WHERE status IN (?,?)`,
		StatusRunning, StatusQueued)
	if err != nil {
		return 0
	}
	type swept struct{ id, status string }
	var ids []swept
	for rows.Next() {
		var sw swept
		if rows.Scan(&sw.id, &sw.status) == nil {
			ids = append(ids, sw)
		}
	}
	rows.Close()
	for _, sw := range ids {
		if sw.status == StatusQueued {
			s.appendLog(sw.id, "Kẹt sau restart (đang chờ mà tiến trình cũ đã chết) — đánh dấu thất bại, tạo lại job nếu cần")
		} else {
			s.appendLog(sw.id, "Gián đoạn khi khởi động lại")
		}
		s.setStatus(sw.id, StatusFailed, 100)
	}
	return len(ids)
}

// DownloadVideo fetches a direct video URL (mp4/webm/mov) to a local file — used
// when the user pastes a Muse AI video link or CDN URL.
func DownloadVideo(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("video HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 250<<20))
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}

// stampUserVersion records the schema generation for future migrations
// (R2-W7): a fresh database is marked with v, an existing stamp is never
// overwritten here.
func stampUserVersion(db *sql.DB, v int) error {
	var cur int
	if err := db.QueryRow("PRAGMA user_version").Scan(&cur); err != nil {
		return err
	}
	if cur == 0 {
		_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
		return err
	}
	return nil
}

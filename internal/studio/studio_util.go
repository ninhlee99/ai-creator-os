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

// MarkInterruptedJobs fails every job left "running" from a previous process
// (their render goroutines died with it). Call once at startup; returns the
// number of jobs swept. The user can resume film jobs with RerunFilmJob.
func (s *Studio) MarkInterruptedJobs() int {
	rows, err := s.db.Query(`SELECT id FROM studio_jobs WHERE status=?`, StatusRunning)
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
		s.appendLog(id, "Gián đoạn khi khởi động lại")
		s.setStatus(id, StatusFailed, 100)
	}
	return len(ids)
}

// DownloadAudio fetches a direct audio URL (mp3/m4a) to a local file — used
// when the user pastes a TikTok CDN / direct link for the trending sound.
func DownloadAudio(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("audio HTTP %d", resp.StatusCode)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 100<<20))
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

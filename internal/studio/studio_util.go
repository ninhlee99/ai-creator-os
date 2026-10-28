package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// DeleteJob cancels a running job (if any), waits for its goroutine to exit,
// then removes its assets, output, work dir and DB rows. Works for any job
// kind.
func (s *Studio) DeleteJob(id string) error {
	return s.deleteJob(id, 5*time.Second)
}

func (s *Studio) deleteJob(id string, stopTimeout time.Duration) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s", id)
	}
	jobDir, safe := safeJobWorkDir(s.workRoot, id)
	if !safe {
		return fmt.Errorf("job %s có đường dẫn làm việc không an toàn", id)
	}
	if j.Kind == KindTikTokRadarRefresh && (j.Status == StatusQueued || j.Status == StatusRunning) {
		return fmt.Errorf("không thể xóa job Radar đang chạy; theo dõi trong tab Radar TikTok")
	}
	s.CancelRunning(id)
	deadline := time.Now().Add(stopTimeout)
	for {
		s.mu.Lock()
		_, stillRunning := s.running[id]
		s.mu.Unlock()
		if !stillRunning {
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("job %s chưa dừng; chưa xóa dữ liệu", id)
		}
		time.Sleep(100 * time.Millisecond)
	}
	j, ok = s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s sau khi dừng", id)
	}
	outputPath := ""
	if j.Output != "" && s.outDir != "" {
		candidate := s.OutputPath(j.Output)
		if pathWithinDir(s.outDir, candidate) {
			outputPath = candidate
		}
	}
	for _, a := range s.ListAssets(id) {
		if pathWithinDir(jobDir, a.Path) || outputPath != "" && sameJobFile(a.Path, outputPath) {
			_ = os.Remove(a.Path)
		}
	}
	if outputPath != "" {
		_ = os.Remove(outputPath)
	}
	_ = os.RemoveAll(jobDir)
	if _, err := s.db.Exec(`DELETE FROM studio_tiktok_publications WHERE job_id=?`, id); err != nil {
		return fmt.Errorf("xoá liên kết TikTok: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM studio_assets WHERE job_id=?`, id); err != nil {
		return fmt.Errorf("xoá assets: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM studio_jobs WHERE id=?`, id); err != nil {
		return fmt.Errorf("xoá job: %w", err)
	}
	s.removeJobInputUploads(j)
	return nil
}

func (s *Studio) removeJobInputUploads(job Job) {
	paths, err := studioJobInputPaths(job)
	if err != nil || len(paths) == 0 {
		return
	}
	referenced, ok := s.otherJobAndPlanInputPaths(job.ID)
	if !ok {
		return
	}

	roots := []string{s.UploadDir(), filepath.Join(filepath.Dir(s.outDir), "uploads")}
	resolvedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err == nil {
			resolvedRoots = append(resolvedRoots, resolved)
		}
	}
	if len(resolvedRoots) == 0 {
		return
	}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		info, err := os.Lstat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			continue
		}
		owned := false
		for _, root := range resolvedRoots {
			relative, err := filepath.Rel(root, resolved)
			if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if referenced[filepath.Clean(absolute)] || referenced[filepath.Clean(resolved)] {
			continue
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		_ = os.Remove(absolute)
	}
}

func studioJobInputPaths(job Job) ([]string, error) {
	switch job.Kind {
	case KindAffiliate:
		var p AffiliateParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, err
		}
		return affiliateInputPaths(p), nil
	case KindViral:
		var p ViralParams
		if err := json.Unmarshal([]byte(job.Params), &p); err != nil {
			return nil, err
		}
		return []string{p.SampleVideo}, nil
	default:
		return nil, nil
	}
}

func affiliateInputPaths(p AffiliateParams) []string {
	paths := append([]string{p.ProductPhoto}, p.ExtraPhotos...)
	return append(paths, p.Clips...)
}

func (s *Studio) otherJobAndPlanInputPaths(exceptJobID string) (map[string]bool, bool) {
	referenced := map[string]bool{}
	add := func(paths []string) {
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if absolute, err := filepath.Abs(path); err == nil && path != "" {
				clean := filepath.Clean(absolute)
				referenced[clean] = true
				if resolved, err := filepath.EvalSymlinks(clean); err == nil {
					referenced[filepath.Clean(resolved)] = true
				}
			}
		}
	}

	rows, err := s.db.Query(`SELECT kind,params FROM studio_jobs WHERE id<>?`, exceptJobID)
	if err != nil {
		return nil, false
	}
	for rows.Next() {
		var candidate Job
		if err := rows.Scan(&candidate.Kind, &candidate.Params); err != nil {
			rows.Close()
			return nil, false
		}
		paths, err := studioJobInputPaths(candidate)
		if err != nil {
			rows.Close()
			return nil, false
		}
		add(paths)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false
	}
	rows.Close()

	planRows, err := s.db.Query(`SELECT payload FROM studio_affiliate_plans`)
	if err != nil {
		return nil, false
	}
	for planRows.Next() {
		var payload string
		var prep AffiliatePreparation
		if err := planRows.Scan(&payload); err != nil {
			planRows.Close()
			return nil, false
		}
		if err := json.Unmarshal([]byte(payload), &prep); err != nil {
			planRows.Close()
			return nil, false
		}
		add(affiliateInputPaths(prep.Params))
	}
	if err := planRows.Err(); err != nil {
		planRows.Close()
		return nil, false
	}
	planRows.Close()

	var graphTable int
	err = s.db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='studio_workflow_graphs'`).Scan(&graphTable)
	if err != nil && err != sql.ErrNoRows {
		return nil, false
	}
	if err == nil {
		graphRows, err := s.db.Query(`SELECT graph_json FROM studio_workflow_graphs`)
		if err != nil {
			return nil, false
		}
		for graphRows.Next() {
			var graphJSON string
			var graph flow.FlowGraph
			if err := graphRows.Scan(&graphJSON); err != nil {
				graphRows.Close()
				return nil, false
			}
			if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
				graphRows.Close()
				return nil, false
			}
			for _, node := range graph.Nodes {
				add(flowStringValues(node.Params))
				add(flowStringValues(node.State.Outputs))
				add([]string{node.State.OutputPreview})
			}
		}
		if err := graphRows.Err(); err != nil {
			graphRows.Close()
			return nil, false
		}
		graphRows.Close()
	}
	return referenced, true
}

func flowStringValues(value any) []string {
	var values []string
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case string:
			values = append(values, value)
		case []any:
			for _, item := range value {
				visit(item)
			}
		case map[string]any:
			for _, item := range value {
				visit(item)
			}
		}
	}
	visit(value)
	return values
}

// MarkInterruptedJobs fails interrupted Studio jobs at startup. TikTok Radar
// refresh jobs are excluded because their persisted queue resumes after WebAI
// initialization.
//
// Đợt Y: also fails jobs left "queued" — a queued job's goroutine died with
// the old process too, and nothing ever re-dispatches it, so it would sit
// at "đang chờ" forever while the UI pretends work is happening.
func (s *Studio) MarkInterruptedJobs() int {
	rows, err := s.db.Query(`SELECT id, status, kind FROM studio_jobs WHERE status IN (?,?)`,
		StatusRunning, StatusQueued)
	if err != nil {
		return 0
	}
	type swept struct{ id, status, kind string }
	var ids []swept
	for rows.Next() {
		var sw swept
		if rows.Scan(&sw.id, &sw.status, &sw.kind) == nil {
			if sw.kind == KindTikTokRadarRefresh {
				continue
			}
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

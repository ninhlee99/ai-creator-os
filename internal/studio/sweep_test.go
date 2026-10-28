package studio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Đợt L: sweeper job treo + dọn đĩa.

// insertRawJob chèn job trực tiếp để kiểm soát created_at/finished_at.
func insertRawJob(t *testing.T, st *Studio, id, kind, status, createdAt, finishedAt, output string) {
	t.Helper()
	_, err := st.db.Exec(
		`INSERT INTO studio_jobs(id,kind,title,status,progress,output,created_at,finished_at) VALUES(?,?,?,?,?,?,?,?)`,
		id, kind, "t", status, 0, output, createdAt, finishedAt)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
}
func TestSweepHungJobs(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-13 * time.Hour).Format("2006-01-02T15:04:05")
	fresh := time.Now().Add(-time.Hour).Format("2006-01-02T15:04:05")
	insertRawJob(t, st, "hung1", KindViral, StatusRunning, old, "", "")
	insertRawJob(t, st, "fresh1", KindViral, StatusRunning, fresh, "", "")
	insertRawJob(t, st, "done1", KindViral, StatusDone, old, old, "")

	if n := st.SweepHungJobs(12 * time.Hour); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	if j, _ := st.GetJob("hung1"); j.Status != StatusFailed {
		t.Fatalf("hung1 status = %s, want failed", j.Status)
	}
	if j, _ := st.GetJob("fresh1"); j.Status != StatusRunning {
		t.Fatalf("fresh1 status = %s, want running (untouched)", j.Status)
	}
	if j, _ := st.GetJob("hung1"); j.Log == "" {
		t.Fatal("hung1 log should explain the sweep")
	}
}

func TestPruneFailedWork(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02T15:04:05")
	insertRawJob(t, st, "fail1", KindViral, StatusFailed, now, now, "")
	// work dir + asset file của job lỗi
	workFile := filepath.Join(outDir, "studio", "fail1", "tmp.bin")
	if err := os.MkdirAll(filepath.Dir(workFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workFile, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(
		`INSERT INTO studio_assets(job_id,idx,kind,path,status) VALUES(?,0,'image',?,?)`,
		"fail1", workFile, StatusDone); err != nil {
		t.Fatal(err)
	}
	// job done không bị đụng
	insertRawJob(t, st, "done1", KindViral, StatusDone, now, now, "")
	doneWork := filepath.Join(outDir, "studio", "done1", "tmp.bin")
	if err := os.MkdirAll(filepath.Dir(doneWork), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doneWork, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	n, freed := st.PruneFailedWork()
	if n != 1 {
		t.Fatalf("pruned %d jobs, want 1", n)
	}
	if freed < 10 {
		t.Fatalf("freed %d bytes, want >= 10", freed)
	}
	if _, err := os.Stat(workFile); !os.IsNotExist(err) {
		t.Fatal("work file của job lỗi phải bị xóa")
	}
	if _, err := os.Stat(doneWork); err != nil {
		t.Fatal("work file của job done không được đụng")
	}
	if j, _ := st.GetJob("fail1"); j.Status != StatusFailed {
		t.Fatal("DB row của job lỗi phải giữ lại")
	}
}

func TestPruneOldJobsRemovesJobArtifactsAndKeepsFinalOutput(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	old := time.Now().Add(-90 * 24 * time.Hour).Format("2006-01-02T15:04:05")
	finalOutput := filepath.Join(outDir, "final.mp4")
	workFile := filepath.Join(st.WorkDir(), "old-affiliate", "scene.png")
	productInput := filepath.Join(st.UploadDir(), "old-product.jpg")
	for _, path := range []string{finalOutput, workFile, productInput} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep-or-clean"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	insertRawJob(t, st, "old-affiliate", KindAffiliate, StatusDone, old, old, filepath.Base(finalOutput))
	params, err := json.Marshal(AffiliateParams{ProductPhoto: productInput})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE studio_jobs SET params=? WHERE id=?`, string(params), "old-affiliate"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO studio_assets(job_id,idx,kind,path,status) VALUES(?,0,'image',?,?)`,
		"old-affiliate", workFile, StatusDone); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`INSERT INTO studio_assets(job_id,idx,kind,path,status) VALUES(?,1,'video',?,?)`,
		"old-affiliate", finalOutput, StatusDone); err != nil {
		t.Fatal(err)
	}

	if got := st.PruneOldJobs(60 * 24 * time.Hour); got != 1 {
		t.Fatalf("pruned %d jobs, want 1", got)
	}
	if _, ok := st.GetJob("old-affiliate"); ok {
		t.Fatal("old job row remains")
	}
	var assetCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM studio_assets WHERE job_id=?`, "old-affiliate").Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 {
		t.Fatalf("old job has %d orphaned asset rows", assetCount)
	}
	if _, err := os.Stat(workFile); !os.IsNotExist(err) {
		t.Fatalf("old work file remains: %v", err)
	}
	if _, err := os.Stat(productInput); !os.IsNotExist(err) {
		t.Fatalf("old input upload remains: %v", err)
	}
	if _, err := os.Stat(finalOutput); err != nil {
		t.Fatalf("final output was removed: %v", err)
	}
}

func TestAppendLogPreservesLatestPipelineEvidenceWhenTrimming(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	insertRawJob(t, st, "long-job", KindAffiliate, StatusRunning, "now", "", "")

	markers := []string{
		"PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:",
		"DIRECTOR_START:", "DIRECTOR_PASS:",
		"WORKFLOW_START:", "WORKFLOW_PASS:",
		"VISUAL_QC_START:", "VISUAL_QC_PASS:",
	}
	for _, marker := range markers {
		st.appendLog("long-job", marker+" stage complete")
	}
	for i := 0; i < maxJobLogLines+50; i++ {
		st.appendLog("long-job", fmt.Sprintf("render progress %d", i))
	}

	job, ok := st.GetJob("long-job")
	if !ok {
		t.Fatal("job disappeared")
	}
	for _, marker := range markers {
		if !strings.Contains(job.Log, marker) {
			t.Fatalf("trimmed log lost pipeline evidence %q", marker)
		}
	}
	if strings.Contains(job.Log, "render progress 0") || !strings.Contains(job.Log, fmt.Sprintf("render progress %d", maxJobLogLines+49)) {
		t.Fatal("log tail was not retained or old progress line was not trimmed")
	}
	if lines := strings.Count(job.Log, "\n") + 1; lines > maxJobLogLines+len(markers) {
		t.Fatalf("trimmed log has %d lines, want at most %d", lines, maxJobLogLines+len(markers))
	}
}

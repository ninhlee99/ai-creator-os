package studio

import (
	"os"
	"path/filepath"
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
	insertRawJob(t, st, "hung1", KindStory, StatusRunning, old, "", "")
	insertRawJob(t, st, "fresh1", KindStory, StatusRunning, fresh, "", "")
	insertRawJob(t, st, "done1", KindStory, StatusDone, old, old, "")

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
	insertRawJob(t, st, "fail1", KindStory, StatusFailed, now, now, "")
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
	insertRawJob(t, st, "done1", KindStory, StatusDone, now, now, "")
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

func TestPruneOldPublishedOutputs(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldVideo := filepath.Join(outDir, "old.mp4")
	newVideo := filepath.Join(outDir, "new.mp4")
	for _, p := range []string{oldVideo, newVideo} {
		if err := os.WriteFile(p, []byte("video-data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-40 * 24 * time.Hour).Format("2006-01-02T15:04:05")
	recent := time.Now().Add(-time.Hour).Format("2006-01-02T15:04:05")
	insertRawJob(t, st, "pub-old", KindStory, StatusDone, old, old, oldVideo)
	insertRawJob(t, st, "pub-new", KindStory, StatusDone, recent, recent, newVideo)
	insertRawJob(t, st, "unpub-old", KindStory, StatusDone, old, old, oldVideo)
	isPublished := func(id string) bool { return id == "pub-old" || id == "pub-new" }

	n, freed := st.PruneOldPublishedOutputs(30*24*time.Hour, isPublished)
	if n != 1 {
		t.Fatalf("pruned %d, want 1 (chỉ video cũ + đã đăng)", n)
	}
	if freed <= 0 {
		t.Fatal("phải giải phóng bytes")
	}
	if _, err := os.Stat(oldVideo); !os.IsNotExist(err) {
		t.Fatal("video cũ đã đăng phải bị xóa file local")
	}
	if _, err := os.Stat(newVideo); err != nil {
		t.Fatal("video mới đăng chưa tới hạn — không được xóa")
	}
	if j, _ := st.GetJob("pub-old"); j.Output != "" {
		t.Fatal("output DB phải được xóa để UI không trỏ file chết")
	}
}

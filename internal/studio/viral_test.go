package studio

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestViralScriptGeneration(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	p := ViralParams{
		Topic:      "3 Mẹo Tiết Kiệm Thời Gian Khi Dùng Laptop",
		Genre:      "lifehack",
		VisualMood: "cinematic",
	}

	script, err := s.GenerateViralScript(context.Background(), p)
	if err != nil {
		t.Fatalf("GenerateViralScript: %v", err)
	}

	if script.Title == "" {
		t.Errorf("expected non-empty Title")
	}
	if script.Hook3s == "" {
		t.Errorf("expected non-empty Hook3s")
	}
	if len(script.Scenes) < 3 {
		t.Errorf("expected at least 3 scenes, got %d", len(script.Scenes))
	}
	for i, sc := range script.Scenes {
		if sc.VoiceText == "" {
			t.Errorf("scene %d missing VoiceText", i+1)
		}
		if sc.ImagePrompt == "" {
			t.Errorf("scene %d missing ImagePrompt", i+1)
		}
	}
}

func TestEnqueueViralJob(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	p := ViralParams{
		Topic: "Khám Phá Vũ Trụ Bí Ẩn",
		Genre: "mystery",
	}

	jobID, err := s.EnqueueViral(context.Background(), p)
	if err != nil {
		t.Fatalf("EnqueueViral: %v", err)
	}
	if jobID == "" {
		t.Fatalf("expected non-empty jobID")
	}

	jobs := s.ListViralJobs(10)
	if len(jobs) == 0 {
		t.Fatalf("expected at least 1 viral job")
	}
	if jobs[0].Kind != KindViral {
		t.Errorf("expected kind %q, got %q", KindViral, jobs[0].Kind)
	}
	s.CancelRunning(jobID)
	time.Sleep(100 * time.Millisecond)
	s.DeleteJob(jobID)
}

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

func TestAnalyzeViralOriginal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Test case 1: Douyin hot dance clip
	clipDouyin := "data/clips/viral/clip_cn_douyin_hot.mp4"
	if fileExists(clipDouyin) {
		prof := AnalyzeViralOriginal(ctx, clipDouyin, dir)
		if prof.MotionVibe != "energetic_beat" {
			t.Errorf("expected energetic_beat, got %s", prof.MotionVibe)
		}
		if prof.VisualMood != "neon_aesthetic" {
			t.Errorf("expected neon_aesthetic, got %s", prof.VisualMood)
		}
		if prof.Duration <= 0 {
			t.Errorf("expected positive duration, got %f", prof.Duration)
		}
	}

	// Test case 2: Bedroom dance clip
	clipBedroom := "data/clips/viral/clip_03_sexy_dance_60s.mp4"
	if fileExists(clipBedroom) {
		prof := AnalyzeViralOriginal(ctx, clipBedroom, dir)
		if prof.MotionVibe != "smooth_sensual" {
			t.Errorf("expected smooth_sensual, got %s", prof.MotionVibe)
		}
		if prof.VisualMood != "warm_studio" {
			t.Errorf("expected warm_studio, got %s", prof.VisualMood)
		}
	}
}

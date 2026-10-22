package studio

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestViralScriptGenerationRequiresObservedSource(t *testing.T) {
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

	_, err = s.GenerateViralScript(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "thời lượng video nguồn") {
		t.Fatalf("GenerateViralScript error = %v, want missing source duration", err)
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
	if err == nil || !strings.Contains(err.Error(), "credits") {
		t.Fatalf("EnqueueViral error = %v, want explicit credits consent", err)
	}
	if jobID != "" || len(s.ListViralJobs(10)) != 0 {
		t.Fatalf("paid video edit must not enqueue a job: id=%q jobs=%d", jobID, len(s.ListViralJobs(10)))
	}
}

func TestGeminiOmniVideoEditorIsDisabledBeforeFileAccess(t *testing.T) {
	mg := &GeminiMediaGen{keys: []string{"not-used"}}
	err := mg.EditCharacterVideoSegment(context.Background(), "/missing/source.mp4", nil, "/tmp/output.mp4")
	if err == nil || !strings.Contains(err.Error(), "free-only") {
		t.Fatalf("editor error = %v, want free-only policy error", err)
	}
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

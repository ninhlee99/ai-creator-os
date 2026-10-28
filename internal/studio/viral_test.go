package studio

import (
	"context"
	"encoding/json"
	"math"
	"os"
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

func TestEnqueueValidatedViralReturnsIDWhenPersistedJobCannotStart(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.mu.Lock()
	st.shuttingDown = true
	st.mu.Unlock()

	source := filepath.Join(st.UploadDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := st.enqueueValidatedViral(ViralParams{Topic: "retry source", SampleVideo: source}, true)
	if err == nil || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("enqueueValidatedViral error = %v, want shutdown error", err)
	}
	if id == "" {
		t.Fatal("persisted failed job returned empty ID")
	}
	job, ok := st.GetJob(id)
	if !ok || job.Status != StatusFailed {
		t.Fatalf("persisted job = (%+v, %v), want failed row", job, ok)
	}
	var saved ViralParams
	if err := json.Unmarshal([]byte(job.Params), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SampleVideo != source {
		t.Fatalf("persisted source path = %q, want %q", saved.SampleVideo, source)
	}
}

func TestValidateViralSourceDurationMatchesRenderLimits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration float64
		wantErr  bool
	}{
		{name: "minimum", duration: minViralSourceDurationSeconds},
		{name: "maximum", duration: maxViralSourceDurationSeconds},
		{name: "too short", duration: minViralSourceDurationSeconds - 0.01, wantErr: true},
		{name: "too long", duration: maxViralSourceDurationSeconds + 0.01, wantErr: true},
		{name: "not finite", duration: math.Inf(1), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateViralSourceDuration(tc.duration)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateViralSourceDuration(%v) error = %v, wantErr %v", tc.duration, err, tc.wantErr)
			}
		})
	}
}

func TestParseViralReferenceAnalysisFailsClosed(t *testing.T) {
	tests := []struct {
		name, raw string
	}{
		{name: "invalid json", raw: `not json`},
		{name: "missing observations", raw: `{"summary":"source frames","beats":[]}`},
		{name: "out of range timestamp", raw: `{"summary":"source frames","beats":[{"time_sec":9,"framing":"full body","subject_action":"arms raised"}]}`},
		{name: "no visible evidence", raw: `{"summary":"source frames","beats":[{"time_sec":1,"camera_movement":"unclear"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseViralReferenceAnalysis(tc.raw, 8); err == nil {
				t.Fatal("parseViralReferenceAnalysis accepted invalid or missing visual evidence")
			}
		})
	}
}

func TestParseViralReferenceAnalysisKeepsObservedBeat(t *testing.T) {
	raw := `{"summary":"Performer is centered","beats":[{"time_sec":1.25,"framing":"full body","subject_action":"right arm raised","body_movement":"unclear between samples"}]}`
	summary, beats, err := parseViralReferenceAnalysis(raw, 8)
	if err != nil {
		t.Fatalf("parseViralReferenceAnalysis: %v", err)
	}
	if summary != "Performer is centered" || len(beats) != 1 || beats[0].TimeSec != 1.25 || beats[0].SubjectAction != "right arm raised" {
		t.Fatalf("parsed source evidence changed: summary=%q beats=%+v", summary, beats)
	}
}

func TestValidateViralReferenceScenesRejectsTimelineMismatch(t *testing.T) {
	valid := []ViralScene{{
		SourceStartSec: 0, SourceEndSec: 6, DurationSec: 6,
		ImagePrompt: "Observed centered performer", VideoPrompt: "Recreate raised right arm at 1.2s",
	}}
	if err := validateViralReferenceScenes(valid, 6); err != nil {
		t.Fatalf("valid storyboard rejected: %v", err)
	}
	if valid[0].Index != 1 || valid[0].Phase != "source_beat" || valid[0].DurationSec != 6 {
		t.Fatalf("valid storyboard not normalized: %+v", valid[0])
	}

	invalid := []ViralScene{{
		SourceStartSec: 0, SourceEndSec: 6, DurationSec: 5,
		ImagePrompt: "Observed centered performer", VideoPrompt: "Recreate raised right arm at 1.2s",
	}}
	if err := validateViralReferenceScenes(invalid, 6); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("timeline mismatch error = %v, want duration mismatch", err)
	}
	if err := validateViralReferenceScenes(valid, math.NaN()); err == nil {
		t.Fatal("accepted non-finite source duration")
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

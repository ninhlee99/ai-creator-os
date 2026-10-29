package studio

import (
	"bytes"
	"context"
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

	// Không còn yêu cầu xác nhận credits thủ công, nhưng vẫn kiểm tra các điều kiện khác (ví dụ character editor)
	_, err = s.EnqueueViral(context.Background(), p)
	if err == nil || strings.Contains(err.Error(), "credits") {
		t.Fatalf("EnqueueViral error = %v, must not block on credits consent", err)
	}
}

func TestEnqueueValidatedViralRejectsShutdownBeforePersistence(t *testing.T) {
	dir := t.TempDir()
	st, err := New(filepath.Join(dir, "studio.db"), nil, nil, nil, filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.Shutdown(0)

	source := filepath.Join(st.UploadDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := st.enqueueValidatedViral(ViralParams{Topic: "retry source", SampleVideo: source}, true)
	if err == nil || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("enqueueValidatedViral error = %v, want shutdown error", err)
	}
	if id != "" {
		t.Fatalf("rejected pre-shutdown job returned ID %q", id)
	}
	if jobs := st.ListViralJobs(10); len(jobs) != 0 {
		t.Fatalf("rejected pre-shutdown job persisted unexpectedly: %+v", jobs)
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

func TestValidateTikTokSourceFileEnforcesSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(path, []byte("four"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateTikTokSourceFileSize(path, 4); err != nil {
		t.Fatalf("source exactly at byte limit rejected: %v", err)
	}
	if err := validateTikTokSourceFileSize(path, 3); err == nil {
		t.Fatal("source over byte limit unexpectedly accepted")
	}
}

func TestCopyViralFileCopiesContentAndTruncatesDestination(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	destination := filepath.Join(dir, "destination.mp4")
	content := bytes.Repeat([]byte("media-frame"), 1<<16)
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, bytes.Repeat([]byte("stale"), 1<<17), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyViralFile(source, destination); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, content) {
		t.Fatalf("copy mismatch: got %d bytes, want %d", len(copied), len(content))
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

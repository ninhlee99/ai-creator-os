package studio

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEvaluateVideoQuality(t *testing.T) {
	st := &Studio{}
	videoPath := filepath.Join("..", "web", "static", "sample_916.mp4")
	if !fileExists(videoPath) {
		t.Skip("sample_916.mp4 not found for video quality evaluation test")
	}

	workDir := t.TempDir()
	eval, err := st.EvaluateVideoQuality(context.Background(), videoPath, workDir, func(ctx context.Context, prompt string, frames []string) (string, error) {
		return "Độ nét sắc sảo, ánh sáng ấm áp, chuyển động mượt mà và tự nhiên.", nil
	})
	if err != nil {
		t.Fatalf("EvaluateVideoQuality failed: %v", err)
	}

	if eval == nil {
		t.Fatal("EvaluateVideoQuality returned nil evaluation")
	}

	if eval.Width != 1080 || eval.Height != 1920 {
		t.Errorf("width x height = %dx%d, want 1080x1920", eval.Width, eval.Height)
	}

	if eval.DurationSec <= 0 {
		t.Errorf("duration = %f, want > 0", eval.DurationSec)
	}

	if eval.OverallScore < 50 || eval.OverallScore > 100 {
		t.Errorf("overall score = %d, want between 50 and 100", eval.OverallScore)
	}

	if !eval.HasAudio {
		t.Errorf("has audio = false, want true for sample_916.mp4")
	}

	if eval.Status != "pass" && eval.Status != "good" {
		t.Errorf("status = %s, want pass or good", eval.Status)
	}

	reportFile := filepath.Join(workDir, "video_quality_report.json")
	if !fileExists(reportFile) {
		t.Errorf("video_quality_report.json was not created in workDir")
	}
}

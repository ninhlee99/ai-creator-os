package studio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVeoModelsPriority(t *testing.T) {
	models := GetVeoModels()
	if len(models) == 0 {
		t.Fatal("expected at least 1 candidate Veo model")
	}
	if models[0] != "veo-3.1-generate-preview" {
		t.Errorf("expected default top model to be veo-3.1-generate-preview, got %s", models[0])
	}

	// Test user override
	os.Setenv("GEMINI_VEO_MODEL", "custom-veo-model")
	defer os.Unsetenv("GEMINI_VEO_MODEL")

	overrideModels := GetVeoModels()
	if len(overrideModels) == 0 || overrideModels[0] != "custom-veo-model" {
		t.Errorf("expected custom override to take top priority, got %v", overrideModels)
	}
}

func TestMuseAIClientHealth(t *testing.T) {
	clientNoKey := NewMuseAIClient("", "")
	if clientNoKey.Healthy() {
		t.Error("expected client without key to be unhealthy")
	}

	clientWithKey := NewMuseAIClient("test-key-123", "")
	if !clientWithKey.Healthy() {
		t.Error("expected client with key to be healthy")
	}
}

func TestAvailableVideoEngines(t *testing.T) {
	st := &Studio{}
	engines := st.AvailableVideoEngines()
	if len(engines) < 3 {
		t.Fatalf("expected at least 3 video engines, got %v", engines)
	}

	hasVeo := false
	hasMuse := false
	for _, e := range engines {
		if e == "veo3" {
			hasVeo = true
		}
		if e == "museai" {
			hasMuse = true
		}
	}
	if !hasVeo || !hasMuse {
		t.Errorf("expected veo3 and museai in available engines, got %v", engines)
	}
}

func TestGenerateMotionVideoClipFallback(t *testing.T) {
	dir := t.TempDir()
	outMP4 := filepath.Join(dir, "test_clip.mp4")
	keyframe := makeTestPNG(t, dir, "frame.png", "testsrc")

	st, err := New(filepath.Join(dir, "test_studio.db"), nil, nil, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	var logs []string
	logFn := func(msg string) {
		logs = append(logs, msg)
	}

	err = st.GenerateMotionVideoClip(ctx, MotionVideoOptions{
		Prompt:      "cinematic fashion model walking down street",
		FirstFrame:  keyframe,
		Seconds:     3,
		AspectRatio: "9:16",
		Engine:      "auto",
		OutputPath:  outMP4,
		LogFn:       logFn,
	})

	if err != nil {
		t.Fatalf("GenerateMotionVideoClip error: %v", err)
	}

	if !fileExists(outMP4) {
		t.Fatalf("expected output video file to exist at %s", outMP4)
	}

	w, h := ProbeDims(ctx, outMP4)
	if w != 1080 || h != 1920 {
		t.Errorf("expected dims 1080x1920, got %dx%d", w, h)
	}
}

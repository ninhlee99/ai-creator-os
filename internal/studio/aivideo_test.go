package studio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type countingVideoGen struct{ calls int }

func (*countingVideoGen) GenerateImage(context.Context, string, []ImageRef, string) error {
	return errors.New("test provider: image generation disabled")
}

func (g *countingVideoGen) GenerateVideo(context.Context, string, string, int, string, string) error {
	g.calls++
	return errors.New("test provider: video generation disabled")
}

func (*countingVideoGen) Name() string                 { return "test" }
func (*countingVideoGen) Healthy(context.Context) bool { return true }

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
		t.Error("expected client without a verified API to be unhealthy")
	}

	clientWithKey := NewMuseAIClient("test-key-123", "")
	if clientWithKey.Healthy() {
		t.Error("credentials must not imply an undocumented video API is healthy")
	}

	clientWithCookie := NewMuseAIClient("", "")
	clientWithCookie.SetCookieHeader("ps_l=session123")
	if clientWithCookie.Healthy() {
		t.Error("session cookie must not enable an undocumented video API")
	}
	if err := clientWithKey.RenderMuseAIVideo(context.Background(), "prompt", "", 5, "9:16", filepath.Join(t.TempDir(), "out.mp4"), nil); !errors.Is(err, ErrMuseVideoAPIUnavailable) {
		t.Fatalf("RenderMuseAIVideo error = %v, want ErrMuseVideoAPIUnavailable", err)
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
	videoGen := &countingVideoGen{}

	st, err := New(filepath.Join(dir, "test_studio.db"), nil, videoGen, nil, dir)
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
	if videoGen.calls != 1 {
		t.Fatalf("auto mode called video provider %d times, want 1", videoGen.calls)
	}

	if !fileExists(outMP4) {
		t.Fatalf("expected output video file to exist at %s", outMP4)
	}

	w, h := ProbeDims(ctx, outMP4)
	if w != 1080 || h != 1920 {
		t.Errorf("expected dims 1080x1920, got %dx%d", w, h)
	}
}

func TestGoogleFlowRequiresExportedClipAndDoesNotCallVideoAPI(t *testing.T) {
	dir := t.TempDir()
	videoGen := &countingVideoGen{}
	st, err := New(filepath.Join(dir, "test_studio.db"), nil, videoGen, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	err = st.GenerateMotionVideoClip(context.Background(), MotionVideoOptions{
		Prompt:     "A creator presents a product",
		Engine:     "googleflow",
		OutputPath: filepath.Join(dir, "clip.mp4"),
	})
	if err == nil {
		t.Fatal("expected a clear error when Google Flow clip has not been exported")
	}
	if videoGen.calls != 0 {
		t.Fatalf("Google Flow selection called direct video API %d times, want 0", videoGen.calls)
	}
}

func TestStudioWebAIResolver(t *testing.T) {
	st := &Studio{}
	_, err := st.GetWebAISession("googleflow")
	if err == nil {
		t.Fatal("expected error when no resolver set")
	}

	st.SetWebAIResolver(func(platform string) (*WebAISession, error) {
		if platform == "googleflow" {
			return &WebAISession{
				Platform:     "googleflow",
				SessionToken: "test-token",
				CookieHeader: "__Secure-1PSID=test-sid",
				Email:        "creator@gmail.com",
			}, nil
		}
		return nil, os.ErrNotExist
	})

	sess, err := st.GetWebAISession("googleflow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.Email != "creator@gmail.com" || sess.SessionToken != "test-token" {
		t.Fatalf("unexpected session: %+v", sess)
	}
}

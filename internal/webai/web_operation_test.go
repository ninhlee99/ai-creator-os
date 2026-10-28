package webai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAIOperationCapabilitiesMatchProviderFeatures(t *testing.T) {
	want := map[string]WebAICapabilities{
		PlatformChatGPT: {ImageInput: true, VideoInput: true, ImageOutput: true},
		PlatformClaude:  {ImageInput: true},
		PlatformGemini:  {ImageInput: true, VideoInput: true, ImageOutput: true, VideoOutput: true},
		PlatformGrok:    {ImageInput: true, VideoInput: true, ImageOutput: true, VideoOutput: true},
	}
	for platform, expected := range want {
		got, ok := WebAIOperationCapabilities(platform)
		if !ok || got != expected {
			t.Errorf("capabilities for %s = %+v, exists=%v; want %+v", platform, got, ok, expected)
		}
	}
	if _, ok := WebAIOperationCapabilities("unknown"); ok {
		t.Fatal("unknown providers must not inherit capabilities")
	}
}

func TestValidateWebAIInputFilesUsesProviderAndTypeLimits(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "reference.png")
	video := filepath.Join(dir, "reference.mp4")
	if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	chatgpt, _ := WebAIOperationCapabilities(PlatformChatGPT)
	if got, err := validateWebAIInputFiles(PlatformChatGPT, []string{image, video}, chatgpt); err != nil || len(got) != 2 {
		t.Fatalf("ChatGPT should accept image and video paths, got %v, err=%v", got, err)
	}
	claude, _ := WebAIOperationCapabilities(PlatformClaude)
	if _, err := validateWebAIInputFiles(PlatformClaude, []string{video}, claude); err == nil {
		t.Fatal("Claude web adapter must reject unsupported video attachments")
	}
}

func TestWebAIFileInputMustAcceptEverySelectedKind(t *testing.T) {
	paths := []string{"model.jpg", "clip.mp4"}
	if !webAIInputAcceptsPaths(flowFileInput{Accept: "image/*,video/*", Multiple: true}, paths) {
		t.Fatal("combined image/video input should accept mixed uploads")
	}
	if webAIInputAcceptsPaths(flowFileInput{Accept: "image/*", Multiple: true}, paths) {
		t.Fatal("image-only input must reject mixed image/video uploads")
	}
}

func TestDetectedWebAIMediaExtensionRejectsHTMLResponses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider-download")
	if err := os.WriteFile(path, []byte("<!doctype html><title>login</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ext := detectedWebAIMediaExtension(path, "video"); ext != "" {
		t.Fatalf("HTML response must not be accepted as a video: %q", ext)
	}
	if err := os.WriteFile(path, []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ext := detectedWebAIMediaExtension(path, "video"); ext != ".mp4" {
		t.Fatalf("MP4 signature returned extension %q", ext)
	}
}

func TestDetectedWebAIMediaExtensionRejectsHTMLWithMediaFilename(t *testing.T) {
	for _, filename := range []string{"login.png", "login.mp4"} {
		t.Run(filename, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), filename)
			if err := os.WriteFile(path, []byte("<!doctype html><title>login</title>"), 0o600); err != nil {
				t.Fatal(err)
			}
			outputType := "image"
			if filepath.Ext(filename) == ".mp4" {
				outputType = "video"
			}
			if ext := detectedWebAIMediaExtension(path, outputType); ext != "" {
				t.Fatalf("HTML body named %s must be rejected, got %q", filename, ext)
			}
		})
	}
}

func TestWebAIOperationPromptStatesRequestedMediaType(t *testing.T) {
	brief := "A model presents a red silk dress."
	if got := webAIOperationPrompt("image", brief); got == brief || !strings.Contains(got, "Create one image") {
		t.Fatalf("image prompt omitted output mode: %q", got)
	}
	if got := webAIOperationPrompt("video", brief); got == brief || !strings.Contains(got, "Create one video") {
		t.Fatalf("video prompt omitted output mode: %q", got)
	}
	if got := webAIOperationPrompt("text", brief); got != brief {
		t.Fatalf("text prompt should remain unchanged: %q", got)
	}
}

func TestWebAIOperationReusesOnlySameProviderConversationURL(t *testing.T) {
	page := imageAnalysisPages[PlatformChatGPT]
	conversation := "https://chatgpt.com/c/abc123"
	if got := webAIOperationStartURL(page, "text", conversation); got != conversation {
		t.Fatalf("same-provider conversation URL = %q, want %q", got, conversation)
	}
	if got := webAIOperationStartURL(page, "text", "https://evil.example/c/abc123"); got != page.url {
		t.Fatalf("cross-provider URL must fall back to provider home, got %q", got)
	}
	if got := webAIConversationURL(page, "http://chatgpt.com/c/abc123"); got != "" {
		t.Fatalf("insecure conversation URL must be rejected, got %q", got)
	}
}

func TestNormalizeFlowAspectAcceptsDocumentedAndLegacyValues(t *testing.T) {
	for input, want := range map[string]string{
		"9:16": "9:16", "VIDEO_ASPECT_RATIO_PORTRAIT": "9:16",
		"16:9": "16:9", "VIDEO_ASPECT_RATIO_LANDSCAPE": "16:9",
		"1:1": "1:1", "VIDEO_ASPECT_RATIO_SQUARE": "1:1",
	} {
		got, err := normalizeFlowAspect(input)
		if err != nil || got != want {
			t.Errorf("normalizeFlowAspect(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizeFlowAspect("4:3"); err == nil {
		t.Fatal("unsupported aspect ratios must fail instead of silently using portrait")
	}
}

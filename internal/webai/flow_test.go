package webai

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGoogleFlowPlatformAndBundle(t *testing.T) {
	// 1. Verify PlatformGoogleFlow is in SupportedPlatforms
	platforms := SupportedPlatforms()
	var foundFlow bool
	var foundMuse bool
	for _, p := range platforms {
		if p.ID == PlatformMuseAI {
			foundMuse = true
			if p.Badge != "Video · handoff" {
				t.Errorf("expected manual video handoff badge for Muse AI, got %s", p.Badge)
			}
			if p.URL != "https://muse.ai" {
				t.Errorf("expected Muse URL https://muse.ai, got %s", p.URL)
			}
		}
		if p.ID == PlatformGoogleFlow {
			foundFlow = true
			if p.URL != "https://labs.google/fx/tools/flow" {
				t.Errorf("expected Flow URL labs.google/fx/tools/flow, got %s", p.URL)
			}
			if p.Badge != "Video · handoff" {
				t.Errorf("expected manual video handoff badge, got %s", p.Badge)
			}
		}
	}
	if !foundFlow {
		t.Fatalf("PlatformGoogleFlow not found in SupportedPlatforms")
	}
	if !foundMuse {
		t.Fatalf("PlatformMuseAI not found in SupportedPlatforms")
	}

	// 2. Test Store Account saving for Google Flow
	dbPath := filepath.Join(t.TempDir(), "webai_flow_test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = store.SaveAccount(Account{
		Platform:    PlatformGoogleFlow,
		AccountName: "Google Flow Main",
		Email:       "ninh.flow@gmail.com",
		SessionKey:  "flow-session-key-xyz",
		Status:      StatusConnected,
		Tier:        "free",
	})
	if err != nil {
		t.Fatalf("SaveAccount for Google Flow failed: %v", err)
	}

	acc, err := store.PickActiveAccount(PlatformGoogleFlow)
	if err != nil {
		t.Fatalf("PickActiveAccount failed: %v", err)
	}
	if acc.Platform != PlatformGoogleFlow {
		t.Errorf("expected platform googleflow, got %s", acc.Platform)
	}

	// 3. Test BuildFlowPromptBundle
	engine := NewEngine(store)
	bundle := engine.BuildFlowPromptBundle("Video Viral Mỹ Phẩm", "Skincare", "fashion", []string{
		"Girl applying cream",
		"Smooth glowing skin reveal",
	})

	if bundle.TotalScenes != 2 {
		t.Errorf("expected 2 scenes, got %d", bundle.TotalScenes)
	}
	if bundle.Aspect != "9:16 vertical" {
		t.Errorf("expected 9:16 vertical, got %s", bundle.Aspect)
	}
	if len(bundle.Scenes) != 2 || bundle.Scenes[0].KeyframeFile != "scene_01.png" {
		t.Errorf("invalid scene keyframe: %+v", bundle.Scenes)
	}

	// 4. Flow generation must fail explicitly until a supported API exists.
	if _, err := engine.GenerateFlowVideoClip(context.Background(), "test prompt", "scene_01.png", 5); err == nil {
		t.Fatal("expected manual generation handoff error")
	}
}

func TestVideoProviderPoolsRotateAccountsIndependently(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "webai_video_pool_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, platform := range []string{PlatformGoogleFlow, PlatformMuseAI} {
		for _, email := range []string{"first@example.com", "second@example.com"} {
			if err := store.SaveAccount(Account{
				Platform:    platform,
				AccountName: email,
				Email:       email,
				SessionKey:  "session-" + email,
				Status:      StatusConnected,
			}); err != nil {
				t.Fatalf("SaveAccount(%s, %s): %v", platform, email, err)
			}
		}
	}

	for _, platform := range []string{PlatformGoogleFlow, PlatformMuseAI} {
		first, err := store.PickActiveAccount(platform)
		if err != nil {
			t.Fatalf("PickActiveAccount(%s) first: %v", platform, err)
		}
		second, err := store.PickActiveAccount(platform)
		if err != nil {
			t.Fatalf("PickActiveAccount(%s) second: %v", platform, err)
		}
		if first.ID == second.ID {
			t.Fatalf("%s pool selected same account twice despite another unused account", platform)
		}
	}
}

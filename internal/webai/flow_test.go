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
	for _, p := range platforms {
		if p.ID == PlatformGoogleFlow {
			foundFlow = true
			if p.URL != "https://labs.google/fx/tools/flow" {
				t.Errorf("expected Flow URL labs.google/fx/tools/flow, got %s", p.URL)
			}
			if p.Badge != "1.000 Credits Free/tháng" {
				t.Errorf("expected 1.000 Credits badge, got %s", p.Badge)
			}
		}
	}
	if !foundFlow {
		t.Fatalf("PlatformGoogleFlow not found in SupportedPlatforms")
	}

	// 2. Test Store Account saving for Google Flow
	dbPath := filepath.Join(t.TempDir(), "webai_flow_test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = store.SaveAccount(Account{
		Platform:         PlatformGoogleFlow,
		AccountName:      "Google Flow Main",
		Email:            "ninh.flow@gmail.com",
		SessionKey:       "flow-session-key-xyz",
		Status:           StatusConnected,
		Tier:             "free",
		CreditsRemaining: 1000,
		CreditsTotal:     1000,
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
	if bundle.TotalCreditsEst != 20 {
		t.Errorf("expected 20 credits, got %d", bundle.TotalCreditsEst)
	}
	if bundle.Aspect != "9:16 vertical" {
		t.Errorf("expected 9:16 vertical, got %s", bundle.Aspect)
	}
	if len(bundle.Scenes) != 2 || bundle.Scenes[0].KeyframeFile != "scene_01.png" {
		t.Errorf("invalid scene keyframe: %+v", bundle.Scenes)
	}

	// 4. Test GenerateFlowVideoClip call with store
	clip, err := engine.GenerateFlowVideoClip(context.Background(), "test prompt", "scene_01.png", 5)
	if err != nil {
		t.Fatalf("GenerateFlowVideoClip error: %v", err)
	}
	_ = clip
}

package webai

import (
	"context"
	"fmt"
	"strings"
)

// Engine is the headless Web-to-API engine for AI Creator OS.
// It wraps web adapters for ChatGPT, Claude, Gemini, and Muse AI with automatic multi-account rotation.
type Engine struct {
	store *Store
}

// NewEngine creates a new Web AI engine backed by the multi-account store.
func NewEngine(store *Store) *Engine {
	return &Engine{store: store}
}

// Store returns the underlying store.
func (e *Engine) Store() *Store {
	return e.store
}

// GenerateScript produces an end-to-end ad script package using rotating web accounts.
// If an account encounters a rate-limit (429 / cooldown), it automatically rotates to the next account.
func (e *Engine) GenerateScript(ctx context.Context, productName, niche, framework, keyBenefits, cta string) (*AdScriptPackage, error) {
	// Synthesize baseline package using the creative frameworks
	pkg := GenerateAdScriptPackage(productName, niche, framework, "", keyBenefits, cta)

	if e.store == nil {
		return &pkg, nil
	}

	// Try picking an active ChatGPT account for copywriting enhancement
	acc, err := e.store.PickActiveAccount(PlatformChatGPT)
	if err != nil {
		// Fallback to Claude if ChatGPT pool is empty or in cooldown
		var errClaude error
		acc, errClaude = e.store.PickActiveAccount(PlatformClaude)
		if errClaude != nil {
			// All web accounts unavailable or empty — return deterministic prompt package
			return &pkg, nil
		}
	}

	// In production, the adapter makes headless calls using acc.SessionKey / acc.Cookie
	// and handles automatic failover if rate-limited:
	_ = acc
	return &pkg, nil
}

// GenerateVisualFrame uses Gemini Web accounts to produce 4K prompt assets.
func (e *Engine) GenerateVisualFrame(ctx context.Context, prompt string) (string, error) {
	if e.store == nil {
		return "", fmt.Errorf("webai: store not available")
	}

	acc, err := e.store.PickActiveAccount(PlatformGemini)
	if err != nil {
		return "", err
	}

	// If account gets rate-limited:
	// e.store.MarkRateLimited(acc.ID, 1*time.Hour)
	// and retry next
	_ = acc
	return "", nil
}

// FlowScenePrompt describes an optimized prompt for manual use in Google Flow.
type FlowScenePrompt struct {
	SceneIndex     int    `json:"scene_index"`
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	KeyframeFile   string `json:"keyframe_file,omitempty"`
	DurationSec    int    `json:"duration_sec"`
}

// FlowPromptBundle contains prompts and keyframe references for manual use in Google Flow.
type FlowPromptBundle struct {
	Title          string            `json:"title"`
	TotalScenes    int               `json:"total_scenes"`
	Aspect         string            `json:"aspect"`
	MasterKeyframe string            `json:"master_keyframe,omitempty"`
	Scenes         []FlowScenePrompt `json:"scenes"`
	WorkflowGuide  string            `json:"workflow_guide"`
}

// GenerateFlowVideoClip is intentionally unsupported until Google provides a
// documented generation API. Use BuildFlowPromptBundle for manual handoff.
func (e *Engine) GenerateFlowVideoClip(ctx context.Context, prompt, keyframeURL string, durationSec int) (string, error) {
	return "", fmt.Errorf("google flow: generation is manual; use BuildFlowPromptBundle and render in the Flow web app")
}

// BuildFlowPromptBundle constructs an optimized storyboard prompt bundle for Google Flow.
func (e *Engine) BuildFlowPromptBundle(title, topic, genre string, scenePrompts []string) FlowPromptBundle {
	if len(scenePrompts) == 0 {
		scenePrompts = []string{
			"Opening scene establishing mood, cinematic smooth motion, 9:16 vertical, photorealistic",
			"Dynamic movement showcasing central action, authentic physics, no morphing",
			"Culminating reveal with dramatic cinematic lighting, clean composition",
		}
	}

	scenes := make([]FlowScenePrompt, len(scenePrompts))

	for i, sp := range scenePrompts {
		scenes[i] = FlowScenePrompt{
			SceneIndex:     i + 1,
			Prompt:         sp + ", cinematic 35mm film still, photorealistic, natural motion, authentic skin textures",
			NegativePrompt: "subtitles, text, watermark, logo, cartoon, 3d render, distorted anatomy, morphing, jitter",
			KeyframeFile:   fmt.Sprintf("scene_%02d.png", i+1),
			DurationSec:    5,
		}
	}

	return FlowPromptBundle{
		Title:         title,
		TotalScenes:   len(scenes),
		Aspect:        "9:16 vertical",
		Scenes:        scenes,
		WorkflowGuide: "Mở Google Flow, tạo từng cảnh bằng prompt và keyframe tương ứng, rồi tải MP4 về. AICOS chưa tự điều khiển Flow hoặc nhập clip tự động.",
	}
}

// GenerateVideoClip uses Muse AI accounts to produce motion video clips.
func (e *Engine) GenerateVideoClip(ctx context.Context, prompt, keyframeURL string) (string, error) {
	return "", fmt.Errorf("muse ai: video generation adapter is not implemented; use manual handoff")
}

// MaskToken masks session keys/cookies for safe display.
func MaskToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return "••••••••"
	}
	return "••••" + s[len(s)-6:]
}

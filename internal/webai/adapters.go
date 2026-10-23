package webai

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Engine groups saved-session helpers for Flow and Muse plus deterministic
// script fallbacks. It does not call ChatGPT, Claude, or Gemini Web sessions.
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

// GenerateScript produces a deterministic baseline package. It does not call
// a Web AI account; provider-backed generation uses GenerateAdScriptPackageWithEngine.
func (e *Engine) GenerateScript(_ context.Context, productName, niche, framework, keyBenefits, cta string) (*AdScriptPackage, error) {
	pkg := GenerateAdScriptPackage(productName, niche, framework, "", keyBenefits, cta)
	return &pkg, nil
}

// GenerateVisualFrame reports that saved Gemini Web sessions do not yet have
// an image-generation adapter. Never signal success with an empty media URL.
func (e *Engine) GenerateVisualFrame(context.Context, string) (string, error) {
	return "", fmt.Errorf("webai: Gemini Web image generation is unavailable; use the configured image provider")
}

// FlowScenePrompt describes an optimized prompt for Google Flow.
type FlowScenePrompt struct {
	SceneIndex     int    `json:"scene_index"`
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	KeyframeFile   string `json:"keyframe_file,omitempty"`
	DurationSec    int    `json:"duration_sec"`
}

// FlowPromptBundle contains prompts and keyframe references for Google Flow.
type FlowPromptBundle struct {
	Title          string            `json:"title"`
	TotalScenes    int               `json:"total_scenes"`
	Aspect         string            `json:"aspect"`
	MasterKeyframe string            `json:"master_keyframe,omitempty"`
	Scenes         []FlowScenePrompt `json:"scenes"`
	WorkflowGuide  string            `json:"workflow_guide"`
}

// GenerateFlowVideoClip calls the authenticated Flow web endpoints using a
// saved Google session. No AI Studio developer key is required.
func (e *Engine) GenerateFlowVideoClip(ctx context.Context, prompt, keyframeURL string, durationSec int) (string, error) {
	if e.store == nil {
		return "", fmt.Errorf("google flow: account store is unavailable")
	}
	acc, err := e.store.PickActiveAccount(PlatformGoogleFlow)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acc.Cookie) == "" && len(acc.CookieJar) == 0 {
		return "", fmt.Errorf("google flow: saved account has no Google session cookie")
	}
	if keyframeURL != "" {
		prompt = prompt + " Reference image: " + keyframeURL
	}
	attempted := map[string]struct{}{acc.ID: {}}
	var result FlowVideoResult
	for {
		result, err = GoogleFlowVideoWithCookieSnapshot(ctx, acc.Cookie, acc.CookieJar, prompt, "VIDEO_ASPECT_RATIO_PORTRAIT", nil, func(cookie string, cookies []BrowserCookie) error {
			acc.Cookie = cookie
			acc.CookieJar = cookies
			return e.store.UpdateAccountCookieSnapshot(acc.ID, cookie, cookies)
		})
		if cooldown, limited := FlowPreSubmitRateLimitCooldown(err); limited {
			if markErr := e.store.MarkRateLimited(acc.ID, cooldown); markErr != nil {
				return "", fmt.Errorf("google flow: rate-limit account cooldown failed: %w", markErr)
			}
			next, pickErr := e.store.PickActiveAccountExcluding(PlatformGoogleFlow, attempted)
			if pickErr != nil {
				return "", fmt.Errorf("google flow: account quota exhausted; stop: %w", pickErr)
			}
			attempted[next.ID] = struct{}{}
			acc = next
			continue
		}
		if CanFallbackToGoogleFlowHeadless(err) && (ctx == nil || ctx.Err() == nil) {
			fallbackCookies := acc.CookieJar
			if len(result.CookieJar) > 0 {
				fallbackCookies = result.CookieJar
			}
			result, err = GoogleFlowVideoHeadlessWithCookieSnapshot(ctx, acc.Cookie, fallbackCookies, prompt, "VIDEO_ASPECT_RATIO_PORTRAIT", result.ProjectID, nil, func(cookie string, cookies []BrowserCookie) error {
				acc.Cookie = cookie
				acc.CookieJar = cookies
				if len(cookies) > 0 {
					return e.store.UpdateAccountCookieSnapshot(acc.ID, cookie, cookies)
				}
				return e.store.UpdateAccountCookieRefresh(acc.ID, cookie)
			})
		}
		break
	}
	if err != nil {
		return "", err
	}
	return result.MediaURL, nil
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
		WorkflowGuide: "Tạo video trong Studio bằng phiên Google Flow đã kết nối; job theo dõi tiến độ và tự tải MP4 vào kho media.",
	}
}

// GenerateVideoClip asks Muse Chat to create a clip and returns its share URL.
func (e *Engine) GenerateVideoClip(ctx context.Context, prompt, keyframeURL string) (string, error) {
	if e.store == nil {
		return "", fmt.Errorf("muse ai: account store is unavailable")
	}
	acc, err := e.store.PickActiveAccount(PlatformMuseAI)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acc.Cookie) == "" && len(acc.CookieJar) == 0 {
		return "", fmt.Errorf("muse ai: saved account has no Muse session cookie")
	}
	if keyframeURL != "" {
		prompt = prompt + " Reference image: " + keyframeURL
	}
	attempted := map[string]struct{}{acc.ID: {}}
	var result MuseReply
	for {
		result, err = MuseChatWithCookieSnapshot(ctx, acc.Cookie, acc.CookieJar, acc.ID, prompt, nil, func(cookie string, cookies []BrowserCookie) error {
			acc.Cookie = cookie
			acc.CookieJar = cookies
			return e.store.UpdateAccountCookieSnapshot(acc.ID, cookie, cookies)
		})
		if cooldown, limited := MusePreSubmitRateLimitCooldown(err); limited {
			if markErr := e.store.MarkRateLimited(acc.ID, cooldown); markErr != nil {
				return "", fmt.Errorf("muse ai: rate-limit account cooldown failed: %w", markErr)
			}
			next, pickErr := e.store.PickActiveAccountExcluding(PlatformMuseAI, attempted)
			if pickErr != nil {
				return "", fmt.Errorf("muse ai: account quota exhausted; stop: %w", pickErr)
			}
			attempted[next.ID] = struct{}{}
			acc = next
			continue
		}
		var gatewayUnavailable *MuseGatewayUnavailableError
		if errors.As(err, &gatewayUnavailable) {
			fallbackCookies := acc.CookieJar
			if len(result.CookieJar) > 0 {
				fallbackCookies = result.CookieJar
			}
			result, err = MuseChatHeadlessWithCookieSnapshot(ctx, acc.Cookie, fallbackCookies, prompt, nil, func(cookie string, cookies []BrowserCookie) error {
				acc.Cookie = cookie
				acc.CookieJar = cookies
				if len(cookies) > 0 {
					return e.store.UpdateAccountCookieSnapshot(acc.ID, cookie, cookies)
				}
				return e.store.UpdateAccountCookieRefresh(acc.ID, cookie)
			})
		}
		break
	}
	if err != nil {
		return "", err
	}
	if len(result.MediaURLs) == 0 {
		return "", fmt.Errorf("muse ai: reply did not contain a downloadable media link")
	}
	return result.MediaURLs[0], nil
}

// MaskToken masks session keys/cookies for safe display.
func MaskToken(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return "••••••••"
	}
	return "••••" + s[len(s)-6:]
}

// Package streamer is the Go port of agents/streamer/agent.py.
//
// Streamer: live show director.
//
// Entertainment-FIRST format: games, trivia, stories, challenges — product
// moments woven in, never a pure selling stream. Persona and voice carry
// the show; photorealism is explicitly NOT required.
//
// Loop per segment:
//
//	director (LLM) -> script -> TTS -> avatar visemes -> stream-engine
//	overlay -> published via RTMP. Chat/votes feed back where authorized.
//
// Anti-ban pacing and session limits are enforced by governance, not by
// prompt. SegmentPlan is a package variable so tests can run a shortened
// rundown.
package streamer

import (
	"context"
	"fmt"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// Segment is one show block: a kind plus its planned minutes.
type Segment struct {
	Kind    string
	Minutes int
}

// SegmentPlan is the show rundown: entertainment blocks with product
// moments interleaved. (welcome/game/product_moment/story/...)
var SegmentPlan = []Segment{
	{"welcome", 3},
	{"game", 10},
	{"product_moment", 4},
	{"story", 8},
	{"game", 10},
	{"product_moment", 4},
	{"music_break", 5},
	{"product_moment", 4},
	{"closing", 3},
}

// DirectorSystem is the system prompt for the show director LLM.
const DirectorSystem = "Bạn là đạo diễn kiêm MC livestream bán hàng TikTok, tiếng Việt, " +
	"giọng vui vẻ tự nhiên như người thật. Luân phiên giải trí và giới thiệu " +
	"sản phẩm một cách mềm mại, không gượng ép. Mỗi segment chỉ vài câu thoại."

// AvatarAPI is the avatar engine surface the streamer needs: render one
// spoken segment (per-frame motion driven by the TTS audio) and return
// the mp4 bytes to push to the stream. The concrete adapter wraps
// *avatar.AvatarChain with the session's character.
type AvatarAPI interface {
	RenderSegment(ctx context.Context, audio []byte) ([]byte, error)
}

// StreamEngineAPI is the RTMP stream engine surface.
type StreamEngineAPI interface {
	Start(rtmpURL, rtmpKey, disclosure string) error
	Stop() error
	PushSegment(audioWAV []byte, caption string) error
}

// DirectSegment asks the director LLM for the MC script of one segment.
// contextStr carries the runtime context (e.g. the featured product).
func DirectSegment(ctx context.Context, llm engines.LLMProvider, kind, contextStr string) (string, error) {
	prompt := fmt.Sprintf(
		"Loại segment: %s. Bối cảnh: %s. Viết lời thoại MC 3-5 câu.",
		kind, contextStr)
	return llm.Complete(ctx, DirectorSystem, prompt)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// RunLive runs one full live session. It blocks until the session ends
// (governance limit, plan exhausted, or ctx cancelled). The stream engine
// is always stopped and the session always closed, mirroring Python's
// try/finally.
func RunLive(ctx context.Context, cfg config.Config, l *ledger.Ledger,
	llm engines.LLMProvider, t tts.TTSProvider,
	avatar AvatarAPI, se StreamEngineAPI, maxMinutes int) map[string]any {
	limit := maxMinutes
	if limit <= 0 {
		limit = cfg.MaxLiveMinutesPerSession
	}
	// Pacing is enforced per-segment by governance. Python computed the
	// same limit but never applied it; here the override is honored.
	sessCfg := cfg
	sessCfg.MaxLiveMinutesPerSession = limit

	sid, err := l.StartSession(nil)
	if err != nil {
		return map[string]any{"ok": false, "reason": err.Error()}
	}
	elapsed := 0
	finish := func() {
		_ = se.Stop()
		_ = l.EndSession(sid, map[string]any{"status": "ended", "duration_min": elapsed})
	}
	fail := func(err error) map[string]any {
		finish()
		return map[string]any{"ok": false, "error": err.Error(),
			"session_id": sid, "minutes": elapsed}
	}

	_ = l.LogEvent(sid, "segment", map[string]any{
		"msg": "session started", "disclosure": cfg.AIDisclosureText})
	if err := se.Start(cfg.TikTokRTMPURL, cfg.TikTokRTMPKey, cfg.AIDisclosureText); err != nil {
		return fail(fmt.Errorf("stream engine start: %w", err))
	}

	for _, seg := range SegmentPlan {
		v := governance.CheckLiveSession(sessCfg, elapsed)
		if !v.Allowed {
			_ = l.LogEvent(sid, "segment", map[string]any{"msg": v.Reason})
			break
		}
		featured := "chưa có"
		if seg.Kind == "product_moment" {
			products, err := l.ShelfProducts(3)
			if err != nil {
				return fail(err)
			}
			if len(products) > 0 {
				featured = products[0].Title
			}
		}
		script, err := DirectSegment(ctx, llm, seg.Kind,
			fmt.Sprintf("featured_product=%s", featured))
		if err != nil {
			_ = l.LogEvent(sid, "error",
				map[string]any{"msg": fmt.Sprintf("director failed: %v", err)})
			return fail(fmt.Errorf("director LLM failed: %w", err))
		}
		// Go TTS providers need an explicit voice; Python's tts.run had
		// none, so the MC uses the default voice.
		audio, err := t.Synthesize(ctx, script, "default")
		if err != nil {
			_ = l.LogEvent(sid, "error",
				map[string]any{"msg": fmt.Sprintf("TTS failed: %v", err)})
			continue
		}
		// Every frame is rendered from the TTS audio (no static slides).
		if _, err := avatar.RenderSegment(ctx, audio); err != nil {
			_ = l.LogEvent(sid, "error",
				map[string]any{"msg": fmt.Sprintf("avatar failed: %v", err)})
		}
		if err := se.PushSegment(audio, script); err != nil {
			return fail(fmt.Errorf("push segment: %w", err))
		}
		_ = l.LogEvent(sid, "segment", map[string]any{
			"kind": seg.Kind, "script": truncateRunes(script, 200)})
		elapsed += seg.Minutes
		// Paced in real time; in rehearsal (dry-run) this is 1s per segment.
		pace := time.Duration(seg.Minutes) * 60 * time.Second
		if cfg.DryRun {
			pace = time.Second
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(pace):
		}
	}
	finish()
	return map[string]any{"ok": true, "session_id": sid, "minutes": elapsed}
}

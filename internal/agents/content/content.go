// Package content is the Go port of agents/content/agent.py.
//
// Content factory: short videos + AI short films, multi-platform.
//
// Per account, per persona:
//   storyteller -> short_film (AI illustrated films) + short_video (affiliate)
//   teacher      -> short_video (lessons)
//   gamer/coder  -> short_video (highlights/tips)
//   musician     -> ai_music / ai_remix (phase 3)
//
// Every rendered video is published through all configured publishers for
// the account (TikTok, Facebook Page, YouTube). YouTube additionally
// filters by the account's youtube_content_types — each channel only
// receives the kinds its owner enabled. Draft-first wherever the platform
// supports it.
//
// Semantic differences from the Python original:
//   - PickKind takes the YouTube content types explicitly (falls back to
//     account.YoutubeContentTypes when nil) instead of the ledger.
//   - RunForAccount does not insert a content_items row: the Go ledger
//     exposes no generic insert, so the "distributed" decision targets the
//     account username instead of a content-item id.
//   - The publisher builder is a package variable (newPublishers) so tests
//     can inject fakes without real platform credentials.
package content

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/agents/governance"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

// KindsByPersona maps a persona to the content kinds it produces, in
// priority order.
var KindsByPersona = map[string][]string{
	"storyteller": {"short_film", "short_video"},
	"teacher":     {"short_video"},
	"gamer":       {"short_video"},
	"coder":       {"short_video"},
	"musician":    {"ai_music", "ai_remix"},
	"dancer":      {"short_video"},
}

// newPublishers builds the platform publishers for an account. Overridable
// in tests to inject fakes.
var newPublishers = publishers.BuildPublishers

// gatedKinds are the kinds that need an enabled YouTube channel.
func gatedKind(k string) bool {
	return k == "ai_music" || k == "ai_remix" || k == "short_film"
}

// PickKind decides what this account produces. The persona gives the menu;
// YouTube-gated kinds are only picked when the channel allows them —
// except short_film, which storytellers can also send to TikTok/FB.
func PickKind(account *network.Account, youtubeContentTypes []string) string {
	ytc := youtubeContentTypes
	if ytc == nil {
		ytc = account.YoutubeContentTypes
	}
	kinds, ok := KindsByPersona[account.Persona]
	if !ok {
		kinds = []string{"short_video"}
	}
	for _, kind := range kinds {
		if gatedKind(kind) {
			allowed := false
			for _, y := range ytc {
				if y == kind {
					allowed = true
					break
				}
			}
			if allowed {
				return kind
			}
			// Storyteller films can also go to TikTok/FB without YouTube.
			if kind == "short_film" {
				return kind
			}
			continue
		}
		return kind
	}
	return "short_video"
}

// DefaultSlide is the fallback visual when no image model is wired: a
// title-ish color slide rendered with ffmpeg's lavfi source. It satisfies
// engines.ImageFunc.
func DefaultSlide(ctx context.Context, prompt, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "lavfi", "-i",
		"color=c=0x1a1a2e:s=1080x1920:d=1",
		"-frames:v", "1", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg slide: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// MakeShortVideo renders a single-scene affiliate/lesson video:
// cover image -> TTS narration -> one scene.
func MakeShortVideo(ctx context.Context, topic string, t tts.TTSProvider,
	imageFn engines.ImageFunc, workdir, voice string) (map[string]any, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, err
	}
	cover := filepath.Join(workdir, "cover.png")
	if err := imageFn(ctx, topic, cover); err != nil {
		return nil, fmt.Errorf("image: %w", err)
	}
	wav := filepath.Join(workdir, "voice.wav")
	if _, err := engines.SynthNarration(ctx, t, topic, voice, wav); err != nil {
		return nil, err
	}
	secs, err := engines.WavSeconds(wav)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(workdir, "short.mp4")
	if err := engines.RenderScene(ctx, cover, wav, secs+0.5, "9:16", out); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return map[string]any{
		"ok": true, "path": out,
		"seconds": math.Round(secs*10) / 10,
	}, nil
}

// Distribute publishes one video to every configured platform for the
// account. Per-platform results are returned and recorded in the ledger.
func Distribute(ctx context.Context, account *network.Account, kind, videoPath,
	title, description string, l *ledger.Ledger) []map[string]any {
	results := []map[string]any{}
	target := fmt.Sprintf("%s:%s", account.Username, kind)
	for _, pub := range newPublishers(account.Username, account.YoutubeChannel,
		account.YoutubeContentTypes, kind) {
		if !pub.Handles(kind) {
			continue
		}
		r := pub.Publish(ctx, videoPath, title, description, kind)
		results = append(results, map[string]any{
			"platform": pub.Name(), "ok": r.Ok,
			"remote_id": r.RemoteID, "url": r.URL,
			"draft": r.Draft, "error": r.Error,
		})
		detail := r.RemoteID
		if detail == "" {
			detail = r.Error
		}
		reason := fmt.Sprintf("%s ok=%v draft=%v %s", pub.Name(), r.Ok, r.Draft, detail)
		action := "published"
		if !r.Ok {
			action = "publish_failed"
		}
		_ = l.Decide("content", action, &target, truncateRunes(reason, 200),
			map[string]any{"platform": pub.Name(), "kind": kind,
				"remote_id": r.RemoteID, "url": r.URL, "draft": r.Draft,
				"error": r.Error})
	}
	return results
}

func pickTopic(account *network.Account) string {
	if len(account.Topics) > 0 {
		return account.Topics[0]
	}
	if account.Niche != "" {
		return account.Niche
	}
	return "giới thiệu"
}

func voiceFor(persona string) string {
	switch persona {
	case "storyteller":
		return "narrator"
	case "teacher":
		return "clear-teacher"
	default:
		return "default"
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func asString(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

// RunForAccount produces and distributes one content item for one account.
// The persona picks the kind; the account config picks the platforms.
func RunForAccount(ctx context.Context, cfg config.Config, l *ledger.Ledger,
	llm engines.LLMProvider, t tts.TTSProvider,
	account *network.Account, imageFn engines.ImageFunc) map[string]any {
	spent, err := l.DailySpendUSD()
	if err != nil {
		return map[string]any{"ok": false, "reason": err.Error()}
	}
	if v := governance.EvaluateAll(cfg, spent); !v.Allowed {
		return map[string]any{"ok": false, "reason": v.Reason}
	}
	if imageFn == nil {
		imageFn = DefaultSlide
	}

	kind := PickKind(account, nil)
	topic := pickTopic(account)
	workdir := filepath.Join("work", "content", account.Username, kind)
	voice := voiceFor(account.Persona)

	var made map[string]any
	switch kind {
	case "short_film":
		film, err := engines.MakeFilm(ctx, topic, llm, t, imageFn, workdir, voice, 90)
		if err != nil {
			return map[string]any{"ok": false, "reason": err.Error()}
		}
		made = map[string]any{"path": film.Path, "title": film.Title, "seconds": film.Seconds}
	case "ai_music", "ai_remix":
		// Phase 3: music pipeline lands here; v1 keeps the slot reserved.
		target := account.Username
		_ = l.Decide("content", "skip", &target,
			fmt.Sprintf("%s reserved for phase 3", kind), nil)
		return map[string]any{"ok": true, "skipped": kind}
	default:
		m, err := MakeShortVideo(ctx, topic, t, imageFn, workdir, voice)
		if err != nil {
			return map[string]any{"ok": false, "reason": err.Error()}
		}
		made = m
	}

	title := truncateRunes(asString(made, "title", topic), 100)
	path := asString(made, "path", "")
	pubs := Distribute(ctx, account, kind, path, title, title+" #aivideo", l)
	okPlatforms := []string{}
	for _, p := range pubs {
		if ok, _ := p["ok"].(bool); ok {
			if name, _ := p["platform"].(string); name != "" {
				okPlatforms = append(okPlatforms, name)
			}
		}
	}
	target := account.Username
	_ = l.Decide("content", "distributed", &target,
		fmt.Sprintf("%s -> %v", kind, okPlatforms),
		map[string]any{"kind": kind, "results": pubs})
	return map[string]any{"ok": true, "kind": kind, "path": path, "platforms": pubs}
}

// Run is the network entry: run for every onboarded account. Per-account
// panics are captured and recorded as content/error decisions, mirroring
// Python's per-account try/except.
func Run(ctx context.Context, cfg config.Config, l *ledger.Ledger,
	llm engines.LLMProvider, t tts.TTSProvider) map[string]any {
	mgr, err := network.NewAccountManager(l, cfg.DatabasePath)
	if err != nil {
		return map[string]any{"ok": false, "reason": err.Error()}
	}
	accounts, err := mgr.List("growing", "live_ready", "live")
	if err != nil {
		return map[string]any{"ok": false, "reason": err.Error()}
	}
	done := []map[string]any{}
	for _, acct := range accounts {
		func() {
			defer func() {
				if r := recover(); r != nil {
					target := acct.Username
					_ = l.Decide("content", "error", &target,
						truncateRunes(fmt.Sprintf("%v", r), 200), nil)
					done = append(done, map[string]any{
						acct.Username: map[string]any{"ok": false, "reason": fmt.Sprintf("%v", r)},
					})
				}
			}()
			done = append(done, map[string]any{
				acct.Username: RunForAccount(ctx, cfg, l, llm, t, acct, nil),
			})
		}()
	}
	return map[string]any{"ok": true, "accounts": done}
}

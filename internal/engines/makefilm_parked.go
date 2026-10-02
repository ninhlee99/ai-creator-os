//go:build parked

package engines

// makefilm_parked.go — the old end-to-end MakeFilm pipeline (hardcoded
// 9:16, superseded by the studio package's runFilm). It is only compiled
// with -tags parked (its sole caller, internal/agents/content, is parked
// too). BuildSRT, RenderScene, AssembleFilm, WavSeconds, PlanFilm and
// SynthNarration stay in film.go because they are still used live.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
)

// FilmResult is the outcome of MakeFilm.
type FilmResult struct {
	Path    string
	Title   string
	Seconds float64
	Scenes  int
}

// MakeFilm is the end-to-end pipeline: plan -> images -> narration ->
// scenes -> final mp4. Returns path, title, total seconds, scene count.
func MakeFilm(ctx context.Context, topic string, llm LLMProvider, t tts.TTSProvider, imageFn ImageFunc, workdir, voice string, targetSeconds int) (FilmResult, error) {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return FilmResult{}, err
	}
	plan, err := PlanFilm(ctx, llm, topic, targetSeconds, 6)
	if err != nil {
		return FilmResult{}, err
	}
	var scenes, narrations []string
	var durations []float64
	for i, sc := range plan.Scenes {
		img := filepath.Join(workdir, fmt.Sprintf("scene%d.png", i))
		imgPrompt := sc.ImagePrompt
		if imgPrompt == "" {
			imgPrompt = topic // fall back to topic when the LLM skipped the prompt
		}
		if err := imageFn(ctx, imgPrompt, img); err != nil {
			return FilmResult{}, fmt.Errorf("image scene %d: %w", i, err)
		}
		wavPath := filepath.Join(workdir, fmt.Sprintf("scene%d.wav", i))
		if _, err := SynthNarration(ctx, t, sc.Narration, voice, wavPath); err != nil {
			return FilmResult{}, err
		}
		wavSec, err := WavSeconds(wavPath)
		if err != nil {
			return FilmResult{}, fmt.Errorf("wav scene %d: %w", i, err)
		}
		dur := float64(sc.Seconds)
		if wavSec > dur {
			dur = wavSec
		}
		mp4 := filepath.Join(workdir, fmt.Sprintf("scene%d.mp4", i))
		if err := RenderScene(ctx, img, wavPath, dur, "9:16", mp4); err != nil {
			return FilmResult{}, err
		}
		scenes = append(scenes, mp4)
		narrations = append(narrations, sc.Narration)
		durations = append(durations, dur)
	}
	out := filepath.Join(workdir, "film.mp4")
	if err := AssembleFilm(ctx, scenes, BuildSRT(narrations, durations), out); err != nil {
		return FilmResult{}, err
	}
	total := 0.0
	for _, d := range durations {
		total += d
	}
	return FilmResult{Path: out, Title: plan.Title, Seconds: total, Scenes: len(scenes)}, nil
}

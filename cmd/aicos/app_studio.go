package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

func initializeStudio(srv *web.Server, dataDir, outDir string, geminiKeys []string, llmChain *engines.LLMChain, ttsChain *tts.TTSChain) *studio.Studio {
	// Studio reuses the app's LLM, TTS and Gemini image providers.
	mediaGen := studio.NewGeminiMediaGen(geminiKeys)
	narrator := studio.Narrator(func(ctx context.Context, text, voice string) ([]byte, error) {
		if voice == "" {
			voice = "default"
		}
		return ttsChain.Synthesize(ctx, text, voice)
	})
	st, err := studio.New(filepath.Join(dataDir, "studio.db"), studioLLMAdapter{c: llmChain}, mediaGen, narrator, outDir)
	if err != nil {
		log.Printf("studio: init failed: %v (studio page disabled)", err)
		return nil
	}

	srv.Studio = st
	// Keys saved in Settings also update image generation after startup.
	srv.OnGeminiKeys = mediaGen.SetKeys
	if keys := srv.SyncGeminiKeys(); len(keys) > 0 {
		log.Printf("gemini: %d key dùng chung cho viết chữ + giọng đọc + tạo ảnh", len(keys))
	}
	st.DiskGuard = func() (int, error) { return automation.DiskUsagePct(dataDir) }
	if n := st.MarkInterruptedJobs(); n > 0 {
		log.Printf("studio: %d job(s) interrupted by restart — marked failed (resume from Studio)", n)
	}
	log.Printf("studio: ready (mediagen=%s keys=%d)", mediaGen.Name(), mediaGen.KeyCount())
	return st
}

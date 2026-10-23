package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

func initializeStudio(srv *web.Server, dataDir, outDir string, geminiKeys []string, llmChain *engines.LLMChain, ttsChain *tts.TTSChain) *studio.Studio {
	// Studio reuses the app's LLM, TTS and Gemini image-analysis providers.
	// Prompt-only image generation goes through Pollinations Free API Classic.
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
	// Keys saved in Settings also update Gemini image analysis after startup.
	srv.OnGeminiKeys = mediaGen.SetKeys
	if keys := srv.SyncGeminiKeys(); len(keys) > 0 {
		log.Printf("gemini: %d key dùng chung cho viết chữ + giọng đọc + phân tích ảnh", len(keys))
	}
	st.DiskGuard = func() (int, error) { return automation.DiskUsagePct(dataDir) }
	if n := st.MarkInterruptedJobs(); n > 0 {
		log.Printf("studio: %d job(s) interrupted by restart — marked failed (resume from Studio)", n)
	}
	if n, err := st.RecoverInterruptedPublishLegs(); err != nil {
		log.Printf("studio: could not recover interrupted publish legs: %v", err)
	} else if n > 0 {
		log.Printf("studio: %d interrupted publish leg(s) need platform review before retry", n)
	}
	if srv.Growth != nil {
		items, err := srv.Growth.InterruptedEnqueueItems()
		if err != nil {
			log.Printf("growth: could not recover interrupted production claims: %v", err)
		} else {
			for _, item := range items {
				jobID, found, err := st.GrowthStoryJob(item.AccountID, item.ID)
				if err != nil {
					log.Printf("growth: could not reconcile item %d: %v", item.ID, err)
					continue
				}
				if found {
					err = srv.Growth.SetItemProduction(item.ID, jobID,
						growth.VariantTitle(item), growth.VariantCaption(item))
				} else {
					err = srv.Growth.SetItemStatus(item.ID, growth.ItemPlanned,
						"Không tìm thấy Studio job đã tạo; Growth sẽ thử lại mục này.")
				}
				if err != nil {
					log.Printf("growth: could not finish recovery for item %d: %v", item.ID, err)
				}
			}
			if len(items) > 0 {
				log.Printf("growth: reconciled %d interrupted production claim(s)", len(items))
			}
		}
	}
	log.Printf("studio: ready (mediagen=%s keys=%d)", mediaGen.Name(), mediaGen.KeyCount())
	return st
}

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

func initializeProducts(srv *web.Server, mgr *network.AccountManager, settings automation.LedgerSettings, dataDir string) (*products.Store, bool) {
	if srv.Studio == nil {
		return nil, false
	}
	store, err := products.NewStore(filepath.Join(dataDir, "products.db"))
	if err != nil {
		log.Printf("products: init failed: %v (product discovery disabled)", err)
		return nil, false
	}
	srv.Products = store
	if n, err := srv.MigrateLedgerShelf(); err != nil {
		log.Printf("products: migrate legacy ledger shelf: %v", err)
	} else if n > 0 {
		log.Printf("products: moved %d legacy ledger products into the shared store", n)
	}
	if err := automation.MigrateProductsSettings(store, settings); err != nil {
		log.Printf("settings: migrate products settings: %v", err)
	}
	if removed, err := store.RemoveSource("market_verified"); err != nil {
		log.Printf("products: remove unverified catalog failed: %v", err)
	} else if removed > 0 {
		log.Printf("products: removed %d unverified catalog rows", removed)
	}

	models, err := srv.Studio.ModelLibrary()
	if err != nil {
		log.Printf("autopilot: model library failed: %v", err)
		return store, false
	}
	configureAutopilot(srv, mgr, store, models, settings, dataDir)
	return store, true
}

func configureAutopilot(srv *web.Server, mgr *network.AccountManager, store *products.Store, models *studio.ModelStore, settings automation.LedgerSettings, dataDir string) {
	autopilot := studio.NewAutopilot(srv.Studio, mgr, store, nil, models)
	if path := filepath.Join(dataDir, "autopilot-music.m4a"); fileExists(path) {
		autopilot.MusicPath = path
	} else if path := filepath.Join(dataDir, "trending-audio.m4a"); fileExists(path) {
		autopilot.MusicPath = path
	}
	srv.Autopilot = autopilot
	log.Printf("autopilot: ready (nguồn sản phẩm: Accesstrade store)")
	publishHook := srv.AutomationService().AutoPublishHook()
	srv.Studio.SetOnDone(func(jobID string) {
		if publishHook != nil {
			publishHook(jobID)
		}
		go notifyCompletedStudioJob(srv, jobID)
	})
	seedAutopilotSchedule(settings)
}

func notifyCompletedStudioJob(srv *web.Server, jobID string) {
	job, ok := srv.Studio.GetJob(jobID)
	if !ok {
		return
	}
	title := job.Title
	if title == "" {
		title = "Video AI Studio"
	}
	msg := fmt.Sprintf("🎬 [STUDIO] ✨ Video hoàn tất: \"%s\" (ID: %s, Thể loại: %s)!\nĐã sẵn sàng xuất bản đa kênh.", title, jobID, job.Kind)
	bot, _ := srv.EffectiveEnv("TELEGRAM_BOT_TOKEN")
	chat, _ := srv.EffectiveEnv("TELEGRAM_CHAT_ID")
	if (bot == "" || chat == "") && srv.Ledger != nil {
		b, _, _ := srv.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
		c, _, _ := srv.Ledger.GetSetting(automation.KeyNotifyTelegramChat)
		if b != "" && c != "" {
			bot, chat = b, c
		}
	}
	if bot != "" && chat != "" {
		_ = notify.SendTelegram(bot, chat, msg)
	}
	if discord, _ := srv.EffectiveEnv("DISCORD_WEBHOOK_URL"); discord != "" {
		_ = notify.SendDiscord(discord, "✨ Video Studio Hoàn Tất", msg)
	}
}

func seedAutopilotSchedule(settings automation.LedgerSettings) {
	if _, ok := settings.Get(web.SettingAutopilotEnabled); ok {
		return
	}
	if _, present := os.LookupEnv("AUTOPILOT_SCHEDULE"); !present {
		return
	}
	if getenvBool("AUTOPILOT_SCHEDULE", false) {
		_ = settings.Set(web.SettingAutopilotEnabled, "1")
		_ = settings.Set(web.SettingAutopilotInterval, strconv.Itoa(getenvInt("AUTOPILOT_INTERVAL_HOURS", 6)))
		return
	}
	_ = settings.Set(web.SettingAutopilotEnabled, "0")
}

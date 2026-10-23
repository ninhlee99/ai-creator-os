package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/ticksup"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

type appWeb struct {
	manager  *network.AccountManager
	server   *web.Server
	config   *web.Config
	settings automation.LedgerSettings
	tickSup  *ticksup.Supervisor
	outDir   string
}

func initializeWeb(ctx context.Context, l *ledger.Ledger, dataDir, dbPath string, firstRun bool, llmChain *engines.LLMChain, ttsChain *tts.TTSChain) (*appWeb, error) {
	health := make(map[string]web.HealthChecker)
	for _, name := range llmChain.ActiveProviders() {
		if provider, ok := llmChain.Provider(name); ok {
			if checker, ok := any(provider).(web.HealthChecker); ok {
				health["llm:"+name] = checker
			}
		}
	}
	for _, name := range ttsChain.ActiveProviders() {
		if provider, ok := ttsChain.Provider(name); ok {
			if checker, ok := any(provider).(web.HealthChecker); ok {
				health["tts:"+name] = checker
			}
		}
	}
	log.Printf("health: %d probes wired", len(health))

	manager, err := network.NewAccountManager(l, dbPath)
	if err != nil {
		return nil, fmt.Errorf("accounts: %w", err)
	}
	config := web.LoadConfig()
	config.DatabasePath = dbPath
	config.Version = version
	server, err := web.NewServer(config, l, manager, dbPath)
	if err != nil {
		return nil, fmt.Errorf("web: %w", err)
	}
	if server.Growth != nil {
		if n, err := server.Growth.RecoverInterruptedPublishingItems(); err != nil {
			log.Printf("growth: could not recover interrupted uploads: %v", err)
		} else if n > 0 {
			log.Printf("growth: %d interrupted upload(s) need YouTube review before retry", n)
		}
	}
	server.Onboarding = firstRun
	server.RefreshGeminiFreeTierGate()
	server.LLM = llmChainAdapter{c: llmChain}
	server.TTS = ttsChainAdapter{c: ttsChain}
	server.Health = health

	settings := automation.LedgerSettings{L: l}
	outDir := filepath.Join(dataDir, "output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("web: out dir: %w", err)
	}
	server.OutDir = outDir

	// OAuth tokens live under the app data directory; the worker refreshes them in the background.
	publishers.SetTokenDir(filepath.Join(dataDir, "tokens"))
	publishers.MigrateTokensFromCWD()
	publishers.StartTokenWorker(ctx, 30*time.Minute)
	publishers.SetOnTokenAlert(func(rep publishers.TokenHealthReport) {
		msg := fmt.Sprintf("⚠️ [Cảnh báo Token] Kênh %s (%s) bị gián đoạn: %s. Vui lòng kết nối lại trong Cài đặt.", rep.Username, rep.Platform, rep.Message)
		bot, _, _ := l.GetSetting(automation.KeyNotifyTelegramBot)
		chat, _, _ := l.GetSetting(automation.KeyNotifyTelegramChat)
		if bot != "" && chat != "" {
			_ = notify.SendTelegram(bot, chat, msg)
		}
		webhook, _, _ := l.GetSetting(automation.KeyNotifyWebhookURL)
		if webhook != "" {
			_ = notify.SendDiscord(webhook, "⚠️ Cảnh Báo Kết Nối Kênh", msg)
		}
	})

	tickSup := ticksup.New(
		func() ticksup.Alerter { return server.AutomationService() },
		tts.GeminiQuotaCircuit().TakeAlert,
	)
	server.TickBeats = func() map[string]web.TickBeat {
		out := make(map[string]web.TickBeat)
		for name, beat := range tickSup.Snapshot() {
			out[name] = web.TickBeat{LastStart: beat.LastStart, LastOK: beat.LastOK, LastErr: beat.LastErr, Runs: beat.Runs}
		}
		return out
	}
	server.QuotaCircuit = func() (bool, string) {
		circuit := tts.GeminiQuotaCircuit()
		if !circuit.Open() {
			return false, ""
		}
		return true, circuit.OpenUntil().Format("02/01 15:04")
	}
	server.RemoteNeedsPassword = true
	server.StaleTicks = func() []string { return tickSup.Stale(time.Now()) }

	return &appWeb{manager: manager, server: server, config: config, settings: settings, tickSup: tickSup, outDir: outDir}, nil
}

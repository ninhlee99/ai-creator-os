package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/harness"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/runtimeguard"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/ticksup"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

func startWorkersAndServe(ctx context.Context, cancel context.CancelFunc, addr string, srv *web.Server, l *ledger.Ledger, mgr *network.AccountManager, llmChain *engines.LLMChain, webCfg *web.Config, settings automation.LedgerSettings, tickSup *ticksup.Supervisor, autopilotArmed bool, guard *runtimeguard.Guard, studioInst *studio.Studio) {
	bioRetentionDone := startBioTryOnRetentionWorker(ctx, srv, guard)
	if srv != nil && srv.Products != nil && guard != nil {
		guard.Go(ctx, "bio:tryon-renderer", srv.RunBioTryOnWorker)
	}

	if err := automation.SeedMasterSwitch(settings, getenvBool("MASTER_SWITCH", false)); err != nil {
		log.Printf("settings: seed master switch: %v", err)
	}
	netCfg := network.DefaultNetConfig()
	netCfg.MasterSwitch = automation.MasterOn(settings)
	netCfg.MasterGate = automation.MasterSwitchGate{Settings: settings}
	netCfg.DryRun = webCfg.DryRun()
	netCfg.KillSwitch = getenvBool("KILL_SWITCH", false)
	netCfg.Gate = webCfg
	log.Printf("network: master=%v dry_run=%v kill_switch=%v", netCfg.MasterSwitch, netCfg.DryRun, netCfg.KillSwitch)
	daemon := network.NewDaemon(l, mgr, llmChain, netCfg)
	guard.Go(ctx, "network:daemon", func(c context.Context) {
		for {
			select {
			case <-c.Done():
				return
			default:
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("daemon: PANIC recovered, khởi động lại vòng sau: %v", r)
					}
				}()
				daemon.Run(c, 60*time.Second)
			}()
			select {
			case <-c.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	})

	ticksup.ArmLoops(ctx, tickSup, ticksup.Deps{
		NewService: srv.AutomationService,
		Autopilot:  autopilotArmed,
		Growth:     srv.Growth != nil,
		AT:         srv.AT != nil,
		Story:      srv.Studio != nil,
		Viral:      srv.Studio != nil,
	})

	startHarness(ctx, srv, mgr, settings, webCfg, llmChain)

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		log.Printf("http: listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("shutdown: signal received, stopping...")
	cancel()

	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	if err := httpSrv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	if studioInst != nil {
		log.Printf("shutdown: chờ job studio đang chạy...")
		studioInst.Shutdown(30 * time.Second)
	}
	<-bioRetentionDone
	log.Printf("shutdown: complete")
}

func startBioTryOnRetentionWorker(ctx context.Context, srv *web.Server, guard *runtimeguard.Guard) <-chan struct{} {
	done := make(chan struct{})
	if srv == nil || srv.Products == nil || guard == nil || srv.DataDir == "" {
		close(done)
		return done
	}
	guard.Go(ctx, "bio:tryon-retention", func(ctx context.Context) {
		defer close(done)
		cleanup := func() {
			dedupeRemoved, dedupeErr := srv.Products.CleanupExpiredBioEventDedupe(time.Now())
			if dedupeErr != nil {
				log.Printf("bio analytics retention: cleanup failed: %v", dedupeErr)
			} else if dedupeRemoved > 0 {
				log.Printf("bio analytics retention: removed %d expired dedupe keys", dedupeRemoved)
			}

			removed, err := srv.Products.CleanupExpiredBioTryOnResults(
				filepath.Join(srv.DataDir, products.BioTryOnResultsDir), time.Now(),
			)
			if err != nil {
				log.Printf("bio try-on retention: cleanup failed: %v", err)
				return
			}
			if removed > 0 {
				log.Printf("bio try-on retention: removed %d expired jobs/results", removed)
			}
		}

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			cleanup()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	return done
}

func startHarness(ctx context.Context, srv *web.Server, mgr *network.AccountManager, settings automation.LedgerSettings, webCfg *web.Config, llmChain *engines.LLMChain) {
	autonomyLevel := harness.AutonomyAssisted
	if stored, ok := settings.Get("harness.autonomy_level"); ok {
		if n, err := strconv.Atoi(stored); err == nil && n >= int(harness.AutonomyAssisted) && n <= int(harness.AutonomyFullyAutonomous) {
			autonomyLevel = harness.AutonomyLevel(n)
		}
	}
	harnessInst, err := harness.New(
		srv.DB(), srv.Studio, mgr, srv.Growth, studioLLMAdapter{c: llmChain},
		harness.Config{
			AutonomyLevel: autonomyLevel,
			Interval:      6 * time.Hour,
			ExecutionAllowed: func() bool {
				return automation.MasterSwitchGate{Settings: settings}.MasterEnabled() && webCfg.LiveEnabled()
			},
		},
	)
	if err != nil {
		log.Printf("harness: init failed: %v", err)
		return
	}
	srv.Harness = harnessInst
	if srv.Studio != nil && harnessInst.Memory() != nil {
		srv.Studio.PromptEnhancer = harnessInst.Memory().GeneratePromptContext
	}
	harnessInst.StartBackgroundLoop(ctx)
	log.Printf("harness: OODA background loop started (configured level %d; production actions still require supported inputs)", harnessInst.AutonomyLevel())
}

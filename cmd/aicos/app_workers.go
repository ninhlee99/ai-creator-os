package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/ticksup"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

func startWorkersAndServe(ctx context.Context, cancel context.CancelFunc, addr string, srv *web.Server, settings automation.LedgerSettings, tickSup *ticksup.Supervisor, autopilotArmed bool, studioInst *studio.Studio) {
	if err := automation.SeedMasterSwitch(settings, getenvBool("MASTER_SWITCH", false)); err != nil {
		log.Printf("settings: seed master switch: %v", err)
	}
	ticksup.ArmLoops(ctx, tickSup, ticksup.Deps{
		NewService: srv.AutomationService,
		Autopilot:  autopilotArmed,
		Growth:     srv.Growth != nil,
		AT:         srv.AT != nil,
		Studio:     srv.Studio != nil,
	})

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 300 * time.Second,
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
	log.Printf("shutdown: complete")
}

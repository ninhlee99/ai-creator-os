package main

import (
	"log"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/web"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func initializeAccesstrade(srv *web.Server, dataDir string) *accesstrade.Store {
	store, err := accesstrade.NewStore(filepath.Join(dataDir, "accesstrade.db"))
	if err != nil {
		log.Printf("accesstrade: init failed: %v (Accesstrade disabled)", err)
		return nil
	}
	srv.AT = store
	log.Printf("accesstrade: ready")
	return store
}

func initializeWebAI(srv *web.Server, dataDir string) *webai.Store {
	store, err := webai.NewStore(filepath.Join(dataDir, "webai.db"))
	if err != nil {
		log.Printf("webai: init failed: %v", err)
		return nil
	}
	srv.WebAI = store
	srv.WireStudioWebAI()
	log.Printf("webai: ready (%d accounts connected)", store.CountConnected())
	return store
}

package main

import (
	"log"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/web"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func initializeWebAI(srv *web.Server, dataDir string) *webai.Store {
	store, err := webai.NewStore(filepath.Join(dataDir, "webai.db"))
	if err != nil {
		log.Printf("webai: init failed: %v", err)
		return nil
	}
	srv.WebAI = store
	srv.WireStudioWebAI()
	log.Printf("webai: store opened (%d accounts marked connected; provider access not checked)", store.CountConnected())
	return store
}

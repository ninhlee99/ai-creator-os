// Command aicos is the single binary for AI Creator OS.
//
// One binary, zero runtime dependencies: it serves the web dashboard,
// runs the network daemon (accounts, onboarding, scheduler, topics),
// wires the LLM/TTS provider chains and supervises the local sidecars
// (llama-server, VieNeu TTS).
//
// Usage:
//
//	aicos                 # serve dashboard on :8080, run workers
//	aicos -addr :9000     # custom listen address
//	aicos -data ./data    # custom data directory
//
// Environment:
//
//	GEMINI_API_KEY        free-tier key for the gemini LLM/TTS providers
//	DRY_RUN               network daemon dry-run (default true)
//	KILL_SWITCH           emergency stop for the network daemon (default false)
//	MASTER_SWITCH         network daemon on/off (default false)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

var version = "v0.5-go"

// decideAdapter adapts *ledger.Ledger to engines.DecisionLogger: the ledger
// returns errors, the chain interface does not — log instead of crashing.
type decideAdapter struct{ l *ledger.Ledger }

func (d decideAdapter) Decide(agent, action string, target *string, reason string, inputs map[string]any) {
	if err := d.l.Decide(agent, action, target, reason, inputs); err != nil {
		log.Printf("decide(%s/%s): %v", agent, action, err)
	}
}

// ttsChainAdapter adapts *engines.TTSChain to web.TTSChainAPI. The engines
// chain takes a typed ChainConfig; the web layer passes raw JSON.
type ttsChainAdapter struct{ c *engines.TTSChain }

func (a ttsChainAdapter) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	return a.c.Synthesize(ctx, text, voice)
}

func (a ttsChainAdapter) ProviderNames() []string { return a.c.ActiveProviders() }

func (a ttsChainAdapter) SetConfig(raw json.RawMessage) {
	var cfg engines.ChainConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Printf("tts: SetConfig: invalid JSON, keeping current config: %v", err)
		return
	}
	a.c.SetConfig(cfg)
	log.Printf("tts: config updated via dashboard: active=%v", a.c.ActiveProviders())
}

// vieNeuAdapter adapts *tts.VieNeuProvider to web.VieNeuCtl so the settings
// page can control the sidecar.
type vieNeuAdapter struct{ v *tts.VieNeuProvider }

func (a vieNeuAdapter) Status() (string, string) {
	s := a.v.Status()
	return s.State, s.Detail
}

func (a vieNeuAdapter) Start(ctx context.Context) error { return a.v.Start(ctx) }

func (a vieNeuAdapter) Stop() error { return a.v.Stop() }

func (a vieNeuAdapter) Restart(ctx context.Context) error { return a.v.Restart(ctx) }

func (a vieNeuAdapter) SetVoice(preset string) error {
	a.v.SetVoice(preset)
	return nil
}

func (a vieNeuAdapter) EnsureModel(ctx context.Context, onProgress func(downloaded, total int64)) error {
	return a.v.EnsureModel(ctx, onProgress)
}

func (a vieNeuAdapter) Voices() []string {
	vv, err := a.v.Voices(context.Background())
	if err != nil || len(vv) == 0 {
		return web.VieNeuVoiceFallback
	}
	names := make([]string, 0, len(vv))
	for _, v := range vv {
		if v.Name != "" {
			names = append(names, v.Name)
		} else {
			names = append(names, v.ID)
		}
	}
	return names
}

func getenvBool(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func main() {
	addr := flag.String("addr", ":8080", "listen address for the web dashboard")
	dataDir := flag.String("data", "./data", "data directory (sqlite db, models, jobs, output)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("aicos", version)
		os.Exit(0)
	}

	log.Printf("aicos %s starting (addr=%s data=%s)", version, *addr, *dataDir)

	// -- 1. data dir + ledger ---------------------------------------------
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	dbPath := *dataDir + "/ledger.db"
	l, err := ledger.New(dbPath)
	if err != nil {
		log.Fatalf("ledger: %v", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			log.Printf("ledger close: %v", err)
		}
	}()
	log.Printf("ledger: opened %s", dbPath)

	// -- 2. decision logger ------------------------------------------------
	decider := decideAdapter{l: l}

	// -- 3. chain configs from the settings table --------------------------
	geminiKey := os.Getenv("GEMINI_API_KEY")
	loadChain := func(key string, def engines.ChainConfig) (engines.ChainConfig, bool) {
		raw, ok, err := l.GetSetting(key)
		if err != nil {
			log.Printf("settings %s: read error, using default: %v", key, err)
			return def, false
		}
		if !ok || raw == "" {
			return def, false
		}
		var cfg engines.ChainConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil || len(cfg.Order) == 0 {
			log.Printf("settings %s: invalid JSON, using default", key)
			return def, false
		}
		return cfg, true
	}
	cfgSrc := func() (engines.ChainConfig, engines.ChainConfig) {
		llmCfg, llmSaved := loadChain(ledger.SettingLLMChain, engines.DefaultLLMConfig(geminiKey))
		ttsCfg, ttsSaved := loadChain(ledger.SettingTTSChain, engines.DefaultTTSConfig(geminiKey))
		if llmSaved {
			log.Printf("llm: using saved chain config (llm.chain)")
		} else {
			log.Printf("llm: using default chain config")
		}
		if ttsSaved {
			log.Printf("tts: using saved chain config (tts.chain)")
		} else {
			log.Printf("tts: using default chain config")
		}
		return llmCfg, ttsCfg
	}

	// -- 4. provider chains -------------------------------------------------
	llmChain, ttsChain := engines.DefaultChains(*dataDir, geminiKey, decider, cfgSrc)
	log.Printf("llm: active providers: %v", llmChain.ActiveProviders())
	log.Printf("tts: active providers: %v", ttsChain.ActiveProviders())
	if geminiKey == "" {
		log.Printf("note: GEMINI_API_KEY not set — gemini tiers will fail over to local/edge tiers")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// -- 5. sidecars --------------------------------------------------------
	reg := engines.DefaultRegistry(*dataDir)

	const llamaModel = "qwen2.5-7b-instruct-q4_k_m.gguf"
	// llama-server runs on 127.0.0.1:8081 (see engines.NewLlamaServerProcess)
	// so it never collides with the dashboard's default -addr :8080.
	llamaProc := engines.NewLlamaServerProcess(reg)
	if reg.Has(llamaModel) {
		go func() {
			log.Printf("sidecar: llama-server starting (model %s)", llamaModel)
			if err := llamaProc.Start(ctx); err != nil {
				log.Printf("sidecar: llama-server failed to start: %v", err)
			} else {
				log.Printf("sidecar: llama-server running")
			}
		}()
	} else {
		log.Printf("sidecar: llama-server not started — model %s chưa được tải (dashboard settings để tải)", llamaModel)
	}

	// VieNeu TTS sidecar: controlled through the provider inside the chain.
	var vieNeu *tts.VieNeuProvider
	if p, ok := ttsChain.Provider("vieneu"); ok {
		if vn, ok := p.(*tts.VieNeuProvider); ok {
			vieNeu = vn
			if reg.VieNeuPresent() {
				go func() {
					log.Printf("sidecar: vieneu starting (first run can take a while)")
					if err := vieNeu.Start(ctx); err != nil {
						log.Printf("sidecar: vieneu failed to start: %v", err)
					} else {
						log.Printf("sidecar: vieneu running")
					}
				}()
			} else {
				log.Printf("sidecar: vieneu not started — repo chưa có tại data dir (dashboard settings để tải/cài đặt)")
			}
		}
	}

	// health probes for the dashboard "Kiểm tra kết nối" buttons
	health := map[string]web.HealthChecker{}
	for _, name := range llmChain.ActiveProviders() {
		if p, ok := llmChain.Provider(name); ok {
			if h, ok := any(p).(web.HealthChecker); ok {
				health["llm/"+name] = h
			}
		}
	}
	for _, name := range ttsChain.ActiveProviders() {
		if p, ok := ttsChain.Provider(name); ok {
			if h, ok := any(p).(web.HealthChecker); ok {
				health["tts/"+name] = h
			}
		}
	}
	log.Printf("health: %d probes wired", len(health))

	// -- 6. accounts + web server -------------------------------------------
	mgr, err := network.NewAccountManager(l, dbPath)
	if err != nil {
		log.Fatalf("accounts: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			log.Printf("accounts close: %v", err)
		}
	}()

	webCfg := web.LoadConfig()
	webCfg.DatabasePath = dbPath
	srv, err := web.NewServer(webCfg, l, mgr, dbPath)
	if err != nil {
		log.Fatalf("web: %v", err)
	}
	defer func() {
		if err := srv.Close(); err != nil {
			log.Printf("web close: %v", err)
		}
	}()
	srv.LLM = llmChain // *engines.LLMChain satisfies web.LLMClient
	srv.TTS = ttsChainAdapter{c: ttsChain}
	srv.Health = health
	if vieNeu != nil {
		srv.VieNeu = vieNeuAdapter{v: vieNeu}
	}

	// -- 7. network daemon ---------------------------------------------------
	netCfg := network.DefaultNetConfig()
	netCfg.MasterSwitch = getenvBool("MASTER_SWITCH", false)
	netCfg.DryRun = getenvBool("DRY_RUN", true)
	netCfg.KillSwitch = getenvBool("KILL_SWITCH", false)
	log.Printf("network: master=%v dry_run=%v kill_switch=%v", netCfg.MasterSwitch, netCfg.DryRun, netCfg.KillSwitch)
	daemon := network.NewDaemon(l, mgr, llmChain, netCfg) // *engines.LLMChain satisfies network.LLMClient
	go daemon.Run(ctx, 60*time.Second)

	// -- 8. http server + graceful shutdown ----------------------------------
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Routes()}
	go func() {
		log.Printf("http: listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Printf("shutdown: signal received, stopping...")

	cancel() // stops the daemon loop and sidecar start contexts

	shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shCancel()
	if err := httpSrv.Shutdown(shCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	if vieNeu != nil {
		if err := vieNeu.Stop(); err != nil {
			log.Printf("sidecar: vieneu stop: %v", err)
		}
	}
	if err := llamaProc.Stop(); err != nil {
		log.Printf("sidecar: llama-server stop: %v", err)
	}

	log.Printf("shutdown: complete")
}

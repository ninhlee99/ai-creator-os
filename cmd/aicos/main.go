// Command aicos is the single binary for AI Creator OS.
//
// One binary, zero runtime dependencies: it serves the web dashboard,
// runs the network daemon (accounts, onboarding, scheduler, topics),
// wires the LLM/TTS provider chains and supervises the local sidecars
// (llama-server, VieNeu TTS).
//
// Usage:
//
//	aicos                 # serve dashboard on 127.0.0.1:8080, run workers
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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

// version mặc định khi build thường; build phát hành truyền
// -ldflags "-X main.version=vX.Y.Z" để đóng dấu thật.
var version = "dev"

// decideAdapter adapts *ledger.Ledger to engines.DecisionLogger: the ledger
// returns errors, the chain interface does not — log instead of crashing.
type decideAdapter struct{ l *ledger.Ledger }

func (d decideAdapter) Decide(agent, action string, target *string, reason string, inputs map[string]any) {
	if err := d.l.Decide(agent, action, target, reason, inputs); err != nil {
		log.Printf("decide(%s/%s): %v", agent, action, err)
	}
}

// ttsChainAdapter adapts *engines.TTSChain to web.TTSChainAPI. The engines
// chain takes a typed ChainConfig; the web layer passes raw JSON in the
// dashboard contract (timeouts in seconds) — decoded by tts.ParseChainJSON.
type ttsChainAdapter struct{ c *engines.TTSChain }

func (a ttsChainAdapter) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	return a.c.Synthesize(ctx, text, voice)
}

func (a ttsChainAdapter) ProviderNames() []string { return a.c.ActiveProviders() }

func (a ttsChainAdapter) SetConfig(raw json.RawMessage) {
	cfg, err := tts.ParseChainJSON(raw)
	if err != nil {
		log.Printf("tts: SetConfig: invalid JSON, keeping current config: %v", err)
		return
	}
	a.c.SetConfig(cfg)
	log.Printf("tts: config updated via dashboard: active=%v", a.c.ActiveProviders())
}

// KeyStatus exposes per-key rotation state for the settings page.
func (a ttsChainAdapter) KeyStatus(provider string) []web.KeyStatus {
	return toWebKeyStatus(a.c.KeyStatus(provider))
}

// ValidateKey tests one API key of a provider (settings "test" button).
func (a ttsChainAdapter) ValidateKey(ctx context.Context, provider string, idx int) error {
	return a.c.ValidateKey(ctx, provider, idx)
}

// llmChainAdapter adapts *engines.LLMChain to the web layer: it satisfies
// web.LLMClient (Complete + Name) and additionally offers raw-JSON
// SetConfig plus key-status / key-test for the settings page. (The typed
// SetConfig(ChainConfig) cannot satisfy the web's SetConfig(json.RawMessage)
// interface, hence this adapter.)
type llmChainAdapter struct{ c *engines.LLMChain }

func (a llmChainAdapter) Complete(ctx context.Context, system, prompt string) (string, error) {
	return a.c.Complete(ctx, system, prompt)
}

func (a llmChainAdapter) Name() string { return a.c.Name() }

// studioLLMAdapter adapts *engines.LLMChain to studio.LLM (the director).
type studioLLMAdapter struct{ c *engines.LLMChain }

func (a studioLLMAdapter) Complete(ctx context.Context, system, prompt string) (string, error) {
	return a.c.Complete(ctx, system, prompt)
}

func (a studioLLMAdapter) Name() string { return a.c.Name() }

func (a studioLLMAdapter) Healthy(context.Context) bool { return len(a.c.ActiveProviders()) > 0 }

func (a llmChainAdapter) SetConfig(raw json.RawMessage) {
	cfg, err := tts.ParseChainJSON(raw)
	if err != nil {
		log.Printf("llm: SetConfig: invalid JSON, keeping current config: %v", err)
		return
	}
	a.c.SetConfig(cfg)
	log.Printf("llm: config updated via dashboard: active=%v", a.c.ActiveProviders())
}

// KeyStatus exposes per-key rotation state for the settings page.
func (a llmChainAdapter) KeyStatus(provider string) []web.KeyStatus {
	return toWebKeyStatus(a.c.KeyStatus(provider))
}

// ValidateKey tests one API key of a provider (settings "test" button).
func (a llmChainAdapter) ValidateKey(ctx context.Context, provider string, idx int) error {
	return a.c.ValidateKey(ctx, provider, idx)
}

// toWebKeyStatus converts the engines keyring statuses to the web
// package's render-ready type (masked, with badge/label fields). The web
// package never imports internal/engines, so the conversion lives here.
func toWebKeyStatus(in []tts.KeyStatus) []web.KeyStatus {
	out := make([]web.KeyStatus, 0, len(in))
	for _, ks := range in {
		w := web.KeyStatus{
			Index:                ks.Index,
			Last4:                ks.Last4,
			State:                ks.State,
			CooldownRemainingSec: ks.CooldownRemainingSec,
			RateLimitHits:        ks.RateLimitHits,
			Requests:             ks.Requests,
			LastRateLimitUnix:    ks.LastRateLimitUnix,
			Total:                len(in),
		}
		w.FillDerived()
		out = append(out, w)
	}
	return out
}

// getenvList reads a comma-separated env var into trimmed non-empty items.
func getenvList(name string) []string {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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

func getenvInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address for the web dashboard (default localhost-only; pass :8080 to expose on LAN)")
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
	// R2-W7: áp dụng bản khôi phục đang chờ TRƯỚC khi mở bất kỳ DB nào.
	if err := backup.ApplyStaged(*dataDir); err != nil {
		log.Fatalf("restore: %v", err)
	}
	dbPath := *dataDir + "/ledger.db"
	firstRun := !fileExists(dbPath)
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
	// GEMINI_API_KEYS (comma-separated) wins; legacy GEMINI_API_KEY is the
	// single-key fallback.
	geminiKeys := getenvList("GEMINI_API_KEYS")
	if len(geminiKeys) == 0 {
		geminiKeys = getenvList("GEMINI_API_KEY")
	}
	loadChain := func(key string, def engines.ChainConfig) (engines.ChainConfig, bool) {
		raw, ok, err := l.GetSetting(key)
		if err != nil {
			log.Printf("settings %s: read error, using default: %v", key, err)
			return def, false
		}
		if !ok || raw == "" {
			return def, false
		}
		cfg, err := tts.ParseChainJSON(json.RawMessage(raw))
		if err != nil || len(cfg.Order) == 0 {
			log.Printf("settings %s: invalid JSON, using default", key)
			return def, false
		}
		return cfg, true
	}
	cfgSrc := func() (engines.ChainConfig, engines.ChainConfig) {
		llmCfg, llmSaved := loadChain(ledger.SettingLLMChain, engines.DefaultLLMConfig(geminiKeys))
		ttsCfg, ttsSaved := loadChain(ledger.SettingTTSChain, engines.DefaultTTSConfig(geminiKeys))
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
	llmChain, ttsChain := engines.DefaultChains(*dataDir, geminiKeys, decider, cfgSrc)
	log.Printf("llm: active providers: %v", llmChain.ActiveProviders())
	log.Printf("tts: active providers: %v", ttsChain.ActiveProviders())
	if len(geminiKeys) == 0 {
		log.Printf("note: GEMINI_API_KEYS / GEMINI_API_KEY not set — gemini tiers will fail over to local/edge tiers")
	} else {
		log.Printf("gemini: %d API key(s) configured (rotation enabled)", len(geminiKeys))
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
				health["llm:"+name] = h
			}
		}
	}
	for _, name := range ttsChain.ActiveProviders() {
		if p, ok := ttsChain.Provider(name); ok {
			if h, ok := any(p).(web.HealthChecker); ok {
				health["tts:"+name] = h
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
	webCfg.Version = version
	srv, err := web.NewServer(webCfg, l, mgr, dbPath)
	if err != nil {
		log.Fatalf("web: %v", err)
	}
	// R2-W7: thư mục dữ liệu trống (chưa có ledger.db) → mở wizard /onboard.
	srv.Onboarding = firstRun
	// R2-W7: token OAuth nằm trong thư mục dữ liệu, không rải ở CWD.
	publishers.SetTokenDir(filepath.Join(*dataDir, "tokens"))
	publishers.MigrateTokensFromCWD()
	defer func() {
		if err := srv.Close(); err != nil {
			log.Printf("web close: %v", err)
		}
	}()
	srv.LLM = llmChainAdapter{c: llmChain} // satisfies web.LLMClient + chain-config interfaces
	srv.TTS = ttsChainAdapter{c: ttsChain}
	// R2-W4: the single configuration facade — ledger settings. All
	// automation switches, the master switch, growth thresholds and the
	// API budget live here.
	autoSettings := automation.LedgerSettings{L: l}
	// OutDir follows the -data flag (NewServer defaults to ./data/*
	// relative to the working directory).
	outDir := filepath.Join(*dataDir, "output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("web: out dir: %v", err)
	}
	srv.OutDir = outDir
	srv.Health = health
	if vieNeu != nil {
		srv.VieNeu = vieNeuAdapter{v: vieNeu}
	}
	// Đợt 3 (A7): enabled local tiers ensure their own models at startup;
	// progress shows in Settings + the homepage runtime block.
	go srv.EnsureLocalModels(ctx)

	// -- 6b. studio: AI video creation (affiliate) --------------------------------
	// The studio reuses the LLM chain (director), the TTS chain (narration)
	// and the Gemini keys (image generation). (Film pipeline parked —
	// PIVOT 2026-10-02.)
	studioMG := studio.NewGeminiMediaGen(geminiKeys)
	studioNarrator := studio.Narrator(func(ctx context.Context, text string) ([]byte, error) {
		return ttsChain.Synthesize(ctx, text, "default")
	})
	if st, err := studio.New(filepath.Join(*dataDir, "studio.db"), studioLLMAdapter{c: llmChain}, studioMG, studioNarrator, outDir); err != nil {
		log.Printf("studio: init failed: %v (studio page disabled)", err)
	} else {
		srv.Studio = st
		// Film Wave 1 (P0-3): jobs left "running" died with the previous
		// process — mark them failed so the UI offers "Chạy tiếp".
		if n := st.MarkInterruptedJobs(); n > 0 {
			log.Printf("studio: %d job(s) interrupted by restart — marked failed (resume from Studio)", n)
		}
		defer func() {
			if err := st.Close(); err != nil {
				log.Printf("studio close: %v", err)
			}
		}()
		log.Printf("studio: ready (mediagen=%s keys=%d)", studioMG.Name(), studioMG.KeyCount())
	}

	// -- 6c. affiliate autopilot: theme -> high-commission product -> video --
	// Ninh uploads model photos per account (Studio UI) and picks a theme;
	// the system hunts products and builds the videos hands-off.
	if srv.Studio != nil {
		pstore, err := products.NewStore(filepath.Join(*dataDir, "products.db"))
		if err != nil {
			log.Printf("products: init failed: %v (product discovery disabled)", err)
		} else {
			defer func() {
				if err := pstore.Close(); err != nil {
					log.Printf("products close: %v", err)
				}
			}()
			srv.Products = pstore
			if n, err := srv.MigrateLedgerShelf(); err != nil {
				log.Printf("products: migrate legacy ledger shelf: %v", err)
			} else if n > 0 {
				log.Printf("products: moved %d legacy ledger products into the shared store", n)
			}
			// R2-W4: single settings facade (ledger settings). One-time
			// copy of the legacy products.db control settings; after
			// that the facade is the only control plane.
			if err := automation.MigrateProductsSettings(pstore, autoSettings); err != nil {
				log.Printf("settings: migrate products settings: %v", err)
			}
			shopClient := &tiktok.ShopClient{
				AppKey:      os.Getenv("TIKTOK_SHOP_APP_KEY"),
				AppSecret:   os.Getenv("TIKTOK_SHOP_APP_SECRET"),
				AccessToken: os.Getenv("TIKTOK_SHOP_ACCESS_TOKEN"),
			}
			shopProvider := &products.TikTokShopProvider{Client: shopClient}
			srv.ProductProviders = []products.Provider{shopProvider}
			models, err := srv.Studio.ModelLibrary()
			if err != nil {
				log.Printf("autopilot: model library failed: %v", err)
			} else {
				ap := studio.NewAutopilot(srv.Studio, mgr, pstore,
					[]products.Provider{shopProvider}, models)
				// Music bed: the UI upload (data/autopilot-music.m4a); the legacy
				// data/trending-audio.m4a file is still honored as a fallback.
				if mp := filepath.Join(*dataDir, "autopilot-music.m4a"); fileExists(mp) {
					ap.MusicPath = mp
				} else if mp := filepath.Join(*dataDir, "trending-audio.m4a"); fileExists(mp) {
					ap.MusicPath = mp
				}
				srv.Autopilot = ap
				log.Printf("autopilot: ready (tiktok_shop configured=%v)",
					shopProvider.Configured())
				// After every finished studio job: auto-publish affiliate
				// videos to TikTok when the UI toggle is on. Fail-closed:
				// without a configured TikTok publisher nothing is posted.
				srv.Studio.SetOnDone(srv.AutomationService().AutoPublishHook())
				// One-time seed from env so existing deployments keep working;
				// after that the web UI (/products) is the only control plane.
				// Đợt 3: automation defaults ON when unset and a stored "0"
				// always wins, so the env seed only lands when AUTOPILOT_SCHEDULE
				// is explicitly present — a fresh install writes nothing, and an
				// existing stored choice is never overwritten. DRY-RUN stays
				// the safety gate (see the scheduler loop below).
				if _, ok := autoSettings.Get(web.SettingAutopilotEnabled); !ok {
					if _, present := os.LookupEnv("AUTOPILOT_SCHEDULE"); present {
						if getenvBool("AUTOPILOT_SCHEDULE", false) {
							_ = autoSettings.Set(web.SettingAutopilotEnabled, "1")
							_ = autoSettings.Set(web.SettingAutopilotInterval,
								strconv.Itoa(getenvInt("AUTOPILOT_INTERVAL_HOURS", 6)))
						} else {
							_ = autoSettings.Set(web.SettingAutopilotEnabled, "0")
						}
					}
				}
				// Auto-publish also defaults ON when unset (Đợt 3) — the old
				// forced "0" seed is gone; kill switch + DRY-RUN gate it.
				// Background scheduler, driven by UI-managed settings.
				// Checks every minute; runs when enabled and the interval elapsed.
				go func() {
					tick := time.NewTicker(time.Minute)
					defer tick.Stop()
					auto := srv.AutomationService()
					for {
						select {
						case <-ctx.Done():
							return
						case <-tick.C:
							for _, note := range auto.AutopilotTick(ctx) {
								log.Printf("%s", note)
							}
						}
					}
				}()
				log.Printf("autopilot: scheduler armed (managed in web UI /products)")
			}
		}
	}

	// -- 7. network daemon ---------------------------------------------------
	// R2-W4 (R2-08): MASTER_SWITCH is a persisted setting now, default
	// OFF. The env seeds it exactly once; afterwards the Settings UI is
	// the only control plane, and the daemon reads the gate live every
	// tick — no restart needed for the toggle to take effect.
	if err := automation.SeedMasterSwitch(autoSettings, getenvBool("MASTER_SWITCH", false)); err != nil {
		log.Printf("settings: seed master switch: %v", err)
	}
	netCfg := network.DefaultNetConfig()
	netCfg.MasterSwitch = automation.MasterOn(autoSettings)
	netCfg.MasterGate = automation.MasterSwitchGate{Settings: autoSettings}
	netCfg.DryRun = getenvBool("DRY_RUN", true)
	netCfg.KillSwitch = getenvBool("KILL_SWITCH", false)
	// ONE source of truth for kill/dry-run: the daemon reads the very
	// *Config the dashboard toggles (webCfg) live on every tick, so the
	// UI Kill switch stops the daemon without a restart.
	netCfg.Gate = webCfg
	log.Printf("network: master=%v dry_run=%v kill_switch=%v", netCfg.MasterSwitch, netCfg.DryRun, netCfg.KillSwitch)
	daemon := network.NewDaemon(l, mgr, llmChain, netCfg) // *engines.LLMChain satisfies network.LLMClient
	go daemon.Run(ctx, 60*time.Second)

	// -- 7b. growth automation: plan -> production -> YouTube publish ------
	// Zero-touch channel growth (docs/CHANNEL_GROWTH.md): due plan items
	// render in Studio and upload to YouTube inside the daily quota. The
	// /growth toggle defaults ON (a stored "0" opts out); dry-run and the
	// kill switch still gate every real action. TikTok stays draft-only
	// pre-audit.
	if srv.Growth != nil {
		go func() {
			tick := time.NewTicker(5 * time.Minute)
			defer tick.Stop()
			auto := srv.AutomationService()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					for _, note := range auto.GrowthTick(ctx, false) {
						log.Printf("growth: %s", note)
					}
				}
			}
		}()
		log.Printf("growth: automation tick armed (mặc định bật ở /growth; dry-run là cổng an toàn)")
	}

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

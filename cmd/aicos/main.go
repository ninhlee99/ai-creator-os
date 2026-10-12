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
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
	"github.com/ninhlee99/ai-creator-os/internal/engines"
	"github.com/ninhlee99/ai-creator-os/internal/engines/local"
	"github.com/ninhlee99/ai-creator-os/internal/engines/tts"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/lockfile"
	"github.com/ninhlee99/ai-creator-os/internal/logx"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
	"github.com/ninhlee99/ai-creator-os/internal/runtimeguard"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/ticksup"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

// version mặc định khi build thường; build phát hành truyền
// -ldflags "-X main.version=vX.Y.Z" để đóng dấu thật.
var version = "dev"

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

// ---------------------------------------------------------------------------
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
	// Đợt S: single-instance — flock giữ suốt thời gian process sống; OS tự
	// giải phóng khi crash nên không bao giờ kẹt khóa "ma". Đã có instance
	// khác (vd: launchd) đang chạy → thoát êm, không giành SQLite.
	lk, err := lockfile.Acquire(filepath.Join(*dataDir, "aicos.lock"))
	if err != nil {
		log.Fatalf("lock: %v", err)
	}
	if lk == nil {
		log.Printf("aicos: đã có instance khác đang chạy trên %s — thoát (không khởi động đúp)", *dataDir)
		os.Exit(0)
	}
	defer lk.Close()
	// Đợt R: app tự xoay log theo dung lượng (20MB, giữ 7 bản). launchd mở
	// StandardOutPath một lần và giữ fd nên newsyslog KHÔNG xoay được log
	// của nó — xem deploy/README.md.
	if rot, rerr := logx.New(filepath.Join(*dataDir, "logs", "aicos.log"), logx.DefaultMaxSize, logx.DefaultMaxBackup); rerr != nil {
		log.Printf("log: không mở được file log xoay (%v) — ghi tiếp ra stderr", rerr)
	} else {
		defer rot.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, rot))
		log.Printf("log: ghi vào %s (tự xoay 20MB, giữ 7 bản)", rot.Path())
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

	// Đợt X: quên mật khẩu dashboard — chủ máy chạy lại app với
	// AICOS_RESET_AUTH=1 để xóa mật khẩu (Windows thường không có sqlite3
	// CLI để xóa tay). Chỉ người start process mới set được env này nên an toàn.
	if os.Getenv("AICOS_RESET_AUTH") == "1" {
		if err := l.SetSetting(web.KeyAuthPasswordHash, ""); err != nil {
			log.Printf("auth reset: %v", err)
		} else {
			log.Printf("auth reset: đã xóa mật khẩu dashboard theo AICOS_RESET_AUTH=1 — đặt mật khẩu mới ở Cài đặt · Hệ thống")
		}
	}

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

	guard := runtimeguard.New()

	// -- 5. sidecars --------------------------------------------------------
	reg := engines.DefaultRegistry(*dataDir)

	const llamaModel = "qwen2.5-7b-instruct-q4_k_m.gguf"
	// llama-server runs on 127.0.0.1:8081 (see engines.NewLlamaServerProcess)
	// so it never collides with the dashboard's default -addr :8080.
	llamaProc := engines.NewLlamaServerProcess(reg)
	if reg.Has(llamaModel) {
		guard.Go(ctx, "sidecar:llama-server", func(c context.Context) {
			log.Printf("sidecar: llama-server starting (model %s)", llamaModel)
			if err := llamaProc.Start(c); err != nil {
				log.Printf("sidecar: llama-server failed to start: %v", err)
			} else {
				log.Printf("sidecar: llama-server running")
			}
		})
	} else {
		log.Printf("sidecar: llama-server not started — model %s chưa được tải (dashboard settings để tải)", llamaModel)
	}

	// VieNeu TTS sidecar: controlled through the provider inside the chain.
	var vieNeu *tts.VieNeuProvider
	if p, ok := ttsChain.Provider("vieneu"); ok {
		if vn, ok := p.(*tts.VieNeuProvider); ok {
			vieNeu = vn
			if reg.VieNeuPresent() {
				guard.Go(ctx, "sidecar:vieneu", func(c context.Context) {
					log.Printf("sidecar: vieneu starting (first run can take a while)")
					if err := vieNeu.Start(c); err != nil {
						log.Printf("sidecar: vieneu failed to start: %v", err)
					} else {
						log.Printf("sidecar: vieneu running")
					}
				})
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
	// Đợt R: một supervisor duy nhất giám sát mọi automation tick (timeout +
	// recover + heartbeat). Service build lazy để luôn thấy đủ các store đã
	// wiring xong (Studio/AT/Reup được gán sau điểm này).
	tickSup := ticksup.New(
		func() ticksup.Alerter { return srv.AutomationService() },
		tts.GeminiQuotaCircuit().TakeAlert,
	)
	// Đợt R+S: /healthz đọc heartbeat tick + trạng thái ngắt mạch quota.
	srv.TickBeats = func() map[string]web.TickBeat {
		out := make(map[string]web.TickBeat)
		for k, v := range tickSup.Snapshot() {
			out[k] = web.TickBeat{
				LastStart: v.LastStart,
				LastOK:    v.LastOK,
				LastErr:   v.LastErr,
				Runs:      v.Runs,
			}
		}
		return out
	}
	srv.QuotaCircuit = func() (bool, string) {
		qc := tts.GeminiQuotaCircuit()
		if !qc.Open() {
			return false, ""
		}
		return true, qc.OpenUntil().Format("02/01 15:04")
	}
	if vieNeu != nil {
		srv.VieNeu = vieNeuAdapter{v: vieNeu}
	}
	// Đợt AK: /healthz báo tick kẹt (needs_restart) + sức khỏe sidecar để
	// watchdog ngoài tự khởi động lại app thay vì chỉ ghi log.
	srv.RemoteNeedsPassword = true
	srv.StaleTicks = func() []string { return tickSup.Stale(time.Now()) }
	srv.Sidecars = func() map[string]web.SidecarBeat {
		out := map[string]web.SidecarBeat{}
		add := func(name string, p *local.Process) {
			if p == nil {
				return
			}
			herr, at, fails := p.HealthStatus()
			b := web.SidecarBeat{State: p.State(), Restarts: p.RestartCount(), HealthFails: fails, CheckedAt: at}
			if herr != nil {
				b.HealthErr = herr.Error()
			}
			out[name] = b
		}
		if reg.Has(llamaModel) {
			add("llama-server", llamaProc)
		}
		if vieNeu != nil {
			add("vieneu", vieNeu.Process())
		}
		return out
	}
	// Đợt 3 (A7): enabled local tiers ensure their own models at startup;
	// progress shows in Settings + the homepage runtime block.
	guard.Go(ctx, "local-models:ensure", func(c context.Context) {
		srv.EnsureLocalModels(c)
	})

	// -- 6b. studio: AI video creation (affiliate) --------------------------------
	// The studio reuses the LLM chain (director), the TTS chain (narration)
	// and the Gemini keys (image generation). (Film pipeline parked —
	// PIVOT 2026-10-02.)
	studioMG := studio.NewGeminiMediaGen(geminiKeys)
	studioNarrator := studio.Narrator(func(ctx context.Context, text, voice string) ([]byte, error) {
		if voice == "" {
			voice = "default"
		}
		return ttsChain.Synthesize(ctx, text, voice)
	})
	if st, err := studio.New(filepath.Join(*dataDir, "studio.db"), studioLLMAdapter{c: llmChain}, studioMG, studioNarrator, outDir); err != nil {
		log.Printf("studio: init failed: %v (studio page disabled)", err)
	} else {
		srv.Studio = st
		// Đợt V: pre-flight đĩa — từ chối job render mới khi đĩa gần đầy.
		st.DiskGuard = func() (int, error) { return automation.DiskUsagePct(*dataDir) }
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

	// -- 6b. Accesstrade (Đợt B): cache chiến dịch + link đã tạo.
	// Fail-soft: mở DB lỗi thì srv.AT = nil, UI fail-closed trung thực.
	if atStore, err := accesstrade.NewStore(filepath.Join(*dataDir, "accesstrade.db")); err != nil {
		log.Printf("accesstrade: init failed: %v (Accesstrade disabled)", err)
	} else {
		defer func() {
			if err := atStore.Close(); err != nil {
				log.Printf("accesstrade close: %v", err)
			}
		}()
		srv.AT = atStore
		log.Printf("accesstrade: ready")
	}

	// -- 6c. Reup Douyin (Đợt D): nguồn + video viral tải về.
	// Fail-soft: mở DB lỗi thì srv.Reup = nil, UI fail-closed trung thực.
	if reupStore, err := reup.NewStore(filepath.Join(*dataDir, "reup.db")); err != nil {
		log.Printf("reup: init failed: %v (Reup disabled)", err)
	} else {
		defer func() {
			if err := reupStore.Close(); err != nil {
				log.Printf("reup close: %v", err)
			}
		}()
		srv.Reup = reupStore
		log.Printf("reup: ready")
	}

	// Đợt R: cờ arm vòng tick — khởi động tập trung ở cuối main qua ticksup.ArmLoops.
	autopilotArmed := false

	// -- 6d. affiliate autopilot: theme -> high-commission product -> video --
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
			// Đợt H1: TikTok Shop provider đã loại bỏ (Accesstrade-only).
			// Nguồn sản phẩm duy nhất là Accesstrade datafeed → products.Store
			// (hunter tick). Autopilot đọc store trước; providers để trống.
			srv.ProductProviders = nil
			models, err := srv.Studio.ModelLibrary()
			if err != nil {
				log.Printf("autopilot: model library failed: %v", err)
			} else {
				ap := studio.NewAutopilot(srv.Studio, mgr, pstore,
					nil, models)
				// Music bed: the UI upload (data/autopilot-music.m4a); the legacy
				// data/trending-audio.m4a file is still honored as a fallback.
				if mp := filepath.Join(*dataDir, "autopilot-music.m4a"); fileExists(mp) {
					ap.MusicPath = mp
				} else if mp := filepath.Join(*dataDir, "trending-audio.m4a"); fileExists(mp) {
					ap.MusicPath = mp
				}
				srv.Autopilot = ap
				log.Printf("autopilot: ready (nguồn sản phẩm: Accesstrade store)")
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
				// Vòng tick khởi động tập trung ở cuối main (ticksup.ArmLoops).
				autopilotArmed = true
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
	// Đợt R: recover để daemon panic không giết cả process.
	// Đợt AI 24/7: vòng lặp tự hồi phục — panic 1 lần không được giết daemon
	// vĩnh viễn (trước đây goroutine chết luôn, network ngừng im lặng).
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
			// Run thoát bất thường (không phải do ctx) → chờ 5s rồi chạy lại.
			select {
			case <-c.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	})

	// -- 7b→7e. automation ticks (Đợt R) -------------------------------------
	// Mọi vòng tick khởi động tập trung qua ticksup.ArmLoops: cadence,
	// timeout, recover, heartbeat nằm gọn trong internal/ticksup (loops.go),
	// main chỉ quyết định tick nào được arm theo store đã wiring xong.
	ticksup.ArmLoops(ctx, tickSup, ticksup.Deps{
		NewService: srv.AutomationService,
		Autopilot:  autopilotArmed,
		Growth:     srv.Growth != nil,
		AT:         srv.AT != nil,
		Reup:       srv.Reup != nil,
		Story:      srv.Studio != nil,
	})

	// -- 8. http server + graceful shutdown ----------------------------------
	// Đợt S: timeout chống slowloris/treo kết nối trên máy 24/7.
	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
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

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/ninhlee99/ai-creator-os/internal/backup"
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/lockfile"
	"github.com/ninhlee99/ai-creator-os/internal/logx"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/web"
)

// version mặc định khi build thường; build phát hành truyền
// -ldflags "-X main.version=vX.Y.Z" để đóng dấu thật.
var version = "dev"

// ---------------------------------------------------------------------------
func main() {
	loadDotEnv(".env")
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
	// Kiểm tra toàn vẹn DB lúc boot; báo rõ cách khôi phục nếu DB hỏng.
	if err := checkDBIntegrity(*dataDir); err != nil {
		log.Fatalf("DB hỏng: %v", err)
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

	// Nạp cache các model Gemini mới nhất đã tự dò được ở lần chạy trước (nếu có).
	if err := gmodels.LoadPersisted(*dataDir); err != nil {
		log.Printf("gmodels: nạp cache model thất bại: %v", err)
	}

	// -- 2. decision logger ------------------------------------------------
	decider := decideAdapter{l: l}

	// -- 3. chain configs from the settings table --------------------------
	geminiKeys := getenvList("GEMINI_API_KEYS")
	if len(geminiKeys) == 0 {
		geminiKeys = getenvList("GEMINI_API_KEY")
	}
	// -- 4. provider chains -------------------------------------------------
	llmChain, ttsChain := loadEngineChains(l, geminiKeys, decider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var studioInst *studio.Studio

	// -- 6. accounts + web server -------------------------------------------
	webRuntime, err := initializeWeb(ctx, l, *dataDir, dbPath, firstRun, llmChain, ttsChain)
	if err != nil {
		log.Fatal(err)
	}
	mgr := webRuntime.manager
	srv := webRuntime.server
	autoSettings := webRuntime.settings
	tickSup := webRuntime.tickSup
	outDir := webRuntime.outDir
	defer func() {
		if err := mgr.Close(); err != nil {
			log.Printf("accounts close: %v", err)
		}
	}()
	defer func() {
		if err := srv.Close(); err != nil {
			log.Printf("web close: %v", err)
		}
	}()
	// -- 6b. studio: AI video creation (affiliate) --------------------------------
	// The studio reuses the LLM chain (director), the TTS chain (narration)
	// and Gemini keys for text/image analysis. Prompt-only image generation
	// uses Pollinations Free API Classic.
	studioInst = initializeStudio(srv, *dataDir, outDir, geminiKeys, llmChain, ttsChain)
	if studioInst != nil {
		defer func() {
			if err := studioInst.Close(); err != nil {
				log.Printf("studio close: %v", err)
			}
		}()
	}

	atStore := initializeAccesstrade(srv, *dataDir)
	if atStore != nil {
		defer func() {
			if err := atStore.Close(); err != nil {
				log.Printf("accesstrade close: %v", err)
			}
		}()
	}

	// Ticks arm after every store and service has been wired.
	pstore, autopilotArmed := initializeProducts(srv, mgr, autoSettings, *dataDir)
	if pstore != nil {
		defer func() {
			if err := pstore.Close(); err != nil {
				log.Printf("products close: %v", err)
			}
		}()
	}

	webAIStore := initializeWebAI(srv, *dataDir)
	if webAIStore != nil {
		defer func() {
			if err := webAIStore.Close(); err != nil {
				log.Printf("webai close: %v", err)
			}
		}()
	}

	startWorkersAndServe(ctx, cancel, *addr, srv, autoSettings, tickSup, autopilotArmed, studioInst)
}

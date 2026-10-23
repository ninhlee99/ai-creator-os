package web

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/harness"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"html/template"
	"log"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Server is the web dashboard. Exported fields are the wiring surface:
// the parent worker injects the ledger, account manager, config, engines
// and sidecar controls here.
type Server struct {
	Ledger *ledger.Ledger
	Mgr    *network.AccountManager
	Cfg    *Config
	LLM    LLMClient
	TTS    TTSChainAPI
	Studio *studio.Studio
	// --- Parked-build support (PIVOT 2026-10-02) ---
	// Avatar / AvatarSidecar / AvatarDir chỉ được dùng khi build với
	// -tags parked (handler avatar đã park). Giữ field ở bản thường để
	// build parked biên dịch được; bản thường không nối gì vào chúng.
	Avatar    AvatarChainAPI
	AvatarDir string
	// Growth is the channel-growth engine store (nil when its tables could
	// not be created; the /growth page then shows an honest error).
	Growth *growth.Store
	// GrowthProducer / GrowthYT are the production backends of the growth
	// automation tick (plan item -> Studio job -> YouTube upload). Nil
	// means the real Studio/publishers adapters; tests inject fakes.
	// CommissionSource is the affiliate-orders feed for money
	// reconciliation (R2-W1). Nil = fail-closed legacy source (TikTok Shop
	// đã loại bỏ ở Đợt H1); tests inject fakes.
	CommissionSource automation.OrdersSource
	GrowthProducer   automation.Producer
	GrowthYT         automation.Uploader
	// Autopilot runs hands-off affiliate cycles (products -> video).
	// Products is the affiliate product store. ProductProviders are the
	// configured product search providers. All three are injected by the
	// cmd wiring; handlers degrade gracefully when they are nil.
	Autopilot        *studio.Autopilot
	Products         *products.Store
	ProductProviders []products.Provider
	// AT is the Accesstrade cache store (campaigns + created links).
	// Injected by cmd wiring; nil when its DB could not be opened —
	// handlers then fail closed with an honest message.
	AT *accesstrade.Store
	// OnGeminiKeys nhận bộ key Gemini dùng chung mỗi khi đổi (main nối vào
	// phần tạo ảnh của Studio).
	OnGeminiKeys func(keys []string)
	Health       map[string]HealthChecker
	// AvatarSidecar: parked-build support (như Avatar ở trên).
	AvatarSidecar AvatarSidecarCtl

	// WebAI quản lý kết nối và phiên đăng nhập Claude, ChatGPT, Gemini, Muse AI.
	WebAI      *webai.Store
	WebAIRelay *webai.ProxyRelay

	// Harness là Master Autonomous Agent điều phối nhận thức OODA và tự học.
	Harness *harness.Harness

	// OutDir holds rendered videos (served at /media/).
	OutDir string
	// DataDir is the app data directory (sqlite DBs, jobs, tokens). The
	// single place state lives; derived from the ledger path (R2-W7).
	DataDir string
	// Onboarding puts the server in first-run wizard mode: every route
	// except /onboard redirects to the wizard until it completes.
	Onboarding bool

	// StartedAt là thời điểm NewServer chạy xong — cho /healthz tính uptime.
	StartedAt time.Time
	// TickBeats trả về heartbeat các automation tick (do supervisor ở
	// cmd/aicos cập nhật). Nil = chưa wiring — /healthz trả ticks rỗng.
	TickBeats func() map[string]TickBeat
	// QuotaCircuit trả về (đang mở, mở tới khi nào) của ngắt mạch quota
	// ngày. Nil = không dùng.
	QuotaCircuit func() (open bool, until string)
	// RemoteNeedsPassword: chưa đặt mật khẩu thì chặn mọi truy cập không
	// đến từ chính máy chủ (Đợt AK). main bật; test mặc định tắt.
	RemoteNeedsPassword bool
	// TrustCloudflareIP enables CF-Connecting-IP only for loopback requests.
	// Set only when Cloudflare Tunnel is the configured local proxy.
	TrustCloudflareIP bool
	// StaleTicks trả về tên các tick quá hạn chưa chạy (kẹt) — Đợt AK:
	// /healthz báo needs_restart để watchdog ngoài tự khởi động lại app.
	StaleTicks func() []string

	// authMu/authSessions: session đăng nhập dashboard (Đợt T). Token →
	// thời điểm hết hạn; restart là đăng nhập lại.
	authMu                sync.Mutex
	authSessions          map[string]time.Time
	bioTryOnKeyMu         sync.Mutex
	bioGoogleSubjectKeyMu sync.Mutex
	bioAnalyticsMu        sync.Mutex
	bioGoogleSubjectKey   []byte
	bioAnalyticsHMACKey   []byte

	db                *sql.DB // read handle for decision/analytics queries
	oauth             oauthStates
	facebookPendingMu sync.Mutex
	facebookPending   map[string]facebookPendingAccount
	templates         map[string]*template.Template

	// Avatar model-download progress — parked-build support (như trên).
	avatarDlMu         sync.Mutex
	avatarDlDownloaded int64
	avatarDlTotal      int64
	avatarDlDone       bool
	avatarDlErr        string
}

// NewServer wires a dashboard against an existing ledger + account manager.
// dbPath is the SQLite file (a second read handle is opened for the
// decision/analytics queries the ledger API does not expose).
func NewServer(cfg *Config, l *ledger.Ledger, mgr *network.AccountManager, dbPath string) (*Server, error) {
	if cfg == nil {
		cfg = LoadConfig()
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open web read db: %w", err)
	}
	// Đợt S: 1 connection duy nhất — PRAGMA busy_timeout bên dưới chỉ có
	// tác dụng khi mọi query đi qua cùng connection (khớp các store khác:
	// ledger/studio). Trước đây pool nhiều connection → dashboard thỉnh
	// thoảng SQLITE_BUSY đúng lúc tick đang ghi.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	s := &Server{
		Ledger:            l,
		Mgr:               mgr,
		Cfg:               cfg,
		Health:            map[string]HealthChecker{},
		OutDir:            "data/output",
		AvatarDir:         "data/avatars", // parked-build support (handler avatar)
		DataDir:           filepath.Dir(dbPath),
		StartedAt:         time.Now(), // Đợt S: /healthz
		TrustCloudflareIP: getenvBool("AICOS_TRUST_CLOUDFLARE_IP", false),
		db:                db,
	}

	s.applyPersistedEnv()
	s.applyPersistedDryRun()
	if gs, err := growth.NewStore(db); err != nil {
		log.Printf("web: growth store: %v", err)
	} else {
		s.Growth = gs
	}
	if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.parseTemplates(); err != nil {
		db.Close()
		return nil, err
	}
	s.WireStudioWebAI()
	return s, nil
}

// WireStudioWebAI kết nối kho tài khoản Web AI (Google Flow, Muse AI) với Studio AI video engine.
func (s *Server) WireStudioWebAI() {
	if s.Studio == nil || s.WebAI == nil {
		return
	}
	s.Studio.SetWebAIResolver(func(platform string) (*studio.WebAISession, error) {
		acc, err := s.WebAI.PickActiveAccount(platform)
		if err != nil {
			return nil, err
		}
		return &studio.WebAISession{
			AccountID:    acc.ID,
			Platform:     acc.Platform,
			SessionToken: acc.SessionKey,
			CookieHeader: acc.Cookie,
			Email:        acc.Email,
		}, nil
	})
	s.Studio.SetWebAIVideoRenderer(func(ctx context.Context, session *studio.WebAISession, request studio.WebAIVideoRequest, outputPath string, progress func(string)) (string, error) {
		if session.Platform != webai.PlatformGoogleFlow {
			return "", fmt.Errorf("reference video generation only supports Google Flow, got %q", session.Platform)
		}
		current := session
		attempted := map[string]struct{}{}
		if id := strings.TrimSpace(current.AccountID); id != "" {
			attempted[id] = struct{}{}
		}
		for {
			var result webai.FlowVideoResult
			var err error
			switch request.Mode {
			case "ingredients":
				result, err = webai.GoogleFlowVideoIngredientsHeadless(ctx, current.CookieHeader, request.Prompt, request.AspectRatio, request.ImagePaths, request.ProjectID, progress)
			case "video_edit":
				result, err = webai.GoogleFlowVideoEditHeadless(ctx, current.CookieHeader, request.Prompt, request.AspectRatio, request.VideoPath, request.ImagePaths, request.ProjectID, progress)
			default:
				return "", fmt.Errorf("unsupported Google Flow reference mode %q", request.Mode)
			}
			if err != nil {
				authRejected := webai.IsWebSessionAuthError(err)
				if authRejected && strings.TrimSpace(current.Email) != "" {
					_ = s.WebAI.MarkSessionExpired(webai.PlatformGoogleFlow, current.Email)
				} else if result.CookieHeader != "" {
					if saveErr := s.persistWebAICookieRefresh(ctx, webai.PlatformGoogleFlow, current.Email, result.CookieHeader, true); saveErr != nil {
						progress("FLOW_COOKIE_REFRESH: chưa xác minh/lưu được cookie mới")
					}
				}
				if cooldown, limited := webai.FlowPreSubmitRateLimitCooldown(err); limited {
					if current.AccountID == "" {
						return "", fmt.Errorf("Google Flow báo hết quota nhưng không xác định được account để lưu cooldown")
					}
					if markErr := s.WebAI.MarkRateLimited(current.AccountID, cooldown); markErr != nil {
						return "", fmt.Errorf("Google Flow báo hết quota; không lưu được cooldown account: %w", markErr)
					}
					next, pickErr := s.WebAI.PickActiveAccountExcluding(webai.PlatformGoogleFlow, attempted)
					if pickErr != nil {
						return "", fmt.Errorf("Google Flow hết quota; dừng vì không còn tài khoản miễn phí khả dụng: %w", pickErr)
					}
					attempted[next.ID] = struct{}{}
					current = &studio.WebAISession{
						AccountID: next.ID, Platform: next.Platform, SessionToken: next.SessionKey,
						CookieHeader: next.Cookie, Email: next.Email,
					}
					request.ProjectID = ""
					progress("FLOW_ACCOUNT_ROTATION: account miễn phí trước hết quota; chuyển sang account miễn phí tiếp theo")
					continue
				}
				return "", err
			}
			if result.CookieHeader != "" && strings.TrimSpace(current.Email) != "" {
				if err := s.persistWebAICookieRefresh(ctx, webai.PlatformGoogleFlow, current.Email, result.CookieHeader, false); err != nil {
					progress("FLOW_COOKIE_REFRESH: chưa lưu được cookie mới; job video vẫn tiếp tục")
				} else {
					progress("FLOW_COOKIE_REFRESH: đã cập nhật cookie account đã dùng")
				}
			}
			if err := webai.DownloadFlowVideo(ctx, result.MediaURL, outputPath); err != nil {
				return "", err
			}
			return result.ProjectID, nil
		}
	}, func(platform string) (bool, string) {
		accounts, err := s.WebAI.GetAccounts(platform)
		if err != nil {
			return false, "không đọc được danh sách account: " + err.Error()
		}
		for _, account := range accounts {
			if account.IsActive && account.IsConfirmedFree() && strings.TrimSpace(account.Cookie) != "" && (account.Status == "ok" || account.Status == "connected") {
				if email := strings.TrimSpace(account.Email); email != "" {
					return true, "Flow · " + email
				}
				return true, "Flow · đã kết nối"
			}
		}
		return false, "Chưa kết nối"
	})
}

// Close releases the dashboard's read handle (ledger/manager are owned by
// the caller).
func (s *Server) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ------------------------------------------------------------------ time

func (s *Server) location() *time.Location {
	if loc, err := time.LoadLocation(s.Cfg.Timezone); err == nil {
		return loc
	}
	return time.Local
}

func (s *Server) today() string {
	return time.Now().In(s.location()).Format("2006-01-02")
}

func (s *Server) nowISO() string {
	return time.Now().In(s.location()).Format("2006-01-02T15:04:05")
}

// -------------------------------------------------------------- templates

// DB returns the underlying sqlite DB handle.
func (s *Server) DB() *sql.DB {
	return s.db
}

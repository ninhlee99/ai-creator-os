package web

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
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
	// Growth is the channel-growth engine store (nil when its tables could
	// not be created; the /growth page then shows an honest error).
	Growth *growth.Store
	// GrowthProducer is the production backend of the TikTok growth plan.
	// CommissionSource is the affiliate-orders feed for money
	// reconciliation (R2-W1). Nil = fail-closed legacy source (TikTok Shop
	// đã loại bỏ ở Đợt H1); tests inject fakes.
	CommissionSource automation.OrdersSource
	GrowthProducer   automation.Producer
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
	// WebAI quản lý account và phiên AI Web đã được khôi phục từ backup.
	WebAI *webai.Store

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
	// StaleTicks trả về tên các tick quá hạn chưa chạy (kẹt) — Đợt AK:
	// /healthz báo needs_restart để watchdog ngoài tự khởi động lại app.
	StaleTicks func() []string

	// authMu/authSessions: session đăng nhập dashboard (Đợt T). Token →
	// thời điểm hết hạn; restart là đăng nhập lại.
	authMu       sync.Mutex
	authSessions map[string]time.Time

	db        *sql.DB // read handle for decision/analytics queries
	oauth     oauthStates
	templates map[string]*template.Template
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
		Ledger:    l,
		Mgr:       mgr,
		Cfg:       cfg,
		Health:    map[string]HealthChecker{},
		OutDir:    "data/output",
		DataDir:   filepath.Dir(dbPath),
		StartedAt: time.Now(), // Đợt S: /healthz
		db:        db,
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
			CookieJar:    append([]webai.BrowserCookie(nil), acc.CookieJar...),
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
			refreshCookie := func(cookie string, cookies []webai.BrowserCookie) error {
				current.CookieHeader = cookie
				current.CookieJar = append([]webai.BrowserCookie(nil), cookies...)
				if len(cookies) > 0 {
					return s.persistWebAICookieSnapshot(current.AccountID, cookie, cookies)
				}
				return s.persistWebAICookieRefresh(current.AccountID, cookie)
			}
			switch request.Mode {
			case "ingredients":
				result, err = webai.GoogleFlowVideoIngredientsHeadlessWithCookieSnapshot(ctx, current.CookieHeader, current.CookieJar, request.Prompt, request.AspectRatio, request.ImagePaths, request.ProjectID, progress, refreshCookie)
			case "video_edit":
				result, err = webai.GoogleFlowVideoEditHeadlessWithCookieSnapshot(ctx, current.CookieHeader, current.CookieJar, request.Prompt, request.AspectRatio, request.VideoPath, request.ImagePaths, request.ProjectID, progress, refreshCookie)
			default:
				return "", fmt.Errorf("unsupported Google Flow reference mode %q", request.Mode)
			}
			if err != nil {
				authRejected := webai.IsWebSessionAuthError(err)
				if authRejected && strings.TrimSpace(current.Email) != "" {
					_ = s.WebAI.MarkSessionExpired(webai.PlatformGoogleFlow, current.Email)
				} else if result.CookieHeader != "" || len(result.CookieJar) > 0 {
					if saveErr := s.persistWebAIResultCookies(current.AccountID, result.CookieHeader, result.CookieJar); saveErr != nil {
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
						return "", fmt.Errorf("Google Flow hết quota; dừng vì không còn tài khoản khả dụng: %w", pickErr)
					}
					attempted[next.ID] = struct{}{}
					current = &studio.WebAISession{
						AccountID: next.ID, Platform: next.Platform, SessionToken: next.SessionKey,
						CookieHeader: next.Cookie, CookieJar: append([]webai.BrowserCookie(nil), next.CookieJar...), Email: next.Email,
					}
					request.ProjectID = ""
					progress("FLOW_ACCOUNT_ROTATION: account trước hết quota; chuyển sang account tiếp theo")
					continue
				}
				return "", err
			}
			if (result.CookieHeader != "" || len(result.CookieJar) > 0) && strings.TrimSpace(current.Email) != "" {
				if err := s.persistWebAIResultCookies(current.AccountID, result.CookieHeader, result.CookieJar); err != nil {
					progress("FLOW_COOKIE_REFRESH: chưa lưu được cookie mới; job video vẫn tiếp tục")
				} else {
					progress("FLOW_COOKIE_REFRESH: đã cập nhật cookie account đã dùng")
				}
			}
			if err := webai.DownloadFlowVideoWithCookieSnapshot(ctx, current.CookieHeader, current.CookieJar, result.MediaURL, outputPath, func(cookie string, cookies []webai.BrowserCookie) error {
				current.CookieHeader = cookie
				current.CookieJar = append([]webai.BrowserCookie(nil), cookies...)
				return s.persistWebAIResultCookies(current.AccountID, cookie, cookies)
			}); err != nil {
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
			if account.IsActive && (strings.TrimSpace(account.Cookie) != "" || len(account.CookieJar) > 0) && (account.Status == "ok" || account.Status == "connected") {
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

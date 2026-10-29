package web

import (
	"context"
	"database/sql"
	"fmt"
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
	"net/url"
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
	// OnGeminiKeys nhận bộ key Gemini dùng chung mỗi khi đổi (main nối vào
	// phần tạo ảnh của Studio).
	OnGeminiKeys func(keys []string)
	Health       map[string]HealthChecker
	// WebAI quản lý account và phiên AI Web. Store không được đưa vào backup
	// ZIP plaintext vì chứa cookie và bearer session.
	WebAI *webai.Store

	// OutDir holds rendered videos (served at /media/).
	OutDir string
	// DataDir is the app data directory (sqlite DBs, jobs, credentials). The
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
	authMu        sync.Mutex
	authSessions  map[string]time.Time
	loginAttempts map[string]loginAttemptState

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
	// Đợt S: 1 connection duy nhất — PRAGMA journal_mode=WAL và busy_timeout
	// bên dưới đảm bảo không bị SQLITE_BUSY khi Growth và Ledger cùng ghi.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
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

// WireStudioWebAI connects saved web accounts to Studio's browser-backed provider operations.
func (s *Server) studioWebAISession(account webai.Account) *studio.WebAISession {
	accountID := account.ID
	return &studio.WebAISession{
		AccountID: account.ID, Platform: account.Platform, AccountName: account.AccountName,
		SessionToken: account.SessionKey, CookieHeader: account.Cookie,
		CookieJar: append([]webai.BrowserCookie(nil), account.CookieJar...), Email: account.Email,
		CookieUpdater: func(cookie string, cookies []webai.BrowserCookie) error {
			return s.persistWebAIResultCookies(accountID, cookie, cookies)
		},
	}
}

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
			AccountName:  acc.AccountName,
			SessionToken: acc.SessionKey,
			CookieHeader: acc.Cookie,
			CookieJar:    append([]webai.BrowserCookie(nil), acc.CookieJar...),
			Email:        acc.Email,
			CookieUpdater: func(cookie string, cookies []webai.BrowserCookie) error {
				return s.persistWebAIResultCookies(acc.ID, cookie, cookies)
			},
		}, nil
	})
	s.Studio.SetWebAIAccountResolver(func(platform, email string) (*studio.WebAISession, error) {
		accounts, err := s.WebAI.GetAccounts(platform)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if !strings.EqualFold(strings.TrimSpace(account.Email), strings.TrimSpace(email)) {
				continue
			}
			if !account.IsActive || (account.Status != "ok" && account.Status != "connected") || strings.TrimSpace(account.Cookie) == "" && len(account.CookieJar) == 0 {
				return nil, fmt.Errorf("Muse account %s is inactive or has no usable session", email)
			}
			return &studio.WebAISession{
				AccountID: account.ID, Platform: account.Platform, AccountName: account.AccountName,
				SessionToken: account.SessionKey, CookieHeader: account.Cookie,
				CookieJar: append([]webai.BrowserCookie(nil), account.CookieJar...), Email: account.Email,
				CookieUpdater: func(cookie string, cookies []webai.BrowserCookie) error {
					return s.persistWebAIResultCookies(account.ID, cookie, cookies)
				},
			}, nil
		}
		return nil, fmt.Errorf("Muse account %s is not connected", email)
	})
	s.Studio.SetWebAIAccountIDResolver(func(platform, accountID string) (*studio.WebAISession, error) {
		accounts, err := s.WebAI.GetAccounts(platform)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if account.ID != accountID {
				continue
			}
			if !account.IsActive || (account.Status != "ok" && account.Status != "connected") || strings.TrimSpace(account.Cookie) == "" && len(account.CookieJar) == 0 {
				return nil, fmt.Errorf("Web AI account %s is inactive or has no usable session", accountID)
			}
			return s.studioWebAISession(account), nil
		}
		return nil, fmt.Errorf("Web AI account %s is not connected to %s", accountID, platform)
	})
	s.Studio.SetWebAIMuseRenderer(func(ctx context.Context, session *studio.WebAISession, request studio.MuseChatRequest, outputPath string, progress func(string)) (string, error) {
		if session.Platform != webai.PlatformMuseAI {
			return "", fmt.Errorf("Muse renderer requires a Muse account, got %q", session.Platform)
		}
		name := strings.Join(strings.Fields(request.AccountName), " ")
		if name == "" {
			name = strings.Join(strings.Fields(session.AccountName), " ")
		}
		if runes := []rune(name); len(runes) > 80 {
			name = string(runes[:80])
		}
		reply, err := webai.MuseChatHeadlessWithFilesAndThreadCookieSnapshotAndAccountName(
			ctx, session.CookieHeader, session.CookieJar, request.Prompt, request.Files,
			request.ThreadURL, request.NewThread, name, progress, session.CookieUpdater,
		)
		session.CookieHeader = reply.CookieHeader
		if len(reply.CookieJar) > 0 {
			session.CookieJar = append([]webai.BrowserCookie(nil), reply.CookieJar...)
		}
		if err != nil {
			return "", err
		}
		if len(reply.MediaData) > 0 {
			if err := saveMuseVideoData(outputPath, reply.MediaData, reply.MediaMime); err != nil {
				return "", fmt.Errorf("could not save Muse video output: %w", err)
			}
			return reply.ThreadURL, nil
		}
		mediaURL := ""
		for _, candidate := range reply.MediaURLs {
			parsed, parseErr := url.Parse(candidate)
			if parseErr != nil {
				continue
			}
			ext := strings.ToLower(filepath.Ext(parsed.Path))
			if ext == ".mp4" || ext == ".webm" {
				mediaURL = candidate
				break
			}
		}
		if mediaURL == "" {
			message := strings.Join(strings.Fields(reply.Text), " ")
			if len([]rune(message)) > 500 {
				message = string([]rune(message)[:500]) + "…"
			}
			return "", fmt.Errorf("Muse replied without a downloadable video: %s", message)
		}
		if err := downloadMuseMediaWithCookieSnapshot(ctx, session.CookieHeader, session.CookieJar, mediaURL, outputPath, session.CookieUpdater); err != nil {
			return "", err
		}
		return reply.ThreadURL, nil
	})
	s.Studio.SetWebAIImageAnalyzer(func(ctx context.Context, session *studio.WebAISession, prompt string, imagePaths []string) (string, error) {
		if session == nil {
			return "", fmt.Errorf("Web AI session is missing")
		}
		return webai.AnalyzeImagesInWebUI(ctx, session.Platform, session.CookieHeader, session.CookieJar, prompt, imagePaths, session.CookieUpdater)
	})
	s.Studio.SetWebAIVideoRenderer(func(ctx context.Context, session *studio.WebAISession, request studio.WebAIVideoRequest, outputPath string, progress func(string)) (string, error) {
		if session.Platform != webai.PlatformGoogleFlow {
			return "", fmt.Errorf("reference video generation only supports Google Flow, got %q", session.Platform)
		}
		current := session
		defer func() { *session = *current }()
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
					current = s.studioWebAISession(*next)
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
					label := "Flow"
					if platform == webai.PlatformMuseAI {
						label = "Muse"
					}
					return true, label + " · " + email
				}
				return true, "đã kết nối"
			}
		}
		return false, "Chưa kết nối"
	})
	studio.SetTikTokCookieProvider(func() (string, []webai.BrowserCookie) {
		if s.WebAI == nil {
			return "", nil
		}
		accounts, err := s.WebAI.GetAccounts(webai.PlatformTikTok)
		if err != nil || len(accounts) == 0 {
			return "", nil
		}
		for _, acc := range accounts {
			if acc.IsActive && (acc.Cookie != "" || len(acc.CookieJar) > 0) {
				return acc.Cookie, acc.CookieJar
			}
		}
		return accounts[0].Cookie, accounts[0].CookieJar
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

package web

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/harness"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
)

//go:embed templates/*.html templates/*/*.html
var templateFS embed.FS

//go:embed static/style.css
var staticCSS []byte

//go:embed static/app.js
var staticJS []byte

//go:embed static/preview.mp3
var staticPreviewAudio []byte

//go:embed static/sample_916.mp4
var staticSampleVideo []byte

// assetVersion là hash nội dung CSS+JS nhúng (Đợt AK): URL asset đổi mỗi khi
// nội dung đổi, nên cập nhật app là trình duyệt lấy giao diện mới ngay —
// trước đây cache 1 giờ khiến người dùng vẫn thấy giao diện cũ.
var assetVersion = func() string {
	h := sha256.Sum256(append(append([]byte{}, staticCSS...), staticJS...))
	return hex.EncodeToString(h[:])[:10]
}()

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
	AT     *accesstrade.Store
	VieNeu VieNeuCtl
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
	// StaleTicks trả về tên các tick quá hạn chưa chạy (kẹt) — Đợt AK:
	// /healthz báo needs_restart để watchdog ngoài tự khởi động lại app.
	StaleTicks func() []string
	// Sidecars trả về sức khỏe các tiến trình phụ (llama-server, VieNeu).
	Sidecars func() map[string]SidecarBeat

	// authMu/authSessions: session đăng nhập dashboard (Đợt T). Token →
	// thời điểm hết hạn; restart là đăng nhập lại.
	authMu       sync.Mutex
	authSessions map[string]time.Time

	db        *sql.DB // read handle for decision/analytics queries
	oauth     oauthStates
	templates map[string]*template.Template

	// VieNeu model-download progress (polled by the settings page).
	vieneuMu         sync.Mutex
	vieneuDownloaded int64
	vieneuTotal      int64
	vieneuDone       bool
	vieneuErr        string
	vieneuActive     bool // đang tải nền — hiện chip trên thanh trên cùng

	// Avatar model-download progress — parked-build support (như trên).
	avatarDlMu         sync.Mutex
	avatarDlDownloaded int64
	avatarDlTotal      int64
	avatarDlDone       bool
	avatarDlErr        string

	// Live-schedule auto-build guard — parked-build support (như trên;
	// handler /schedule đã park).
	schedMu    sync.Mutex
	schedBuilt string
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
		AvatarDir: "data/avatars", // parked-build support (handler avatar)
		DataDir:   filepath.Dir(dbPath),
		StartedAt: time.Now(), // Đợt S: /healthz
		db:        db,
	}

	s.applyPersistedEnv()
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
			Platform:     acc.Platform,
			SessionToken: acc.SessionKey,
			CookieHeader: acc.Cookie,
			Email:        acc.Email,
		}, nil
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

var templateFuncs = template.FuncMap{
	"assetVer": func() string { return assetVersion },
	// fmtTime renders slot minutes-since-midnight as HH:MM.
	"fmtTime": func(min int) string {
		return fmt.Sprintf("%02d:%02d", min/60, min%60)
	},
	// fmtVN renders an ISO timestamp ("2006-01-02T15:04:05") as Vietnamese
	// "02/01/2006 15:04". Đợt AH: người Việt đọc ngày/tháng/năm tự nhiên hơn.
	"fmtVN": func(s string) string {
		for _, layout := range []string{"2006-01-02T15:04:05", time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				if layout == "2006-01-02" {
					return t.Format("02/01/2006")
				}
				return t.Format("02/01/2006 15:04")
			}
		}
		return s
	},
	// fmtVND renders a VND amount with dot thousands separators:
	// 1234500 → "1.234.500 ₫".
	"fmtVND": func(v float64) string { return formatVND(v) },
	// fmtUSD renders a USD amount with comma separators: "$1,234.50".
	"fmtUSD": func(v float64) string { return formatUSD(v) },
	// statusLabel renders a raw status slug as its Vietnamese label.
	"statusLabel": statusLabel,
	// reasonLabel dịch lý do dạng "slug_a -> slug_b" sang tiếng Việt.
	"reasonLabel": reasonLabel,
	// statusClass maps a raw status slug to a semantic badge class suffix.
	"statusClass": statusClass,
	"mul":         func(a, b float64) float64 { return a * b },
	"add":         func(a, b int) int { return a + b },
	"pct":         func(f float64) float64 { return f * 100 },
	"join": func(sep string, xs []string) string {
		return strings.Join(xs, sep)
	},
	"hasPrefix": strings.HasPrefix,
	"urlQuery":  url.QueryEscape,
	// themeLabel renders a theme slug as its catalog label.
	"themeLabel": func(slug string) string {
		if t, ok := products.DescribeTheme(slug); ok {
			return t.Label
		}
		if slug == "" {
			return "—"
		}
		return slug
	},
	// agentLabel renders a raw agent slug as its Vietnamese label + icon
	// for the decision feed (R2-11: no raw slugs in the UI).
	"agentLabel": agentLabel,
	// variantAspect renders a plan variant's delivery frame ("16:9" for the
	// YouTube long-form cut, "9:16" otherwise) so /growth shows the true
	// render shape of each variant.
	"variantAspect": growth.VariantAspect,
	// dict builds a string-keyed map for passing named arguments to
	// partial templates ({{template "x" (dict "A" .B)}}).
	"dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			if k, ok := kv[i].(string); ok {
				m[k] = kv[i+1]
			}
		}
		return m
	},
}

// statusLabel maps raw status slugs (accounts, slots, jobs, assets,
// products, decisions) to Vietnamese labels with diacritics. Unknown slugs
// fall back to a humanized form so nothing renders with underscores.
func statusLabel(s string) string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	if s == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "_", " ")
}

// reasonLabel dịch lý do kỹ thuật trong feed hoạt động: "a_b -> c_d" →
// nhãn tiếng Việt hai đầu, "(missing-key)" → "(thiếu key)". Lý do khác
// giữ nguyên.
func reasonLabel(r string) string {
	r = strings.ReplaceAll(r, "(missing-key)", "(thiếu key)")
	// Bản ghi cũ (trước khi việt hóa) vẫn nằm trong sổ cái: dịch khi hiện.
	if m := legacyTopicRe.FindStringSubmatch(r); m != nil {
		return "đã chốt chủ đề (nguồn: " + sourceLabel(m[1]) + ")"
	}
	if m := legacyAssignRe.FindStringSubmatch(r); m != nil {
		return "gán tính cách " + m[1] + " cho chủ đề “" + m[2] + "”"
	}
	if strings.HasPrefix(r, "đã chốt chủ đề (nguồn: ") {
		src := strings.TrimSuffix(strings.TrimPrefix(r, "đã chốt chủ đề (nguồn: "), ")")
		return "đã chốt chủ đề (nguồn: " + sourceLabel(src) + ")"
	}
	if a, b, ok := strings.Cut(r, " -> "); ok {
		// Đổi nhà cung cấp AI: "gemini -> llama-server (thiếu key)".
		if pa, ok1 := providerLabels[a]; ok1 {
			bName, why, _ := strings.Cut(b, " ")
			if pb, ok2 := providerLabels[bName]; ok2 {
				out := pa + " → " + pb
				if why = strings.Trim(why, "()"); why != "" {
					out += " (lý do: " + why + ")"
				}
				return out
			}
		}
		if !strings.ContainsAny(a+b, " ") {
			return statusLabel(a) + " → " + statusLabel(b)
		}
	}
	return r
}

var (
	legacyTopicRe  = regexp.MustCompile(`^topic resolved \(source=(\w+)\)$`)
	legacyAssignRe = regexp.MustCompile(`^assigned (.+) for researched niche '(.*)'$`)
)

// providerLabels: tên nhà cung cấp AI dễ hiểu cho feed hoạt động.
var providerLabels = map[string]string{
	"gemini":       "Gemini (Google)",
	"llama-server": "AI chạy trên máy",
	"vieneu":       "giọng VieNeu trên máy",
	"edge":         "giọng Edge miễn phí",
	"paid":         "nhà cung cấp trả phí",
}

func sourceLabel(s string) string {
	switch s {
	case "fallback":
		return "mặc định theo tính cách, vì chưa có AI"
	case "hint":
		return "gợi ý của bạn"
	case "auto":
		return "AI tự chọn"
	}
	return s
}

var statusLabels = map[string]string{
	// account lifecycle
	"onboarding":       "Đang thiết lập",
	"researching":      "Đang nghiên cứu",
	"persona_assigned": "Đã gán persona",
	"growing":          "Đang tăng trưởng",
	"live_ready":       "Sẵn sàng",       // legacy: trạng thái sẵn sàng từ pipeline cũ (live đã park)
	"live":             "Live (đã park)", // legacy — giữ để không vỡ status cũ
	"paused":           "Đã tạm dừng",
	"penalized":        "Bị phạt",
	"retired":          "Đã nghỉ",
	// live slots
	"planned": "Đã lên lịch",
	"started": "Đang diễn ra",
	"skipped": "Đã bỏ qua",
	// jobs & assets
	"queued":  "Đang chờ",
	"pending": "Đang chờ",
	"running": "Đang chạy",
	"done":    "Hoàn thành",
	"failed":  "Thất bại",
	// agent team node states
	"working":  "Đang làm",
	"exchange": "Đang nhận việc",
	"waiting":  "Đang chờ",
	"guard":    "Gác cổng",
	// shop products
	"shelf":  "Đang bán",
	"scaled": "Đang mở rộng",
	// decision actions shown in the activity feed
	"niche_research":          "Nghiên cứu niche",
	"account_transition":      "Chuyển giai đoạn",
	"auto_switch":             "Công tắc tự động",
	"account_penalized":       "Kênh bị phạt",
	"advance_stage":           "Lên giai đoạn mới",
	"api_budget":              "Ngân sách API",
	"api_key_add":             "Thêm API key",
	"api_key_delete":          "Xóa API key",
	"api_switch":              "Bật/tắt API",
	"at_automation_save":      "Lưu tự động Accesstrade",
	"auth_password":           "Đổi mật khẩu",
	"auto_pause_account":      "Tự tạm dừng kênh",
	"autopublish_failed":      "Tự đăng TikTok lỗi",
	"autopublish_meta_failed": "Tự đăng Meta lỗi",
	"autopublish_yt_failed":   "Tự đăng YouTube lỗi",
	"avatar_character_add":    "Thêm nhân vật",
	"avatar_character_delete": "Xóa nhân vật",
	"avatar_render_test":      "Thử dựng avatar",
	"avatar_sidecar_restart":  "Khởi động lại avatar",
	"avatar_tier_failover":    "Đổi nhà cung cấp avatar",
	"backup_create":           "Tạo bản sao lưu",
	"backup_offsite":          "Sao lưu ngoài máy",
	"backup_restore":          "Khôi phục sao lưu",
	"breakout_video":          "Video bứt phá",
	"build_schedule":          "Lập lịch",
	"campaign_pending":        "Chiến dịch chờ duyệt",
	"hunt_blocked":            "Chưa săn được sản phẩm",
	"commission_reconcile":    "Đối soát hoa hồng",
	"daily_plan":              "Kế hoạch ngày",
	"dedup_regenerate_angle":  "Đổi góc tránh trùng",
	"defer_quota":             "Hoãn vì hết quota",
	"deny":                    "Từ chối",
	"distributed":             "Đã phân phối",
	"double_down_format":      "Nhân đôi format tốt",
	"drop_stale_item":         "Bỏ mục quá hạn",
	"dry_run":                 "Chế độ chạy thử",
	"error":                   "Lỗi",
	"generate_plan":           "Tạo kế hoạch",
	"kill_format":             "Dừng format kém",
	"kill_product":            "Dừng sản phẩm",
	"kill_switch":             "Dừng khẩn cấp",
	"login":                   "Đăng nhập",
	"mark_stalled":            "Đánh dấu chững",
	"master_switch":           "Công tắc chính",
	"milestone_reached":       "Đạt mốc",
	"network_pause":           "Tạm dừng toàn mạng",
	"notify_save":             "Lưu thông báo",
	"onboard":                 "Thiết lập ban đầu",
	"production_toggle":       "Bật/tắt sản xuất",
	"publish_youtube":         "Đăng YouTube",
	"viral_produce":          "Sản xuất video viral",
	"viral_auto_toggle":      "Bật/tắt tự động viral",
	"harness_cycle":          "Chu kỳ nhận thức OODA",
	"harness_autonomy":       "Đổi cấp độ tự hành",
	"harness_memory":         "Ghi nhớ kinh nghiệm",
	"scale_product":           "Mở rộng sản phẩm",
	"seed":                    "Khởi tạo",
	"session_topic":           "Chủ đề phiên",
	"shelf_product":           "Lên kệ sản phẩm",
	"skip":                    "Bỏ qua",
	"skip_duplicate_youtube":  "Bỏ video trùng",
	"slot_skipped":            "Bỏ lượt",
	"thresholds_reset":        "Đặt lại ngưỡng",
	"thresholds_updated":      "Cập nhật ngưỡng",
	"tier_failover":           "Đổi nhà cung cấp AI",
	"live_unlocked":           "Mở khóa live",
	// growth stages (CHANNEL_GROWTH)
	"cold_start":        "Khởi động (canary)",
	"format_testing":    "Đang test format",
	"scaling":           "Đang mở rộng",
	"monetization_push": "Đẩy tới mốc kiếm tiền",
	"monetized":         "Đã qua cửa kiếm tiền",
	"stalled":           "Đang chững (chờ replan)",
	// growth format verdicts + plan item statuses
	"testing":    "Đang test",
	"winner":     "Format thắng",
	"killed":     "Đã dừng format",
	"producing":  "Đang sản xuất",
	"published":  "Đã đăng",
	"dropped":    "Đã loại",
	"active":     "Đang hiệu lực",
	"superseded": "Đã thay thế",
	// growth phase-2 production/publish states
	"produced":        "Đã sản xuất",
	"waiting_connect": "Chờ kết nối YouTube",
	"waiting_quota":   "Chờ quota YouTube",
}

// statusClass maps a raw status slug to a semantic badge class suffix
// (ok/warn/err/info/no/running/queued/draft) defined in style.css.
func statusClass(s string) string {
	switch s {
	case "live", "live_ready", "done", "started", "shelf", "scaled", "monetized", "winner", "published", "active":
		return "ok"
	case "running", "working", "producing":
		return "running"
	case "queued", "pending":
		return "queued"
	case "failed", "penalized", "killed":
		return "err"
	case "onboarding", "paused", "skipped", "cold_start", "stalled":
		return "warn"
	case "researching", "persona_assigned", "growing", "planned", "exchange", "format_testing", "monetization_push", "scaling", "testing", "produced":
		return "info"
	case "waiting_connect", "waiting_quota":
		return "warn"
	case "dropped", "superseded":
		return "no"
	default:
		return "no"
	}
}

// formatVND renders v as a Vietnamese dong amount: 1234500 → "1.234.500 ₫".
func formatVND(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String() + " ₫"
	}
	return b.String() + " ₫"
}

// formatUSD renders v as a US dollar amount: 1234.5 → "$1,234.50".
// Sub-cent values (API micro-costs) keep 4 decimals instead of "$0.00".
func formatUSD(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	if v > 0 && v < 0.01 {
		if neg {
			return fmt.Sprintf("-$%.4f", v)
		}
		return fmt.Sprintf("$%.4f", v)
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac == 100 {
		whole++
		frac = 0
	}
	digits := strconv.FormatInt(whole, 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return fmt.Sprintf("-$%s.%02d", b.String(), frac)
	}
	return fmt.Sprintf("$%s.%02d", b.String(), frac)
}

// agentMeta is the display form of an agent slug in the decision feed.
type agentMeta struct {
	Icon  string
	Label string
}

// agentLabels maps raw agent slugs (ledger.decisions.agent) to Vietnamese
// display labels. Unknown slugs fall back to a humanized form so nothing
// renders with underscores (R2-11).
var agentLabels = map[string]agentMeta{
	"growth_director":  {Icon: "🌱", Label: "Đạo diễn tăng trưởng"},
	"growth_analyst":   {Icon: "📊", Label: "Phân tích tăng trưởng"},
	"growth_publisher": {Icon: "📤", Label: "Đăng bài tăng trưởng"},
	"growth":           {Icon: "🌱", Label: "Tăng trưởng"},
	"live_planner":     {Icon: "📅", Label: "Lập lịch live"},
	"scheduler":        {Icon: "🗓", Label: "Xếp lịch"},
	"governance":       {Icon: "🛡", Label: "Kiểm soát"},
	"governor":         {Icon: "🛡", Label: "Kiểm soát"},
	"analyst":          {Icon: "📊", Label: "Phân tích"},
	"hunter":           {Icon: "🔎", Label: "Săn sản phẩm"},
	"director":         {Icon: "🎬", Label: "Đạo diễn"},
	"producer":         {Icon: "🎥", Label: "Sản xuất"},
	"content":          {Icon: "✍️", Label: "Nội dung"},
	"onboarding":       {Icon: "🚀", Label: "Thiết lập"},
	"orchestrator":     {Icon: "🤖", Label: "Điều phối"},
	"engines":          {Icon: "⚙️", Label: "Động cơ AI"},
	"system":           {Icon: "⚙️", Label: "Hệ thống"},
	"human":            {Icon: "👤", Label: "Người dùng"},
	"test":             {Icon: "🧪", Label: "Kiểm thử"},
}

func agentLabel(slug string) agentMeta {
	if m, ok := agentLabels[slug]; ok {
		return m
	}
	if slug == "" {
		return agentMeta{Icon: "⚙️", Label: "—"}
	}
	return agentMeta{Icon: "⚙️", Label: strings.ReplaceAll(slug, "_", " ")}
}

// pageFiles maps a page key to the template file rendered inside base.
var pageFiles = map[string]string{
	"dashboard":             "dashboard.html",
	"accounts":              "accounts.html",
	"account_new":           "account_new.html",
	"account_edit":          "account_edit.html",
	"account_detail":        "account_detail.html",
	"studio":                "studio.html",
	"studio_jobs":           "studio_jobs.html",
	"studio_trends":         "studio_trends.html",
	"publishers":            "publishers.html",
	"products":              "products.html",
	"viral":                 "viral.html",
	"stories":               "stories.html",
	"team":                  "team.html",
	"growth":                "growth.html",
	"harness":               "harness.html",
	"settings_he_thong":     "settings/he-thong.html",
	"settings_nha_cung_cap": "settings/nha-cung-cap.html",
	"settings_an_toan":      "settings/an-toan.html",
	"settings_accesstrade":  "settings/accesstrade.html",
	"web_ai":                "web_ai.html",
	"onboard":               "onboard.html",
}

func (s *Server) parseTemplates() error {
	s.templates = make(map[string]*template.Template, len(pageFiles))
	for key, file := range pageFiles {
		t, err := template.New("base").Funcs(templateFuncs).ParseFS(
			templateFS, "templates/base.html", "templates/partials/shared.html",
			"templates/"+file)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", file, err)
		}
		s.templates[key] = t
	}
	// Đợt T: trang đăng nhập standalone (không sidebar) cho authGate.
	t, err := template.New("login").Funcs(templateFuncs).ParseFS(
		templateFS, "templates/login.html")
	if err != nil {
		return fmt.Errorf("parse template login.html: %w", err)
	}
	s.templates["login"] = t

	// Bio Link Storefront standalone (không sidebar, mobile-first Linktree-style).
	tbio, err := template.New("bio").Funcs(templateFuncs).ParseFS(
		templateFS, "templates/bio.html")
	if err != nil {
		return fmt.Errorf("parse template bio.html: %w", err)
	}
	s.templates["bio"] = tbio
	return nil
}

// ctx builds the template context: base flags (Kill/DryRun) plus page data.
func (s *Server) ctx(kv ...any) map[string]any {
	m := map[string]any{
		"Kill":    s.Cfg.KillSwitch(),
		"DryRun":  s.Cfg.DryRun(),
		"AuthOn":  s.authEnabled(),
		"LocalDL": s.localDownloadChip(),
	}
	if s.WebAI != nil {
		m["WebAICount"] = s.WebAI.CountConnected()
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}

// render executes a page inside the base layout. A template/handler error
// never crashes the server: it is logged and answered with a 500.
func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	t, ok := s.templates[page]
	if !ok {
		http.Error(w, "unknown page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("web: render %s: %v", page, err)
	}
}

// --------------------------------------------------------------- view types

// accountView is a network account plus its rendered persona label and growth metrics.
type accountView struct {
	*network.Account
	PersonaLabel string
	Platforms    []string
	GrowthStage  string
	Views30d     int64
	FollowersCnt int64
	NextTarget   string
	NextPct      float64
	HasNext      bool
	AffiliateLink string
	ModelPhotoURL string
	HasModel      bool
}

func personaLabel(key string) string {
	if p, ok := network.PERSONAS[key]; ok {
		return p.Label
	}
	if key == "" {
		return "—"
	}
	return key
}

func (s *Server) wrapAccount(a *network.Account) *accountView {
	ytTypes := a.YoutubeContentTypes
	if len(ytTypes) == 0 && fileExists(publishers.YouTubeTokenPath(a.Username)) {
		ytTypes = []string{"short_video", "short_film"}
		if s.Mgr != nil {
			ch := a.YoutubeChannel
			if ch == "" {
				ch = a.Username
			}
			go func(id int64, channel string) {
				_, _ = s.Mgr.SetYoutube(id, channel, []string{"short_video", "short_film"})
			}(a.ID, ch)
		}
	}
	var plats []string
	for _, p := range publishers.BuildPublishers(a.Username, a.YoutubeChannel, ytTypes) {
		plats = append(plats, p.Name())
	}
	av := &accountView{Account: a, PersonaLabel: personaLabel(a.Persona), Platforms: plats}
	if s.Growth != nil {
		if prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err == nil && prof != nil {
			switch prof.Stage {
			case "cold_start":
				av.GrowthStage = "Khởi động (0-1K)"
			case "monetization":
				av.GrowthStage = "Bật kiếm tiền / YPP"
			case "scaling":
				av.GrowthStage = "Mở rộng quy mô"
			default:
				av.GrowthStage = prof.Stage
			}
		}
		if snap, err := s.Growth.LatestSnapshot(a.ID); err == nil && snap != nil {
			if v, ok := metricValue(snap, "followers"); ok {
				av.FollowersCnt = int64(v)
			}
			if v, ok := metricValue(snap, "views_30d"); ok {
				av.Views30d = int64(v)
			}
		}
		if targets, err := s.Growth.ListTargets(a.ID); err == nil && len(targets) > 0 {
			for _, t := range targets {
				if t.ReachedAt == "" {
					av.NextTarget = t.Label
					av.HasNext = true
					if t.Threshold > 0 && av.FollowersCnt > 0 {
						av.NextPct = float64(av.FollowersCnt) / t.Threshold * 100
						if av.NextPct > 100 {
							av.NextPct = 100
						}
					}
					break
				}
			}
		}
	}
	if av.GrowthStage == "" {
		av.GrowthStage = "Đang vận hành"
	}
	av.AffiliateLink = "https://go.isclix.com/deal-hot"
	if s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil && ms != nil {
			if photos, err := ms.List(a.ID); err == nil && len(photos) > 0 {
				av.HasModel = true
				av.ModelPhotoURL = fmt.Sprintf("/models/%d/%s", a.ID, filepath.Base(photos[0].Path))
			}
		}
	}
	return av
}

type statusCount struct {
	Status string
	N      int
}

func sortedStatusCounts(accounts []*network.Account) []statusCount {
	counts := map[string]int{}
	for _, a := range accounts {
		counts[a.Status]++
	}
	out := make([]statusCount, 0, len(counts))
	for st, n := range counts {
		out = append(out, statusCount{st, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Status < out[j].Status })
	return out
}

// decisionView is the template/JSON-safe projection of a decisions row.
type decisionView struct {
	ID        int64  `json:"id"`
	Agent     string `json:"agent"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// queryDecisions runs a decisions-table SELECT the ledger API does not
// expose, projecting to decisionView.
func (s *Server) queryDecisions(query string, args ...any) ([]decisionView, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []decisionView
	for rows.Next() {
		var d decisionView
		var target, reason sql.NullString
		if err := rows.Scan(&d.ID, &d.Agent, &d.Action, &target, &reason, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Target, d.Reason = nullStr(target), nullStr(reason)
		out = append(out, d)
	}
	return out, rows.Err()
}

// scalarFloat is a tiny helper for the one-off aggregate queries.
func (s *Server) scalarFloat(query string, args ...any) float64 {
	var v float64
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		return 0
	}
	return v
}

// scalarInt is scalarFloat's integer sibling (COUNT-style aggregates).
func (s *Server) scalarInt(query string, args ...any) int64 {
	var v int64
	if err := s.db.QueryRow(query, args...).Scan(&v); err != nil {
		return 0
	}
	return v
}

// --------------------------------------------------------------- publisher

type publisherRow struct {
	Username         string
	Status           string
	TiktokToken      bool
	TiktokTokenDead  bool // refresh token chết / hết hạn
	TiktokClient     bool // client key + secret present in env
	YoutubeToken     bool
	YoutubeClient    bool // OAuth client ID + secret present in env (Đợt AA)
	YoutubeTokenDead bool // refresh token chết (invalid_grant) — Đợt AA
	FbPage           bool
	Instagram        bool // IG_USER_ID + META_PAGE_TOKEN
	Threads          bool // THREADS_USER_ID + THREADS_TOKEN
	Platforms        []string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DB returns the underlying sqlite DB handle.
func (s *Server) DB() *sql.DB {
	return s.db
}

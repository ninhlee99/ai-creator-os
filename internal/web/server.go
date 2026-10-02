package web

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
)

//go:embed templates/*.html templates/*/*.html
var templateFS embed.FS

//go:embed static/style.css
var staticCSS []byte

//go:embed static/app.js
var staticJS []byte

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
	// reconciliation (R2-W1). Nil means the real TikTok Shop client is
	// built from the current env at call time; tests inject fakes.
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
	VieNeu           VieNeuCtl
	Health           map[string]HealthChecker
	Jobs             *JobStore
	// AvatarSidecar: parked-build support (như Avatar ở trên).
	AvatarSidecar AvatarSidecarCtl

	// OutDir holds rendered videos (served at /media/). JobsPath is the
	// JobStore file.
	OutDir   string
	JobsPath string
	// DataDir is the app data directory (sqlite DBs, jobs, tokens). The
	// single place state lives; derived from the ledger path (R2-W7).
	DataDir string
	// Onboarding puts the server in first-run wizard mode: every route
	// except /onboard redirects to the wizard until it completes.
	Onboarding bool

	db        *sql.DB // read handle for decision/analytics queries
	oauth     oauthStates
	templates map[string]*template.Template
	renderer  Renderer

	// VieNeu model-download progress (polled by the settings page).
	vieneuMu         sync.Mutex
	vieneuDownloaded int64
	vieneuTotal      int64
	vieneuDone       bool
	vieneuErr        string

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
		db:        db,
	}
	// R2-W7: job file follows the -data flag (it used to be hardcoded to
	// ./data/content_jobs.json regardless of where the DBs lived).
	s.JobsPath = filepath.Join(s.DataDir, "content_jobs.json")
	s.applyPersistedEnv()
	if gs, err := growth.NewStore(db); err != nil {
		log.Printf("web: growth store: %v", err)
	} else {
		s.Growth = gs
	}
	s.renderer = ffmpegRenderer{s: s}
	if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
		db.Close()
		return nil, err
	}
	s.Jobs = NewJobStore(s.JobsPath)
	if err := s.parseTemplates(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
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
	// fmtTime renders slot minutes-since-midnight as HH:MM.
	"fmtTime": func(min int) string {
		return fmt.Sprintf("%02d:%02d", min/60, min%60)
	},
	// fmtVND renders a VND amount with dot thousands separators:
	// 1234500 → "1.234.500 ₫".
	"fmtVND": func(v float64) string { return formatVND(v) },
	// fmtUSD renders a USD amount with comma separators: "$1,234.50".
	"fmtUSD": func(v float64) string { return formatUSD(v) },
	// statusLabel renders a raw status slug as its Vietnamese label.
	"statusLabel": statusLabel,
	// statusClass maps a raw status slug to a semantic badge class suffix.
	"statusClass": statusClass,
	"mul":         func(a, b float64) float64 { return a * b },
	"pct":         func(f float64) float64 { return f * 100 },
	"join": func(sep string, xs []string) string {
		return strings.Join(xs, sep)
	},
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

var statusLabels = map[string]string{
	// account lifecycle
	"onboarding":       "Đang thiết lập",
	"researching":      "Đang nghiên cứu",
	"persona_assigned": "Đã gán persona",
	"growing":          "Đang tăng trưởng",
	"live_ready":       "Đủ điều kiện live",
	"live":             "Đang live",
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
	"niche_research": "Nghiên cứu niche",
	"live_unlocked":  "Mở khóa live",
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
	"account_detail":        "account_detail.html",
	"studio":                "studio.html",
	"studio_jobs":           "studio_jobs.html",
	"studio_trends":         "studio_trends.html",
	"publishers":            "publishers.html",
	"products":              "products.html",
	"growth":                "growth.html",
	"settings_he_thong":     "settings/he-thong.html",
	"settings_nha_cung_cap": "settings/nha-cung-cap.html",
	"settings_model_local":  "settings/model-local.html",
	"settings_an_toan":      "settings/an-toan.html",
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
	return nil
}

// ctx builds the template context: base flags (Kill/DryRun) plus page data.
func (s *Server) ctx(kv ...any) map[string]any {
	m := map[string]any{
		"Kill":   s.Cfg.KillSwitch(),
		"DryRun": s.Cfg.DryRun(),
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

// accountView is a network account plus its rendered persona label.
type accountView struct {
	*network.Account
	PersonaLabel string
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
	return &accountView{Account: a, PersonaLabel: personaLabel(a.Persona)}
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
	Username     string
	Status       string
	TiktokToken  bool
	TiktokClient bool // client key + secret present in env
	YoutubeToken bool
	FbPage       bool
	Rtmp         bool
	Platforms    []string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

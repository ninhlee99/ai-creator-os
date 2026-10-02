package web

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/style.css
var staticCSS []byte

// Server is the web dashboard. Exported fields are the wiring surface:
// the parent worker injects the ledger, account manager, config, engines
// and sidecar controls here.
type Server struct {
	Ledger *ledger.Ledger
	Mgr    *network.AccountManager
	Cfg    *Config
	LLM    LLMClient
	TTS    TTSChainAPI
	Avatar AvatarChainAPI
	Studio *studio.Studio
	// Autopilot runs hands-off affiliate cycles (products -> video).
	// Products is the affiliate product store. ProductProviders are the
	// configured product search providers. All three are injected by the
	// cmd wiring; handlers degrade gracefully when they are nil.
	Autopilot        *studio.Autopilot
	Products         *products.Store
	ProductProviders []products.Provider
	VieNeu           VieNeuCtl
	AvatarSidecar    AvatarSidecarCtl
	Health           map[string]HealthChecker
	Jobs             *JobStore

	// OutDir holds rendered videos (served at /media/); AvatarDir holds
	// character reference images (served at /avatars/). JobsPath is the
	// JobStore file.
	OutDir    string
	AvatarDir string
	JobsPath  string

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

	// Avatar model-download progress (polled by the settings page).
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
		AvatarDir: "data/avatars",
		JobsPath:  "data/content_jobs.json",
		db:        db,
	}
	s.applyPersistedEnv()
	s.renderer = ffmpegRenderer{s: s}
	if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.MkdirAll(s.AvatarDir, 0o755); err != nil {
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
	"mul": func(a, b float64) float64 { return a * b },
	"pct": func(f float64) float64 { return f * 100 },
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
}

// statusClass maps a raw status slug to a semantic badge class suffix
// (ok/warn/err/info/no/running/queued/draft) defined in style.css.
func statusClass(s string) string {
	switch s {
	case "live", "live_ready", "done", "started", "shelf", "scaled":
		return "ok"
	case "running", "working":
		return "running"
	case "queued", "pending":
		return "queued"
	case "failed", "penalized":
		return "err"
	case "onboarding", "paused", "skipped":
		return "warn"
	case "researching", "persona_assigned", "growing", "planned", "exchange":
		return "info"
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

// pageFiles maps a page key to the template file rendered inside base.
var pageFiles = map[string]string{
	"dashboard":      "dashboard.html",
	"accounts":       "accounts.html",
	"account_new":    "account_new.html",
	"account_detail": "account_detail.html",
	"schedule":       "schedule.html",
	"content":        "content.html",
	"studio":         "studio.html",
	"publishers":     "publishers.html",
	"shop":           "shop.html",
	"products":       "products.html",
	"analytics":      "analytics.html",
	"settings":       "settings.html",
	"team":           "team.html",
}

func (s *Server) parseTemplates() error {
	s.templates = make(map[string]*template.Template, len(pageFiles))
	for key, file := range pageFiles {
		t, err := template.New("base").Funcs(templateFuncs).ParseFS(
			templateFS, "templates/base.html", "templates/"+file)
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

// slotView couples a slot with its account's username for display.
type slotView struct {
	ledger.Slot
	AccountName string
}

func (s *Server) slotViews(slots []ledger.Slot) []slotView {
	names := map[int64]string{}
	if accts, err := s.Mgr.List(); err == nil {
		for _, a := range accts {
			names[a.ID] = a.Username
		}
	}
	out := make([]slotView, 0, len(slots))
	for _, sl := range slots {
		name, ok := names[sl.AccountID]
		if !ok {
			name = strconv.FormatInt(sl.AccountID, 10)
		}
		out = append(out, slotView{Slot: sl, AccountName: name})
	}
	return out
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

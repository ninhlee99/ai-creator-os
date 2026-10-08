package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

// themeOption is one entry of the theme dropdown.
type themeOption struct {
	Slug  string
	Label string
}

// themeOptions lists the curated themes in deterministic order.
func themeOptions() []themeOption {
	out := make([]themeOption, 0, len(products.ThemeOrder))
	for _, slug := range products.ThemeOrder {
		label := slug
		if t, ok := products.DescribeTheme(slug); ok {
			label = t.Label
		}
		out = append(out, themeOption{Slug: slug, Label: label})
	}
	return out
}

// resolveMinCommission parses the "min_commission" percent field; an empty
// value falls back to the theme's suggested floor.
func resolveMinCommission(theme, raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if t, ok := products.DescribeTheme(theme); ok {
			return t.DefaultMinCommission * 100
		}
		return 0
	}
	p, _ := strconv.ParseFloat(raw, 64)
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

// ---------------------------------------------------------------- products

// handleProducts renders the product discovery page: theme + commission
// floor form, plus the best saved products for the selected theme. The
// "ke" tab is the affiliate shelf (trang /shop cũ đã gộp vào đây — Đợt 2).
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	if tab != "ke" {
		tab = "tim"
	}
	if s.Products == nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Tab", tab,
			"Themes", themeOptions(),
			"Shelf", s.shelfViews(),
			"Error", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
		))
		return
	}
	theme := strings.TrimSpace(r.URL.Query().Get("theme"))
	minPct := resolveMinCommission(theme, r.URL.Query().Get("min"))
	var results []products.Product
	if theme != "" {
		var err error
		results, err = s.Products.TopByTheme(theme, minPct/100, 0, 30)
		if err != nil {
			s.render(w, "products", s.ctx(
				"AT", s.atPageData(),
				"Tab", tab,
				"Themes", themeOptions(),
				"Theme", theme,
				"MinPct", minPct,
				"Shelf", s.shelfViews(),
				"Error", "Không đọc được kho sản phẩm: "+err.Error(),
			))
			return
		}
	}
	enabled, hours, lastRun, music, autoPub, youTube := s.scheduleView()
	// Autopilot status surface (A5): next scheduled run + stock on hand.
	nextRun := ""
	if enabled {
		if lt, err := time.Parse(time.RFC3339, lastRun); err == nil {
			nextRun = lt.Add(time.Duration(hours) * time.Hour).Format("15:04 02/01")
		} else {
			nextRun = "vòng tới"
		}
	}
	var stock int64
	if n, err := s.Products.Count(); err == nil {
		stock = n
	}
	s.render(w, "products", s.ctx(
		"AT", s.atPageData(),
		"Tab", tab,
		"Themes", themeOptions(),
		"Theme", theme,
		"MinPct", minPct,
		"Results", results,
		"Shelf", s.shelfViews(),
		"Notice", r.URL.Query().Get("ok"),
		"SchedEnabled", enabled,
		"SchedHours", hours,
		"SchedLastRun", lastRun,
		"SchedNextRun", nextRun,
		"StockCount", stock,
		"MusicName", music,
		"AutoPublish", autoPub,
		"YouTubeEnabled", youTube,
	))
}

// productView is the template projection of one shelf product (Kệ hàng).
type productView struct {
	Title           string
	PlatformPID     string
	Category        string
	Price           float64
	CommissionRate  float64
	CommissionValue float64
	Score           float64
	Status          string
}

// shelfViews projects the shared product store's shelf (status
// shelf/scaled) for the "Kệ hàng" tab — the products AI gắn vào
// video/live. R2-W1: one store — autopilot and the shelf read the same
// kho, so adding here makes the product immediately pickable.
func (s *Server) shelfViews() []productView {
	if s.Products == nil {
		return nil
	}
	shelf, err := s.Products.Shelf(50)
	if err != nil {
		log.Printf("web: shelf products: %v", err)
		return nil
	}
	views := make([]productView, 0, len(shelf))
	for _, p := range shelf {
		views = append(views, productView{
			Title: p.Title, PlatformPID: p.SourceID,
			Category: p.Category, Price: p.Price,
			CommissionRate:  p.CommissionRate,
			CommissionValue: math.Round(p.Price*p.CommissionRate*100) / 100,
			Score:           products.Score(p), Status: p.ShelfStatus,
		})
	}
	return views
}

// ledgerShelfMigrationKey marks the one-time import of the legacy ledger
// products table into the shared store.
const ledgerShelfMigrationKey = "migration.ledger_products_v1"

// MigrateLedgerShelf imports the legacy ledger products table (the Kệ
// hàng tab's old home) into the shared product store — once, idempotently.
// Shelf/scaled rows keep their shelf status + score; discovered/candidate
// rows become plain store products. The ledger table itself is left
// untouched for the parked agent universe (hunter/analyst/streamer).
func (s *Server) MigrateLedgerShelf() (int, error) {
	if s.Products == nil || s.Ledger == nil {
		return 0, nil
	}
	if _, done := s.Products.GetSetting(ledgerShelfMigrationKey); done {
		return 0, nil
	}
	legacy, err := s.Ledger.GetProducts()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, lp := range legacy {
		rating := 0.0
		if lp.SellerRating.Valid {
			rating = lp.SellerRating.Float64
		}
		id, err := s.Products.Save(products.Product{
			Source: "tiktok_shop", SourceID: lp.PlatformPID, Title: lp.Title,
			Category: nullStr(lp.Category), Price: lp.Price,
			CommissionRate: lp.CommissionRate, Rating: rating,
		})
		if err != nil {
			return n, err
		}
		if lp.Status == "shelf" || lp.Status == "scaled" {
			if err := s.Products.SetShelf(id, lp.Status, lp.Score); err != nil {
				return n, err
			}
		}
		n++
	}
	if err := s.Products.SetSetting(ledgerShelfMigrationKey, "1"); err != nil {
		return n, err
	}
	return n, nil
}

// handleProductsShelfAdd adds one product straight onto the shelf (Kệ
// hàng tab) — the manual fallback while the TikTok Shop API is not wired.
// Writes the shared store so the autopilot picks it up immediately.
func (s *Server) handleProductsShelfAdd(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "add shelf product")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse shelf form")
		return
	}
	pid := strings.TrimSpace(r.PostFormValue("platform_pid"))
	title := strings.TrimSpace(r.PostFormValue("title"))
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)
	rate, _ := strconv.ParseFloat(r.PostFormValue("commission_rate"), 64)
	if pid == "" || title == "" {
		seeOther(w, r, "/products?tab=ke")
		return
	}
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	if theme == "" {
		// Don't wipe the theme of a product we already discovered.
		if prev, err := s.Products.GetBySource("manual", pid); err == nil {
			theme = prev.Theme
		}
	}
	p := products.Product{
		Source: "manual", SourceID: pid, Title: title,
		Category: strings.TrimSpace(r.PostFormValue("category")),
		Price:    price, CommissionRate: rate, Theme: theme,
	}
	id, err := s.Products.Save(p)
	if err != nil {
		seeOther(w, r, "/products?tab=ke&err="+url.QueryEscape("Không lưu được sản phẩm: "+err.Error()))
		return
	}
	if err := s.Products.SetShelf(id, "shelf", 0); err != nil {
		seeOther(w, r, "/products?tab=ke&err="+url.QueryEscape("Đã lưu sản phẩm nhưng không chuyển được vào kệ: "+err.Error()))
		return
	}
	seeOther(w, r, "/products?tab=ke")
}

// handleProductsAdd saves a manually-entered product into the store. This is
// the practical path while the TikTok Shop provider is fail-closed: Ninh
// (or Claude on the Mac) pastes a high-commission product's details, and
// autopilot can then pick it up like any discovered product.
func (s *Server) handleProductsAdd(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "add product")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse product form")
		return
	}
	title := strings.TrimSpace(r.PostFormValue("title"))
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	if title == "" || theme == "" {
		s.fail(w, fmt.Errorf("thiếu tên sản phẩm hoặc theme"), "add product")
		return
	}
	price, _ := strconv.ParseFloat(strings.TrimSpace(r.PostFormValue("price")), 64)
	commPct, _ := strconv.ParseFloat(strings.TrimSpace(r.PostFormValue("commission_pct")), 64)
	rating, _ := strconv.ParseFloat(strings.TrimSpace(r.PostFormValue("rating")), 64)
	var sold int64
	fmt.Sscanf(strings.TrimSpace(r.PostFormValue("sold")), "%d", &sold)
	var imgs []string
	for _, u := range strings.Split(r.PostFormValue("image_url"), "\n") {
		if u = strings.TrimSpace(u); u != "" {
			imgs = append(imgs, u)
		}
	}
	p := products.Product{
		Source:         "manual",
		SourceID:       fmt.Sprintf("manual-%d", time.Now().UnixNano()),
		Title:          title,
		Theme:          theme,
		ShopName:       strings.TrimSpace(r.PostFormValue("shop")),
		Price:          price,
		Currency:       "VND",
		CommissionRate: commPct / 100,
		Rating:         rating,
		SoldCount:      sold,
		ImageURLs:      imgs,
		ProductURL:     strings.TrimSpace(r.PostFormValue("product_url")),
	}
	if _, err := s.Products.Save(p); err != nil {
		s.fail(w, err, "save product")
		return
	}
	seeOther(w, r, "/products?theme="+url.QueryEscape(theme)+"&ok="+url.QueryEscape("Đã lưu sản phẩm \""+title+"\" — autopilot có thể dùng ngay."))
}

// handleProductsSearch runs the configured providers for a theme, saves
// every hit into the product store, then displays TopByTheme. Provider
// problems are shown honestly instead of hidden.
func (s *Server) handleProductsSearch(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
		))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse products search form")
		return
	}
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	minPct := resolveMinCommission(theme, r.PostFormValue("min_commission"))
	minComm := minPct / 100
	if theme == "" {
		seeOther(w, r, "/products")
		return
	}
	if len(s.ProductProviders) == 0 {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Theme", theme,
			"MinPct", minPct,
			"Error", "Kho sản phẩm trống cho theme này. Hunter Accesstrade tự quét datafeed mỗi ngày — kiểm tra tab Accesstrade trong Cài đặt (nhập API key một lần).",
		))
		return
	}
	var warnings []string
	for _, p := range s.ProductProviders {
		if c, ok := p.(interface{ Configured() bool }); ok && !c.Configured() {
			warnings = append(warnings,
				fmt.Sprintf("Nguồn %q chưa cấu hình (thiếu API key) — đã bỏ qua.", p.Name()))
		}
	}
	q := products.QueryForTheme(theme, minComm, 50)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	found, errs := products.Aggregate(ctx, s.ProductProviders, q)
	for _, e := range errs {
		warnings = append(warnings, e.Error())
	}
	saved := 0
	for _, p := range found {
		p.Theme = theme
		if _, err := s.Products.Save(p); err != nil {
			log.Printf("web: save product: %v", err)
			continue
		}
		saved++
	}
	results, rerr := s.Products.TopByTheme(theme, minComm, 0, 30)
	data := s.ctx(
		"Themes", themeOptions(),
		"Theme", theme,
		"MinPct", minPct,
		"Results", results,
		"Searched", true,
		"Found", len(found),
		"Saved", saved,
		"Warnings", warnings,
	)
	if rerr != nil {
		data["Error"] = "Không đọc được kho sản phẩm: " + rerr.Error()
	}
	data["AT"] = s.atPageData()
	s.render(w, "products", data)
}

// ------------------------------------------------- account autopilot policy

// autopilotMusicPath is where the UI-uploaded music bed lives.
func autopilotMusicPath(databasePath string) string {
	dir := "."
	if databasePath != "" {
		dir = filepath.Dir(databasePath)
	}
	return filepath.Join(dir, "autopilot-music.m4a")
}

// scheduleView reads the current schedule state for the products page.
func (s *Server) scheduleView() (enabled bool, hours int, lastRun, music string, autoPublish, youTube bool) {
	// Automation switches read from the single settings facade (R2-W4):
	// ledger settings. Switches default ON when unset (Đợt 3); a stored
	// "0" is an explicit operator choice and always wins.
	st := s.settings()
	enabled = automation.AutopilotEnabled(st)
	hours = automation.AutopilotIntervalHours(st)
	lastRun = automation.AutopilotLastRun(st)
	music = automation.AutopilotMusicName(st)
	autoPublish = automation.AutopilotAutoPublish(st)
	youTube = automation.AutopilotYouTubeEnabled(st)
	return
}

// handleProductsSchedule saves the autopilot background schedule from the
// web UI: enable toggle + interval in hours + auto-publish toggle.
func (s *Server) handleProductsSchedule(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		s.fail(w, fmt.Errorf("kho cài đặt chưa khởi tạo"), "save schedule")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse schedule form")
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	autoPub := "0"
	if r.PostFormValue("auto_publish") == "on" {
		autoPub = "1"
	}
	youTube := "0"
	if r.PostFormValue("youtube_enabled") == "on" {
		youTube = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}
	st := s.settings()
	if err := st.Set(SettingAutopilotEnabled, enabled); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotInterval, strconv.Itoa(hours)); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotAutoPublish, autoPub); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotYouTube, youTube); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	msg := "Đã tắt lịch autopilot."
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch autopilot — chạy mỗi %d giờ cho các account đã sẵn sàng.", hours)
	}
	if autoPub == "1" {
		msg += " Video xong sẽ tự đăng TikTok (dạng nháp)."
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape(msg))
}

// handleAutopilotMusicUpload replaces the autopilot music bed from the UI.
// Stored next to the database; the scheduler picks it up automatically.
func (s *Server) handleAutopilotMusicUpload(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "upload music")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20) // 25 MB cap
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "File nhạc quá lớn hoặc lỗi upload (tối đa 25MB).",
		))
		return
	}
	f, hdr, err := r.FormFile("music")
	if err != nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "Chưa chọn file nhạc.",
		))
		return
	}
	defer f.Close()
	if !audioFileRe.MatchString(hdr.Filename) {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "Chỉ nhận file nhạc mp3/m4a/wav/ogg/aac.",
		))
		return
	}
	dst := autopilotMusicPath(s.Cfg.DatabasePath)
	if dir := filepath.Dir(dst); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			s.fail(w, err, "upload music")
			return
		}
	}
	out, err := os.Create(dst)
	if err != nil {
		s.fail(w, err, "upload music")
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		_ = os.Remove(dst)
		s.fail(w, err, "upload music")
		return
	}
	out.Close()
	_ = s.settings().Set(SettingAutopilotMusic, hdr.Filename)
	if s.Autopilot != nil {
		s.Autopilot.MusicPath = dst
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape("Đã cập nhật nhạc nền autopilot: "+hdr.Filename))
}

var audioFileRe = regexp.MustCompile(`(?i)\.(mp3|m4a|wav|ogg|aac)$`)

// The autopilot background schedule is managed in the web UI (not via env
// or CLI). R2-W4: these keys live in the single settings facade (ledger
// settings table) — see internal/automation. Exported so cmd/aicos (the
// scheduler wiring) and internal/automation read the same keys the UI
// writes.
const (
	SettingAutopilotEnabled     = "autopilot_enabled"
	SettingAutopilotInterval    = "autopilot_interval_hours"
	SettingAutopilotLastRun     = "autopilot_last_run"
	SettingAutopilotMusic       = "autopilot_music_name"
	SettingAutopilotAutoPublish = "autopilot_auto_publish"
	SettingAutopilotYouTube     = "autopilot_youtube_enabled"
)

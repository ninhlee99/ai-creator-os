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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

type accesstradeSearchProvider struct {
	client *accesstrade.Client
	store  *products.Store
}

func (p accesstradeSearchProvider) Name() string { return accesstrade.HunterSource }

func (p accesstradeSearchProvider) Search(ctx context.Context, q products.Query) ([]products.Product, error) {
	if _, err := p.client.Hunt(ctx, p.store, accesstrade.HunterFilter{
		Limit: 200, MinCommissionRate: q.MinCommission, ThemeFallback: q.Theme,
		ApprovedOnly: true, MaxAgeDays: 30,
	}); err != nil {
		return nil, err
	}
	items, err := p.store.Search("", q.Theme, q.MinCommission, 500)
	if err != nil {
		return nil, err
	}
	var out []products.Product
	for _, item := range items {
		if len(q.Keywords) > 0 {
			hay := strings.ToLower(item.Title + " " + item.Category + " " + item.ShopName)
			matched := false
			for _, term := range q.Keywords {
				if strings.Contains(hay, strings.ToLower(term)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, item)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

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
// "shelf" tab is the affiliate shelf (legacy /shop route redirects here).
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	rawTab := r.URL.Query().Get("tab")
	tab := productTabName(rawTab)
	if rawTab != "" && rawTab != tab {
		q := r.URL.Query()
		q.Set("tab", tab)
		u := *r.URL
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
		return
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
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	minPct := resolveMinCommission(theme, r.URL.Query().Get("min"))
	var results []products.Product
	var rerr error
	if keyword != "" || theme != "" {
		results, rerr = s.Products.Search(keyword, theme, minPct/100, 30)
	} else {
		results, rerr = s.Products.TopHighCommission(30)
	}
	results = excludeSource(results, "market_verified")
	if rerr != nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Tab", tab,
			"Themes", themeOptions(),
			"Theme", theme,
			"Keyword", keyword,
			"MinPct", minPct,
			"Shelf", s.shelfViews(),
			"Error", "Không đọc được kho sản phẩm: "+rerr.Error(),
		))
		return
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
		"Keyword", keyword,
		"MinPct", minPct,
		"Results", results,
		"Shelf", s.shelfViews(),
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
		"SchedEnabled", enabled,
		"SchedHours", hours,
		"SchedLastRun", lastRun,
		"SchedNextRun", nextRun,
		"StockCount", stock,
		"MusicName", music,
		"AutoPublish", autoPub,
		"YouTubeEnabled", youTube,
		"MetaEnabled", automation.AutopilotMetaEnabled(s.settings()),
	))
}

func productTabName(tab string) string {
	switch tab {
	case "shelf", "ke":
		return "shelf"
	case "", "discover", "tim":
		return "discover"
	default:
		return "discover"
	}
}

// productView is the template projection of one shelf product (Kệ hàng).
type productView struct {
	ID              int64
	Title           string
	PlatformPID     string
	Category        string
	Price           float64
	CommissionRate  float64
	CommissionValue float64
	Score           float64
	Status          string
	ClickCount      int64
	ImageURL        string
	ProductURL      string
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
		if p.Source == "market_verified" {
			continue
		}
		img := ""
		if len(p.ImageURLs) > 0 {
			img = p.ImageURLs[0]
		}
		views = append(views, productView{
			ID:              p.ID,
			Title:           p.Title,
			PlatformPID:     p.SourceID,
			Category:        p.Category,
			Price:           p.Price,
			CommissionRate:  p.CommissionRate,
			CommissionValue: math.Round(p.Price*p.CommissionRate*100) / 100,
			Score:           products.Score(p),
			Status:          p.ShelfStatus,
			ClickCount:      p.ClickCount,
			ImageURL:        img,
			ProductURL:      p.ProductURL,
		})
	}
	return views
}

func excludeSource(items []products.Product, source string) []products.Product {
	out := items[:0]
	for _, p := range items {
		if p.Source != source {
			out = append(out, p)
		}
	}
	return out
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

// parseBounded parses an optional form number: empty → 0 (ok); otherwise
// the value must be a valid number within [0, max]. Used to validate
// product input server-side (Đợt Y) — client-side min/max is not enough.
func parseBounded(s string, max float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || v > max {
		return 0, false
	}
	return v, true
}

// productErr redirects back to the product page with a visible error
// (Đợt Y: ?err= is now rendered by handleProducts — no more silent fails).
func productErr(w http.ResponseWriter, r *http.Request, tab, msg string) {
	u := "/products?err=" + url.QueryEscape(msg)
	if tab != "" {
		u = "/products?tab=" + tab + "&err=" + url.QueryEscape(msg)
	}
	seeOther(w, r, u)
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
	price, ok := parseBounded(r.PostFormValue("price"), 1e12)
	if !ok {
		productErr(w, r, "shelf", "Giá không hợp lệ — nhập số ≥ 0 (đơn vị VND).")
		return
	}
	rate, ok := parseBounded(r.PostFormValue("commission_pct"), 100)
	if !ok {
		productErr(w, r, "shelf", "Hoa hồng không hợp lệ — nhập số từ 0 đến 100 (%).")
		return
	}
	if pid == "" || title == "" {
		productErr(w, r, "shelf", "Thiếu mã hoặc tên sản phẩm.")
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
		Price:    price, Currency: "VND", CommissionRate: rate / 100, Theme: theme,
	}
	id, err := s.Products.Save(p)
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không lưu được sản phẩm: "+err.Error()))
		return
	}
	if err := s.Products.SetShelf(id, "shelf", 0); err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Đã lưu sản phẩm nhưng không chuyển được vào kệ: "+err.Error()))
		return
	}
	seeOther(w, r, "/products?tab=shelf")
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
		productErr(w, r, "", "Thiếu tên sản phẩm hoặc chủ đề.")
		return
	}
	price, ok := parseBounded(r.PostFormValue("price"), 1e12)
	if !ok {
		productErr(w, r, "", "Giá không hợp lệ — nhập số ≥ 0 (đơn vị VND).")
		return
	}
	commPct, ok := parseBounded(r.PostFormValue("commission_pct"), 100)
	if !ok {
		productErr(w, r, "", "Hoa hồng không hợp lệ — nhập số từ 0 đến 100 (%).")
		return
	}
	rating, ok := parseBounded(r.PostFormValue("rating"), 5)
	if !ok {
		productErr(w, r, "", "Đánh giá không hợp lệ — nhập số từ 0 đến 5.")
		return
	}
	var sold int64
	fmt.Sscanf(strings.TrimSpace(r.PostFormValue("sold")), "%d", &sold)
	if sold < 0 {
		sold = 0
	}
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

// handleProductsSearch runs the configured providers for a keyword/theme, saves
// every hit into the product store, then displays the best matches. Provider
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
	keyword := strings.TrimSpace(r.PostFormValue("keyword"))
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	minPct := resolveMinCommission(theme, r.PostFormValue("min_commission"))
	minComm := minPct / 100

	providers := s.ProductProviders
	if len(providers) == 0 {
		client, err := s.atClient()
		if err != nil {
			s.render(w, "products", s.ctx(
				"AT", s.atPageData(), "Tab", "discover", "Themes", themeOptions(),
				"Theme", theme, "Keyword", keyword, "MinPct", minPct, "Shelf", s.shelfViews(),
				"Error", "Kết nối Accesstrade để tìm deal thật; hoặc thêm sản phẩm thủ công.",
			))
			return
		}
		providers = []products.Provider{accesstradeSearchProvider{client: client, store: s.Products}}
	}
	var warnings []string
	for _, p := range providers {
		if c, ok := p.(interface{ Configured() bool }); ok && !c.Configured() {
			warnings = append(warnings,
				fmt.Sprintf("Nguồn %q chưa cấu hình (thiếu API key) — đã bỏ qua.", p.Name()))
		}
	}
	var keywords []string
	if keyword != "" {
		keywords = append(keywords, keyword)
	}
	q := products.Query{
		Theme:         theme,
		Keywords:      keywords,
		MinCommission: minComm,
		Limit:         50,
	}
	if theme != "" && len(keywords) == 0 {
		q = products.QueryForTheme(theme, minComm, 50)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	found, errs := products.Aggregate(ctx, providers, q)
	for _, e := range errs {
		warnings = append(warnings, e.Error())
	}
	saved := 0
	for _, p := range found {
		if p.Theme == "" {
			p.Theme = theme
		}
		if _, err := s.Products.Save(p); err != nil {
			log.Printf("web: save product: %v", err)
			continue
		}
		saved++
	}
	results, rerr := s.Products.Search(keyword, theme, minComm, 30)
	data := s.ctx(
		"Themes", themeOptions(),
		"Theme", theme,
		"Keyword", keyword,
		"MinPct", minPct,
		"Results", results,
		"Shelf", s.shelfViews(),
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

// handleProductsShelfToggle toggles a product on or off the affiliate shelf (Kệ hàng).
func (s *Server) handleProductsShelfToggle(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "toggle shelf")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse toggle form")
		return
	}
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if id <= 0 {
		seeOther(w, r, "/products")
		return
	}
	p, err := s.Products.GetByID(id)
	if err != nil {
		seeOther(w, r, "/products?err="+url.QueryEscape("Không tìm thấy sản phẩm"))
		return
	}
	newStatus := "shelf"
	newScore := products.Score(p)
	msg := "Đã thêm \"" + p.Title + "\" vào Kệ hàng & Bio Link!"
	if p.ShelfStatus == "shelf" || p.ShelfStatus == "scaled" {
		newStatus = ""
		newScore = 0
		msg = "Đã bỏ \"" + p.Title + "\" khỏi Kệ hàng."
	}
	if err := s.Products.SetShelf(id, newStatus, newScore); err != nil {
		seeOther(w, r, "/products?err="+url.QueryEscape("Không cập nhật được kệ: "+err.Error()))
		return
	}
	tab := r.PostFormValue("tab")
	u := "/products?ok=" + url.QueryEscape(msg)
	if tab != "" {
		u = "/products?tab=" + tab + "&ok=" + url.QueryEscape(msg)
	}
	seeOther(w, r, u)
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
	meta := "0"
	if r.PostFormValue("meta_enabled") == "on" {
		meta = "1"
	}
	if err := st.Set(SettingAutopilotMeta, meta); err != nil {
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
	SettingAutopilotMeta        = "autopilot_meta_enabled"
)

// BioProductView represents a product item displayed on the public bio link storefront.
type BioProductView struct {
	ID             int64
	Title          string
	ImageURL       string
	Price          float64
	PriceFormatted string
	OriginalPrice  string
	DiscountBadge  string
	Rating         float64
	SoldCount      int64
	Category       string
	BuyURL         string
	ShopName       string
	ClickCount     int64
}

// BioLinkData represents the data passed to bio.html.
type BioLinkData struct {
	ChannelTitle   string
	ChannelHandle  string
	AvatarLetter   string
	Niche          string
	BioDescription string
	TotalProducts  int
	Products       []BioProductView
	Categories     []string
	CurrentChannel string
	ShareURL       string
	TikTokURL      string
	YouTubeURL     string
	FacebookURL    string
	ZaloURL        string
}

// handleBioLink serves the mobile-first public Bio Link storefront (/bio and /bio/{channel}).
func (s *Server) handleBioLink(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	if channel == "" {
		channel = strings.TrimPrefix(r.URL.Query().Get("channel"), "@")
	}

	channelTitle := "Kênh"
	channelHandle := ""
	niche := ""
	bioDesc := ""
	avatarLetter := "?"
	youtubeURL := ""
	accountFound := false

	if s.Ledger != nil {
		accs, err := s.Ledger.ListAccounts(nil)
		if err == nil && len(accs) > 0 {
			var matched *ledger.Account
			if channel != "" {
				for i := range accs {
					if strings.EqualFold(accs[i].Username, channel) {
						matched = &accs[i]
						break
					}
				}
			}
			if matched == nil && channel == "" {
				matched = &accs[0]
			}
			if matched != nil {
				accountFound = true
				channelHandle = matched.Username
				channelTitle = "@" + matched.Username
				if len(matched.Username) > 0 {
					avatarLetter = strings.ToUpper(matched.Username[:1])
				}
				if matched.Niche.Valid && matched.Niche.String != "" {
					niche = matched.Niche.String
				}
				if matched.YoutubeChannel.Valid && matched.YoutubeChannel.String != "" {
					youtubeURL = matched.YoutubeChannel.String
					if !strings.HasPrefix(strings.ToLower(youtubeURL), "http://") && !strings.HasPrefix(strings.ToLower(youtubeURL), "https://") {
						youtubeURL = "https://www.youtube.com/channel/" + url.PathEscape(youtubeURL)
					}
				}
			} else if channel != "" {
				http.NotFound(w, r)
				return
			}
		}
	} else if channel != "" {
		http.NotFound(w, r)
		return
	}
	if channel != "" && !accountFound {
		http.NotFound(w, r)
		return
	}

	// Fetch shelf products for a configured channel only.
	var prods []products.Product
	if accountFound && s.Products != nil {
		prods, _ = s.Products.Shelf(50)
		if len(prods) == 0 {
			prods, _ = s.Products.All(50)
		}
		prods = excludeSource(prods, "market_verified")
	}

	var items []BioProductView
	var catMap = make(map[string]bool)

	if len(prods) > 0 {
		for _, p := range prods {
			img := ""
			if len(p.ImageURLs) > 0 {
				img = p.ImageURLs[0]
			}
			cat := p.Category
			if cat == "" {
				cat = "Sản phẩm nổi bật"
			}
			catMap[cat] = true

			buyURL := "/bio-go/" + strconv.FormatInt(p.ID, 10)
			if p.ProductURL != "" && strings.HasPrefix(p.ProductURL, "http") {
				buyURL = "/bio-go/" + strconv.FormatInt(p.ID, 10)
			}

			items = append(items, BioProductView{
				ID:             p.ID,
				Title:          p.Title,
				ImageURL:       img,
				Price:          p.Price,
				PriceFormatted: formatVND(p.Price),
				Rating:         p.Rating,
				SoldCount:      p.SoldCount,
				Category:       cat,
				BuyURL:         buyURL,
				ShopName:       p.ShopName,
				ClickCount:     p.ClickCount,
			})
		}
	}

	var cats []string
	for c := range catMap {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	t, ok := s.templates["bio"]
	if !ok {
		http.Error(w, "Bio template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := BioLinkData{
		ChannelTitle:   channelTitle,
		ChannelHandle:  channelHandle,
		AvatarLetter:   avatarLetter,
		Niche:          niche,
		BioDescription: bioDesc,
		TotalProducts:  len(items),
		Products:       items,
		Categories:     cats,
		CurrentChannel: channelHandle,
		ShareURL:       r.Host + r.URL.Path,
		YouTubeURL:     youtubeURL,
	}
	if err := t.ExecuteTemplate(w, "bio", data); err != nil {
		log.Printf("web: render bio: %v", err)
	}
}

// handleBioRedirect redirects /bio-go/{id} to the target affiliate/product URL while recording clicks.
func (s *Server) handleBioRedirect(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id > 0 && s.Products != nil {
		_ = s.Products.RecordClick(id)
		p, err := s.Products.GetByID(id)
		if err == nil && p.ProductURL != "" && strings.HasPrefix(p.ProductURL, "http") {
			http.Redirect(w, r, p.ProductURL, http.StatusFound)
			return
		}
	}
	// Fallback to bio
	http.Redirect(w, r, "/bio", http.StatusFound)
}

// handleBioPin pins a product straight onto the bio storefront from Studio or Viral generators.
func (s *Server) handleBioPin(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho sản phẩm chưa khởi tạo"})
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu tên sản phẩm"})
		return
	}
	productURL := strings.TrimSpace(r.FormValue("product_url"))
	imageURL := strings.TrimSpace(r.FormValue("image_url"))
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		category = "Đồ Chọn Lọc"
	}
	price, _ := parseBounded(r.FormValue("price"), 1e12)
	if price <= 0 {
		price = 199000
	}

	p := products.Product{
		Source:         "manual",
		SourceID:       fmt.Sprintf("pin-%d", time.Now().UnixNano()%1000000),
		Title:          title,
		Category:       category,
		Price:          price,
		Currency:       "VND",
		CommissionRate: 0.15,
		ProductURL:     productURL,
		ShelfStatus:    "shelf",
	}
	if imageURL != "" {
		p.ImageURLs = []string{imageURL}
	}
	id, err := s.Products.Save(p)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Lỗi lưu: " + err.Error()})
		return
	}
	_ = s.Products.SetShelf(id, "shelf", 0)

	s.writeJSON(w, map[string]any{
		"ok":      true,
		"id":      id,
		"message": "✓ Đã ghim sản phẩm vào Kệ hàng & Bio Storefront thành công!",
		"bio_url": "/bio",
	})
}

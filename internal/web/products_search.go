package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

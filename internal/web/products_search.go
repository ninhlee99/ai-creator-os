package web

// Affiliate product catalog: discovery, manual add, shelf, and scheduling.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// handleProductsAdd saves a manually-entered product into the catalog. Manual
// sales and rating values remain user-reported and cannot pass the automated
// affiliate production gate.
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
	originalPrice, ok := parseBounded(r.PostFormValue("original_price"), 1e12)
	if !ok {
		productErr(w, r, "", "Giá gốc không hợp lệ — nhập số ≥ 0 (đơn vị VND).")
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
	sold := int64(0)
	if rawSold := strings.TrimSpace(r.PostFormValue("sold")); rawSold != "" {
		var err error
		sold, err = strconv.ParseInt(rawSold, 10, 64)
		if err != nil || sold < 0 {
			productErr(w, r, "", "Số đã bán không hợp lệ — nhập số nguyên từ 0 trở lên.")
			return
		}
	}
	evidenceURL := strings.TrimSpace(r.PostFormValue("demand_evidence_url"))
	capturedAt := time.Now().UTC()
	if rating > 0 || sold > 0 {
		evidence, err := url.Parse(evidenceURL)
		if err != nil || evidence.Scheme != "https" || evidence.Hostname() == "" || evidence.User != nil {
			productErr(w, r, "", "Rating hoặc số đã bán cần link HTTPS tới trang đang hiển thị các số liệu đó.")
			return
		}
	}
	sourceID := fmt.Sprintf("manual-%d", capturedAt.UnixNano())
	if evidenceURL != "" {
		digest := sha256.Sum256([]byte(evidenceURL))
		sourceID = "manual-evidence-" + hex.EncodeToString(digest[:16])
	}
	var imgs []string
	for _, u := range strings.Split(r.PostFormValue("image_url"), "\n") {
		if u = strings.TrimSpace(u); u != "" {
			imgs = append(imgs, u)
		}
	}
	p := products.Product{
		Source:         "manual",
		SourceID:       sourceID,
		Title:          title,
		Theme:          theme,
		ShopName:       strings.TrimSpace(r.PostFormValue("shop")),
		Price:          price,
		PriceMax:       price,
		OriginalPrice:  originalPrice,
		Currency:       "VND",
		CommissionRate: commPct / 100,
		Rating:         rating,
		SoldCount:      sold, SoldCountKnown: strings.TrimSpace(r.PostFormValue("sold")) != "",
		DemandEvidenceURL: evidenceURL,
		ImageURLs:         imgs,
		ProductURL:        strings.TrimSpace(r.PostFormValue("product_url")),
	}
	if evidenceURL != "" {
		p.DemandEvidenceAt = capturedAt.Format(time.RFC3339Nano)
	}
	if _, err := s.Products.Save(p); err != nil {
		s.fail(w, err, "save product")
		return
	}
	seeOther(w, r, "/products?theme="+url.QueryEscape(theme)+"&ok="+url.QueryEscape("Đã lưu sản phẩm \""+title+"\" trong catalog. Số liệu rating/sales nhập tay chưa xác minh nên không đủ điều kiện tạo video affiliate tự động."))
}

// handleProductsSearch runs the configured providers for a keyword/theme, saves
// every hit into the product store, then displays the best matches. Provider
// problems are shown honestly instead of hidden.
func (s *Server) handleProductsSearch(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.renderProductResults(w, r, s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"SearchError", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
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
			s.renderProductResults(w, r, s.ctx(
				"AT", s.atPageData(), "Tab", "discover", "Themes", themeOptions(),
				"Theme", theme, "Keyword", keyword, "MinPct", minPct, "Shelf", s.shelfViews(),
				"SearchError", "Kết nối Accesstrade để tìm deal thật; hoặc thêm sản phẩm thủ công.",
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
	results = s.attachTikTokRadarEvidence(r.Context(), results)
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
		data["SearchError"] = "Không đọc được kho sản phẩm: " + rerr.Error()
	}
	data["AT"] = s.atPageData()
	s.renderProductResults(w, r, data)
}

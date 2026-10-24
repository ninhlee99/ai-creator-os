package web

// Affiliate product catalog: discovery, manual add, shelf, and scheduling.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ledgerShelfMigrationKey marks the one-time import of the legacy ledger
// products table into the shared store.
const ledgerShelfMigrationKey = "migration.ledger_products_v1"

// MigrateLedgerShelf imports the legacy ledger products table (the Kệ
// hàng tab's old home) into the shared product store — once, idempotently.
// Shelf/scaled rows keep their shelf status + score; discovered/candidate
// rows become plain store products. The ledger table itself is left
// untouched for active affiliate campaign history.
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
	price, ok := parseBounded(r.PostFormValue("price"), 1e12)
	if !ok {
		productErr(w, r, "shelf", "Giá không hợp lệ — nhập số ≥ 0 (đơn vị VND).")
		return
	}
	originalPrice, ok := parseBounded(r.PostFormValue("original_price"), 1e12)
	if !ok {
		productErr(w, r, "shelf", "Giá gốc không hợp lệ — nhập số ≥ 0 (đơn vị VND).")
		return
	}
	rate, ok := parseBounded(r.PostFormValue("commission_pct"), 100)
	if !ok {
		productErr(w, r, "shelf", "Hoa hồng không hợp lệ — nhập số từ 0 đến 100 (%).")
		return
	}
	if price <= 0 || rate <= 0 {
		productErr(w, r, "shelf", "Duyệt sản phẩm cần giá bán và hoa hồng lớn hơn 0.")
		return
	}
	productURL := strings.TrimSpace(r.PostFormValue("product_url"))
	parsedProductURL, urlErr := url.Parse(productURL)
	if urlErr != nil || parsedProductURL.Scheme != "https" || parsedProductURL.Hostname() == "" || parsedProductURL.User != nil {
		productErr(w, r, "shelf", "Cần link affiliate HTTPS hợp lệ trước khi thêm sản phẩm vào kệ.")
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
		Price:    price, PriceMax: price, OriginalPrice: originalPrice, Currency: "VND",
		CommissionRate: rate / 100, Theme: theme, ProductURL: productURL,
	}
	id, err := s.Products.Save(p)
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không lưu được sản phẩm: "+err.Error()))
		return
	}
	if err := s.Products.ReviewProduct(id, products.ReviewApproved, "Duyệt thủ công"); err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Đã lưu sản phẩm nhưng không chuyển được vào kệ: "+err.Error()))
		return
	}
	seeOther(w, r, "/products?tab=shelf")
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
	msg := "Đã thêm \"" + p.Title + "\" vào danh sách sản phẩm affiliate!"
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

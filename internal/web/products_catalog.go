package web

// Affiliate product catalog: discovery, manual add, shelf, and scheduling.

import (
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
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
		s.renderProductResults(w, r, s.ctx(
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
	if tab == "affiliate" {
		// Affiliate operations have their own tab; do not load product candidates.
	} else if tab == "pending" {
		results, rerr = s.Products.ReviewList(products.ReviewPending, keyword, theme, 100)
	} else if tab == "rejected" {
		results, rerr = s.Products.ReviewList(products.ReviewRejected, keyword, theme, 50)
	} else if keyword != "" || theme != "" {
		results, rerr = s.Products.Search(keyword, theme, minPct/100, 30)
	} else {
		results, rerr = s.Products.TopHighCommission(30)
	}
	results = excludeSource(results, "market_verified")
	if rerr != nil {
		s.renderProductResults(w, r, s.ctx(
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
	results = s.attachProductSalesChange(results)
	results = s.attachTikTokRadarEvidence(r.Context(), results)
	s.renderProductResults(w, r, s.ctx(
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
	))
}

func (s *Server) renderProductResults(w http.ResponseWriter, r *http.Request, data map[string]any) {
	if r.Header.Get("HX-Request") == "true" {
		t := s.templates["products"]
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := t.ExecuteTemplate(w, "product-results", data); err != nil {
			log.Printf("web: render product results: %v", err)
			http.Error(w, "Không hiển thị được kết quả sản phẩm", http.StatusInternalServerError)
		}
		return
	}
	s.render(w, "products", data)
}

func productTabName(tab string) string {
	switch tab {
	case "shelf", "ke":
		return "shelf"
	case "affiliate", "tracking":
		return "affiliate"
	case "rejected", "removed":
		return "rejected"
	case "pending", "review":
		return "pending"
	case "", "discover", "tim":
		return "discover"
	default:
		return "discover"
	}
}

// productView is the template projection of one shelf product (Kệ hàng).
type productView struct {
	ID                 int64
	Title              string
	PlatformPID        string
	Category           string
	Price              float64
	PriceMax           float64
	CommissionRate     float64
	CommissionCurrency string
	CommissionValue    float64
	CommissionEstimate bool
	Score              float64
	Status             string
	ImageURL           string
	ProductURL         string
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
		commissionValue := p.CommissionAmount
		commissionCurrency := p.CommissionCurrency
		commissionEstimate := commissionValue <= 0
		if commissionEstimate {
			commissionValue = math.Round(p.Price*p.CommissionRate*100) / 100
			commissionCurrency = p.Currency
		}
		views = append(views, productView{
			ID:                 p.ID,
			Title:              p.Title,
			PlatformPID:        p.SourceID,
			Category:           p.Category,
			Price:              p.Price,
			PriceMax:           p.PriceMax,
			CommissionRate:     p.CommissionRate,
			CommissionCurrency: commissionCurrency,
			CommissionValue:    commissionValue,
			CommissionEstimate: commissionEstimate,
			Score:              products.Score(p),
			Status:             p.ShelfStatus,
			ImageURL:           img,
			ProductURL:         p.ProductURL,
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

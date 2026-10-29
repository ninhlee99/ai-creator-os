package web

// Affiliate product catalog: shelf and scheduling.

import (
	"log"
	"math"
	"net/http"

	"github.com/ninhlee99/ai-creator-os/internal/network"
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

// handleProducts renders the affiliate product page: 1-link import and shelf products.
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.renderProductResults(w, r, s.ctx(
			"Shelf", s.shelfViews(),
			"Error", "Kho sản phẩm chưa được khởi tạo. Hãy kiểm tra cấu hình app rồi thử lại.",
		))
		return
	}

	var availableWorkflows []WorkflowOption
	if s.Studio != nil {
		if fs, err := s.Studio.GetFlowStore(); err == nil && fs != nil {
			if list, err := fs.ListGraphs(); err == nil {
				for _, sum := range list {
					availableWorkflows = append(availableWorkflows, WorkflowOption{
						ID:          sum.ID,
						Title:       sum.Name,
						Category:    sum.Category,
						Description: sum.Description,
						IsPreset:    sum.IsPreset,
					})
				}
			}
		}
	}
	var accountOptions []*network.Account
	if s.Mgr != nil {
		if accs, err := s.Mgr.ListWithAutopilot(); err == nil {
			accountOptions = accs
		}
	}

	s.renderProductResults(w, r, s.ctx(
		"Shelf", s.shelfViews(),
		"AvailableModels", s.listModelPhotoCards(),
		"AccountOptions", accountOptions,
		"AvailableWorkflows", availableWorkflows,
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

// productView is the template projection of one shelf product (Kệ hàng).
type productView struct {
	ID                 int64
	Title              string
	PlatformPID        string
	Category           string
	Price              float64
	PriceMax           float64
	OriginalPrice      float64
	ShopName           string
	Rating             float64
	SoldCount          int64
	CommissionRate     float64
	CommissionCurrency string
	CommissionValue    float64
	CommissionEstimate bool
	Score              float64
	Status             string
	ImageURL           string
	ImageURLs          []string
	ProductURL         string
}

// shelfViews projects the shared product store's shelf (status
// shelf/scaled) for the "Kệ hàng" tab — the products AI gắn vào
// video/live.
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
			OriginalPrice:      p.OriginalPrice,
			ShopName:           p.ShopName,
			Rating:             p.Rating,
			SoldCount:          p.SoldCount,
			CommissionRate:     p.CommissionRate,
			CommissionCurrency: commissionCurrency,
			CommissionValue:    commissionValue,
			CommissionEstimate: commissionEstimate,
			Score:              products.Score(p),
			Status:             p.ShelfStatus,
			ImageURL:           img,
			ImageURLs:          p.ImageURLs,
			ProductURL:         p.ProductURL,
		})
	}
	return views
}

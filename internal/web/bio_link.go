package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
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

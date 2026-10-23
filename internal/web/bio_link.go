package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BioProductView represents a product item displayed on the public bio link storefront.
type BioProductView struct {
	ID                    int64
	Title                 string
	ImageURLs             []string
	Price                 float64
	PriceFormatted        string
	OriginalPrice         string
	DiscountBadge         string
	Rating                float64
	SoldCount             int64
	Category              string
	BuyURL                string
	DetailURL             string
	VoucherCopyURL        string
	ShopName              string
	VoucherCode           string
	VoucherAvailable      bool
	VoucherValue          float64
	VoucherValueFormatted string
	VoucherEndsAt         string
	VoucherTerms          string
	ProductDetails        string
	DataUpdatedAt         string
	PriceStale            bool
	PriceFreshnessUnknown bool
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
	ProfileURL     string
	TikTokURL      string
	YouTubeURL     string
	FacebookURL    string
	ZaloURL        string
}

type BioProductPageData struct {
	ChannelHandle     string
	ProfileURL        string
	Product           BioProductView
	TryOnReady        bool
	TryOnLoginReady   bool
	TryOnSignedIn     bool
	TryOnGoogleEmail  string
	TryOnLoginURL     string
	TryOnLogoutURL    string
	TryOnLoginMessage string
	TryOnEndpoint     string
}

// handleBioLink serves the mobile-first public Bio Link storefront (/bio and /bio/{channel}).
func (s *Server) handleBioLink(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	if channel == "" {
		channel = strings.TrimPrefix(r.URL.Query().Get("channel"), "@")
	}

	if s.Ledger == nil || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	account, profile, err := s.resolvePublicBioAccount(channel)
	if err != nil || account == nil {
		http.NotFound(w, r)
		return
	}
	listings, err := s.Products.ListBioListings(account.ID, true, 100)
	if err != nil {
		http.Error(w, "Bio tạm thời không khả dụng", http.StatusServiceUnavailable)
		return
	}

	var items []BioProductView
	var catMap = make(map[string]bool)
	for _, listing := range listings {
		p := listing.Product
		if !s.bioListingIsPublic(listing) {
			continue
		}
		cat := p.Category
		if cat == "" {
			cat = "Sản phẩm nổi bật"
		}
		catMap[cat] = true
		items = append(items, s.bioProductView(listing, account.Username, time.Now()))
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
	s.recordBioPageView(r, account.ID, 0, "profile_view", time.Now())
	niche := account.Niche.String
	if niche == "" {
		niche = account.NicheHint.String
	}
	description := profile.Description
	if description == "" {
		description = account.NicheHint.String
	}
	data := BioLinkData{
		ChannelTitle:   "@" + account.Username,
		ChannelHandle:  account.Username,
		AvatarLetter:   firstBioLetter(account.Username),
		Niche:          niche,
		BioDescription: description,
		TotalProducts:  len(items),
		Products:       items,
		Categories:     cats,
		CurrentChannel: account.Username,
		ShareURL:       r.Host + r.URL.Path,
		ProfileURL:     "/bio/" + url.PathEscape(account.Username),
		YouTubeURL:     bioYouTubeURL(account.YoutubeChannel.String),
	}
	if err := t.ExecuteTemplate(w, "bio", data); err != nil {
		log.Printf("web: render bio: %v", err)
	}
}

func (s *Server) bioProductView(listing products.BioListing, channel string, now time.Time) BioProductView {
	p := listing.Product
	originalPrice := ""
	discountBadge := ""
	if p.OriginalPrice > p.Price && p.Price > 0 {
		originalPrice = formatBioMoney(p.OriginalPrice, p.Currency)
		discountBadge = fmt.Sprintf("-%d%%", int(math.Round((p.OriginalPrice-p.Price)/p.OriginalPrice*100)))
	}
	voucherCode, voucherValue, voucherEndsAt, voucherTerms := activeBioVoucher(listing, now)
	images := s.bioPublicProductImages(listing, channel)
	category := p.Category
	if category == "" {
		category = "Sản phẩm nổi bật"
	}
	rating, soldCount := 0.0, int64(0)
	if p.HasDemandSignalsAt(now) {
		rating, soldCount = p.Rating, p.SoldCount
	}
	return BioProductView{
		ID: p.ID, Title: p.Title, ImageURLs: images, Price: p.Price,
		PriceFormatted: formatBioMoney(p.Price, p.Currency), OriginalPrice: originalPrice,
		DiscountBadge: discountBadge, Rating: rating, SoldCount: soldCount,
		Category: category, BuyURL: bioBuyURL(channel, p.ID),
		DetailURL:      "/bio/" + url.PathEscape(channel) + "/product/" + strconv.FormatInt(p.ID, 10),
		VoucherCopyURL: "/bio/" + url.PathEscape(channel) + "/products/" + strconv.FormatInt(p.ID, 10) + "/voucher-copy",
		ShopName:       p.ShopName,
		VoucherCode:    voucherCode, VoucherValue: voucherValue,
		VoucherAvailable:      voucherCode != "" || voucherValue > 0,
		VoucherValueFormatted: formatBioMoney(voucherValue, p.Currency), VoucherEndsAt: voucherEndsAt,
		VoucherTerms: voucherTerms, ProductDetails: listing.ProductDetails, DataUpdatedAt: p.DataUpdatedAt,
		PriceStale:            staleBioProductPrice(p, now),
		PriceFreshnessUnknown: p.Price > 0 && strings.TrimSpace(p.DataUpdatedAt) == "",
	}
}

func (s *Server) handleBioProduct(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	account, _, err := s.resolvePublicBioAccount(channel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	listing, err := s.Products.BioListing(account.ID, id, true)
	if err != nil || !s.bioListingIsPublic(listing) {
		http.NotFound(w, r)
		return
	}
	s.recordBioPageView(r, account.ID, id, "product_view", time.Now())
	t, ok := s.templates["bio_product"]
	if !ok {
		http.Error(w, "Bio product template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	detailURL := "/bio/" + url.PathEscape(account.Username) + "/product/" + strconv.FormatInt(id, 10)
	googleAccount, signedIn := s.bioGoogleAccountForRequest(r)
	data := BioProductPageData{
		ChannelHandle:     account.Username,
		ProfileURL:        "/bio/" + url.PathEscape(account.Username),
		Product:           s.bioProductView(listing, account.Username, time.Now()),
		TryOnReady:        s.bioTryOnReady(r.Context()) && s.bioTryOnHasReference(listing),
		TryOnLoginReady:   bioGoogleLoginReady(r),
		TryOnSignedIn:     signedIn,
		TryOnLoginURL:     "/bio/google/login?return=" + url.QueryEscape(detailURL),
		TryOnLogoutURL:    "/bio/google/logout?return=" + url.QueryEscape(detailURL),
		TryOnLoginMessage: bioGoogleLoginMessage(r.URL.Query().Get("google_login")),
		TryOnEndpoint:     "/bio/" + url.PathEscape(account.Username) + "/products/" + strconv.FormatInt(id, 10) + "/try-on",
	}
	if signedIn {
		data.TryOnGoogleEmail = googleAccount.Email
	}
	if err := t.ExecuteTemplate(w, "bio_product", data); err != nil {
		log.Printf("web: render bio product: %v", err)
	}
}

func (s *Server) handleBioVoucherCopy(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id <= 0 || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	account, _, err := s.resolvePublicBioAccount(channel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	listing, err := s.Products.BioListing(account.ID, id, true)
	if err != nil || !s.bioListingIsPublic(listing) {
		http.NotFound(w, r)
		return
	}
	code, value, _, _ := activeBioVoucher(listing, time.Now())
	if code == "" {
		http.NotFound(w, r)
		return
	}
	s.recordBioInteraction(r, account.ID, id, "voucher_copy", time.Now())
	s.writeJSON(w, map[string]any{"ok": true, "code": code, "value": value})
}

func (s *Server) resolvePublicBioAccount(channel string) (*ledger.Account, products.BioProfile, error) {
	if s.Ledger == nil || s.Products == nil {
		return nil, products.BioProfile{}, products.ErrBioProfileNotFound
	}
	var account *ledger.Account
	if channel == "" {
		accountID, err := s.Products.DefaultBioAccountID()
		if err != nil {
			return nil, products.BioProfile{}, err
		}
		account, err = s.Ledger.GetAccount(accountID)
		if err != nil {
			return nil, products.BioProfile{}, err
		}
	} else {
		accounts, err := s.Ledger.ListAccounts(nil)
		if err != nil {
			return nil, products.BioProfile{}, err
		}
		for i := range accounts {
			if strings.EqualFold(accounts[i].Username, channel) {
				account = &accounts[i]
				break
			}
		}
		if account == nil {
			return nil, products.BioProfile{}, products.ErrBioProfileNotFound
		}
	}
	profile, err := s.Products.GetBioProfile(account.ID)
	if err != nil || !profile.Published {
		return nil, products.BioProfile{}, products.ErrBioProfileNotFound
	}
	return account, profile, nil
}

func activeBioVoucher(listing products.BioListing, now time.Time) (code string, value float64, endsAt, terms string) {
	code = strings.TrimSpace(listing.VoucherCode)
	value = listing.VoucherValue
	endsAt = strings.TrimSpace(listing.VoucherEndsAt)
	terms = strings.TrimSpace(listing.VoucherTerms)
	if code == "" && value <= 0 {
		return "", 0, "", ""
	}
	if endsAt != "" {
		end, err := parseBioOfferTime(endsAt)
		if err != nil || !now.Before(end) {
			return "", 0, "", ""
		}
	}
	return code, value, endsAt, terms
}

func (s *Server) bioListingIsPublic(listing products.BioListing) bool {
	return listing.Product.Source != "market_verified" &&
		s.bioProductPublishBlockedReason(listing.Product, listing.ImageURLs) == ""
}

func (s *Server) bioProductPublishBlockedReason(product products.Product, imageOverrides []string) string {
	imageURLs := product.ImageURLs
	if len(imageOverrides) > 0 {
		imageURLs = imageOverrides
	}
	if !s.hasBioProductImage(imageURLs) {
		return "Cần ảnh sản phẩm thật có URL HTTP(S) hoặc file JPEG/PNG hợp lệ trước khi đăng Bio"
	}
	if safeBioExternalURL(product.ProductURL) == "" {
		return "Cần link mua hàng HTTP(S) hợp lệ trước khi đăng Bio"
	}
	return ""
}

func (s *Server) hasBioProductImage(rawURLs []string) bool {
	for _, raw := range rawURLs {
		if safeBioExternalURL(raw) != "" {
			return true
		}
		if _, ok := parseBioProductImageMarker(raw); ok && s.bioProductImageExists(raw) {
			return true
		}
	}
	return false
}

func (s *Server) bioPublicProductImages(listing products.BioListing, channel string) []string {
	rawImages := listing.Product.ImageURLs
	if len(listing.ImageURLs) > 0 {
		rawImages = listing.ImageURLs
	}
	images := make([]string, 0, len(rawImages))
	for _, raw := range rawImages {
		if imageURL := safeBioExternalURL(raw); imageURL != "" {
			images = append(images, imageURL)
		} else if file, ok := parseBioProductImageMarker(raw); ok && s.bioProductImageExists(raw) {
			images = append(images, "/bio/"+url.PathEscape(channel)+"/product/"+
				strconv.FormatInt(listing.Product.ID, 10)+"/image/"+url.PathEscape(file))
		}
		if len(images) == 8 {
			break
		}
	}
	return images
}

func safeBioProductImageURLs(rawURLs []string) []string {
	images := make([]string, 0, len(rawURLs))
	for _, raw := range rawURLs {
		if imageURL := safeBioExternalURL(raw); imageURL != "" {
			images = append(images, imageURL)
			if len(images) == 8 {
				break
			}
		}
	}
	return images
}

func formatBioMoney(amount float64, currency string) string {
	code := normalizedBioCurrency(currency)
	if code == "VND" {
		return formatVND(amount)
	}
	if amount == 0 {
		return ""
	}
	number := strconv.FormatFloat(math.Abs(amount), 'f', -1, 64)
	parts := strings.SplitN(number, ".", 2)
	var grouped strings.Builder
	for i, digit := range parts[0] {
		if i > 0 && (len(parts[0])-i)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	if len(parts) == 2 {
		grouped.WriteByte(',')
		grouped.WriteString(parts[1])
	}
	formatted := grouped.String()
	if amount < 0 {
		formatted = "-" + formatted
	}
	if code == "" {
		return formatted + " · chưa rõ tiền tệ"
	}
	return formatted + " " + code
}

func bioCurrencyLabel(currency string) string {
	if code := normalizedBioCurrency(currency); code != "" {
		return code
	}
	return "chưa rõ tiền tệ"
}

func normalizedBioCurrency(currency string) string {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if len(code) != 3 || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return ""
	}
	return code
}

func parseBioOfferTime(raw string) (time.Time, error) {
	if date, err := time.Parse("2006-01-02", raw); err == nil {
		loc, locErr := time.LoadLocation("Asia/Ho_Chi_Minh")
		if locErr != nil {
			loc = time.FixedZone("ICT", 7*60*60)
		}
		return time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid voucher expiry")
}

func staleBioProductPrice(p products.Product, now time.Time) bool {
	raw := strings.TrimSpace(p.DataUpdatedAt)
	if raw == "" {
		return false
	}
	updated, err := parseBioOfferTime(raw)
	if err != nil {
		if parsed, perr := time.Parse("2006-01-02T15:04:05", raw); perr == nil {
			updated = parsed
		} else {
			return true
		}
	}
	return now.Sub(updated) > 14*24*time.Hour
}

func bioBuyURL(channel string, productID int64) string {
	return "/bio/" + url.PathEscape(channel) + "/go/" + strconv.FormatInt(productID, 10)
}

func bioYouTubeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
		return raw
	}
	return "https://www.youtube.com/channel/" + url.PathEscape(raw)
}

func firstBioLetter(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return "?"
	}
	return strings.ToUpper(string(runes[0]))
}

// handleBioRedirect redirects /bio-go/{id} to the target affiliate/product URL while recording clicks.
func (s *Server) handleBioRedirect(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimPrefix(r.PathValue("channel"), "@")
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("id")
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id <= 0 || s.Products == nil {
		http.NotFound(w, r)
		return
	}
	account, _, err := s.resolvePublicBioAccount(channel)
	if err != nil {
		// Legacy /bio-go/{id} links resolve only through the configured
		// default public channel. They never fall back to a random account.
		if channel != "" {
			http.NotFound(w, r)
			return
		}
		accountID, defaultErr := s.Products.DefaultBioAccountID()
		if defaultErr != nil || s.Ledger == nil {
			http.NotFound(w, r)
			return
		}
		account, err = s.Ledger.GetAccount(accountID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	listing, err := s.Products.BioListing(account.ID, id, true)
	if err != nil || !s.bioListingIsPublic(listing) {
		http.NotFound(w, r)
		return
	}
	target := safeBioExternalURL(listing.Product.ProductURL)
	if target == "" {
		http.NotFound(w, r)
		return
	}
	if s.recordBioInteraction(r, account.ID, id, "outbound_click", time.Now()) {
		_ = s.Products.RecordClick(id)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func safeBioExternalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	return u.String()
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
	originalPrice, _ := parseBounded(r.FormValue("original_price"), 1e12)

	p := products.Product{
		Source:        "manual",
		SourceID:      fmt.Sprintf("pin-%d", time.Now().UnixNano()),
		Title:         title,
		Category:      category,
		Price:         price,
		OriginalPrice: originalPrice,
		Currency:      "VND",
		ProductURL:    productURL,
		ShelfStatus:   "shelf",
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
		"message": "Đã thêm vào Kệ sản phẩm. Chọn kênh và xuất bản sản phẩm trong Quản lý Bio.",
		"bio_url": "/bio-admin",
	})
}

package web

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

type bioAdminChannelView struct {
	ID          int64
	Username    string
	Published   bool
	IsDefault   bool
	Description string
}

type bioAdminProductView struct {
	ID                     int64
	Title                  string
	ImageURL               string
	ImageURLsText          string
	ShopName               string
	FreshnessLabel         string
	PublishBlockedReason   string
	PriceKnown             bool
	PriceFormatted         string
	OriginalPriceFormatted string
	CurrencyLabel          string
	InShelf                bool
	Published              bool
	DisplayOrder           int
	VoucherCode            string
	VoucherValue           float64
	VoucherEndsAt          string
	VoucherTerms           string
	ProductDetails         string
	UploadedImages         []bioAdminUploadedImage
}

type bioAdminUploadedImage struct {
	Marker string
	URL    string
}

type bioAdminPageData struct {
	Channels              []bioAdminChannelView
	Selected              bioAdminChannelView
	Products              []bioAdminProductView
	PublicURL             string
	Error                 string
	Notice                string
	TryOnGoogleLoginReady bool
}

func (s *Server) handleBioAdmin(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil || s.Products == nil {
		http.Error(w, "Kho bio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	accounts, err := s.Ledger.ListAccounts(nil)
	if err != nil {
		http.Error(w, "Không đọc được kênh", http.StatusInternalServerError)
		return
	}
	defaultID, _ := s.Products.DefaultBioAccountID()
	channels := make([]bioAdminChannelView, 0, len(accounts))
	var selectedID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("account_id")); raw != "" {
		selectedID, _ = strconv.ParseInt(raw, 10, 64)
	}
	for _, account := range accounts {
		profile, profileErr := s.Products.GetBioProfile(account.ID)
		channel := bioAdminChannelView{ID: account.ID, Username: account.Username, IsDefault: account.ID == defaultID}
		if profileErr == nil {
			channel.Published = profile.Published
			channel.Description = profile.Description
		}
		channels = append(channels, channel)
		if selectedID == 0 && account.ID == defaultID {
			selectedID = account.ID
		}
	}
	if selectedID == 0 && len(accounts) > 0 {
		selectedID = accounts[0].ID
	}
	data := bioAdminPageData{
		Channels:              channels,
		Error:                 r.URL.Query().Get("err"),
		Notice:                r.URL.Query().Get("ok"),
		TryOnGoogleLoginReady: bioGoogleLoginReady(r),
	}
	for _, channel := range channels {
		if channel.ID == selectedID {
			data.Selected = channel
			break
		}
	}
	if data.Selected.ID > 0 {
		data.PublicURL = "/bio/" + url.PathEscape(data.Selected.Username)
		listings, listErr := s.Products.ListBioListings(selectedID, false, 500)
		if listErr != nil {
			data.Error = "Không đọc được sản phẩm đã chọn: " + listErr.Error()
		} else {
			listingByProduct := make(map[int64]products.BioListing, len(listings))
			for _, listing := range listings {
				listingByProduct[listing.Product.ID] = listing
			}
			all, allErr := s.Products.All(500)
			if allErr != nil {
				data.Error = "Không đọc được kho sản phẩm: " + allErr.Error()
			} else {
				for _, product := range all {
					if product.Source == "market_verified" {
						continue
					}
					listing, listed := listingByProduct[product.ID]
					image := s.bioAdminProductPreviewURL(product, listing)
					view := bioAdminProductView{
						ID: product.ID, Title: product.Title, ImageURL: image,
						ImageURLsText: strings.Join(safeBioProductImageURLs(listing.ImageURLs), "\n"),
						ShopName:      product.ShopName, FreshnessLabel: bioAdminFreshnessLabel(product, time.Now()),
						PublishBlockedReason: s.bioProductPublishBlockedReason(product, listing.ImageURLs),
						PriceKnown:           product.Price > 0,
						CurrencyLabel:        bioCurrencyLabel(product.Currency),
						InShelf:              product.ShelfStatus == "shelf" || product.ShelfStatus == "scaled",
						Published:            listed && listing.Published,
						ProductDetails:       listing.ProductDetails,
					}
					if view.PriceKnown {
						view.PriceFormatted = formatBioMoney(product.Price, product.Currency)
					}
					for _, raw := range listing.ImageURLs {
						if file, ok := parseBioProductImageMarker(raw); ok && s.bioProductImageExists(raw) {
							view.UploadedImages = append(view.UploadedImages, bioAdminUploadedImage{
								Marker: raw, URL: "/bio-admin/product-image/" + url.PathEscape(file),
							})
						}
					}
					if product.OriginalPrice > product.Price && product.Price > 0 {
						view.OriginalPriceFormatted = formatBioMoney(product.OriginalPrice, product.Currency)
					}
					if listed {
						view.DisplayOrder = listing.DisplayOrder
						view.VoucherCode = listing.VoucherCode
						view.VoucherValue = listing.VoucherValue
						view.VoucherEndsAt = listing.VoucherEndsAt
						view.VoucherTerms = listing.VoucherTerms
					}
					data.Products = append(data.Products, view)
				}
			}
		}
	}
	s.render(w, "bio_admin", map[string]any{
		"Channels": data.Channels, "Selected": data.Selected, "Products": data.Products,
		"PublicURL": data.PublicURL, "Error": data.Error, "Notice": data.Notice,
	})
}

func bioAdminFreshnessLabel(product products.Product, now time.Time) string {
	if strings.TrimSpace(product.DataUpdatedAt) == "" {
		if product.Source == "manual" {
			return "Nguồn thủ công · chưa có mốc cập nhật"
		}
		return "Chưa rõ thời điểm cập nhật giá"
	}
	if staleBioProductPrice(product, now) {
		return "Dữ liệu giá cũ · kiểm tra tại shop"
	}
	return "Cập nhật nguồn " + product.DataUpdatedAt
}

func (s *Server) bioAdminProductPreviewURL(product products.Product, listing products.BioListing) string {
	images := product.ImageURLs
	if len(listing.ImageURLs) > 0 {
		images = listing.ImageURLs
	}
	for _, raw := range images {
		if imageURL := safeBioExternalURL(raw); imageURL != "" {
			return imageURL
		}
		if file, ok := parseBioProductImageMarker(raw); ok && s.bioProductImageExists(raw) {
			return "/bio-admin/product-image/" + url.PathEscape(file)
		}
	}
	return ""
}

func (s *Server) handleBioAdminProfileSave(w http.ResponseWriter, r *http.Request) {
	if !parseBioAdminForm(w, r) {
		return
	}
	accountID, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("account_id")), 10, 64)
	back := bioAdminBack(accountID)
	if s.Ledger == nil || s.Products == nil || accountID <= 0 {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Kênh không hợp lệ"), http.StatusSeeOther)
		return
	}
	if _, err := s.Ledger.GetAccount(accountID); err != nil {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Không tìm thấy kênh"), http.StatusSeeOther)
		return
	}
	published := formOn(r, "published")
	makeDefault := formOn(r, "is_default")
	description := strings.TrimSpace(r.PostFormValue("description"))
	if utf8.RuneCountInString(description) > 280 {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Mô tả chỉ được dài tối đa 280 ký tự"), http.StatusSeeOther)
		return
	}
	if err := s.Products.SetBioProfile(accountID, published, description, makeDefault); err != nil {
		http.Redirect(w, r, back+"&err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back+"&ok="+url.QueryEscape("Đã lưu cấu hình bio"), http.StatusSeeOther)
}

func (s *Server) handleBioAdminProductSave(w http.ResponseWriter, r *http.Request) {
	if !parseBioAdminProductForm(w, r) {
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	accountID, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("account_id")), 10, 64)
	productID, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("product_id")), 10, 64)
	back := bioAdminBack(accountID)
	if s.Products == nil || s.Ledger == nil || accountID <= 0 || productID <= 0 {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Kênh hoặc sản phẩm không hợp lệ"), http.StatusSeeOther)
		return
	}
	if _, err := s.Ledger.GetAccount(accountID); err != nil {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Không tìm thấy kênh"), http.StatusSeeOther)
		return
	}
	product, err := s.Products.GetByID(productID)
	if err != nil || product.Source == "market_verified" {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Không tìm thấy sản phẩm hợp lệ"), http.StatusSeeOther)
		return
	}
	previous, previousErr := s.Products.BioListing(accountID, productID, false)
	if previousErr != nil && !errors.Is(previousErr, products.ErrBioProductNotFound) {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Không đọc được cấu hình ảnh hiện tại"), http.StatusSeeOther)
		return
	}
	imageURLs, imageErr := parseBioProductImageURLs(r.PostFormValue("image_urls"))
	if imageErr != nil {
		http.Redirect(w, r, back+"&err="+url.QueryEscape(imageErr.Error()), http.StatusSeeOther)
		return
	}
	previousMarkers := make(map[string]bool)
	for _, image := range previous.ImageURLs {
		if _, ok := parseBioProductImageMarker(image); ok {
			previousMarkers[image] = true
		}
	}
	keptMarkers := make(map[string]bool)
	for _, marker := range r.PostForm["keep_image_asset"] {
		if _, ok := parseBioProductImageMarker(marker); !ok || !previousMarkers[marker] || !s.bioProductImageExists(marker) {
			http.Redirect(w, r, back+"&err="+url.QueryEscape("Ảnh tải lên hiện tại không hợp lệ"), http.StatusSeeOther)
			return
		}
		if !keptMarkers[marker] {
			imageURLs = append(imageURLs, marker)
			keptMarkers[marker] = true
		}
	}
	var uploadHeader *multipart.FileHeader
	if r.MultipartForm != nil {
		files := r.MultipartForm.File["bio_product_image"]
		if len(files) > 1 {
			http.Redirect(w, r, back+"&err="+url.QueryEscape("Mỗi lần chỉ tải lên một ảnh sản phẩm"), http.StatusSeeOther)
			return
		}
		if len(files) == 1 && files[0].Size > 0 {
			uploadHeader = files[0]
		}
	}
	if len(imageURLs) > 8 || (uploadHeader != nil && len(imageURLs) >= 8) {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Mỗi sản phẩm tối đa 8 ảnh"), http.StatusSeeOther)
		return
	}
	value, ok := parseBounded(r.PostFormValue("voucher_value"), 1e12)
	if !ok {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Giá trị voucher không hợp lệ"), http.StatusSeeOther)
		return
	}
	voucherCode := strings.TrimSpace(r.PostFormValue("voucher_code"))
	voucherTerms := strings.TrimSpace(r.PostFormValue("voucher_terms"))
	productDetails := strings.TrimSpace(r.PostFormValue("product_details"))
	if utf8.RuneCountInString(voucherCode) > 80 || utf8.RuneCountInString(voucherTerms) > 200 ||
		utf8.RuneCountInString(productDetails) > 1500 {
		http.Redirect(w, r, back+"&err="+url.QueryEscape("Mã voucher tối đa 80 ký tự, điều kiện tối đa 200 ký tự, mô tả sản phẩm tối đa 1500 ký tự"), http.StatusSeeOther)
		return
	}
	order, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("display_order")))
	if order < 0 {
		order = 0
	}
	endsAt := strings.TrimSpace(r.PostFormValue("voucher_ends_at"))
	if endsAt != "" {
		if _, err := parseBioOfferTime(endsAt); err != nil {
			http.Redirect(w, r, back+"&err="+url.QueryEscape("Ngày hết hạn voucher không hợp lệ"), http.StatusSeeOther)
			return
		}
	}
	published := formOn(r, "published")
	newMarker := ""
	if uploadHeader != nil {
		newMarker, err = s.saveBioProductImage(uploadHeader)
		if err != nil {
			http.Redirect(w, r, back+"&err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		imageURLs = append(imageURLs, newMarker)
	}
	if published {
		if reason := s.bioProductPublishBlockedReason(product, imageURLs); reason != "" {
			if newMarker != "" {
				_ = s.removeBioProductImage(newMarker)
			}
			http.Redirect(w, r, back+"&err="+url.QueryEscape(reason), http.StatusSeeOther)
			return
		}
	}
	if err := s.Products.SetBioListingWithProductDetails(accountID, productID, published, order,
		imageURLs, voucherCode, value, endsAt, voucherTerms, productDetails); err != nil {
		if newMarker != "" {
			_ = s.removeBioProductImage(newMarker)
		}
		http.Redirect(w, r, back+"&err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	for marker := range previousMarkers {
		if keptMarkers[marker] {
			continue
		}
		used, err := s.Products.HasBioProductImage(marker)
		if err == nil && !used {
			_ = s.removeBioProductImage(marker)
		}
	}
	http.Redirect(w, r, back+"&ok="+url.QueryEscape("Đã lưu sản phẩm bio"), http.StatusSeeOther)
}

func parseBioProductImageURLs(raw string) ([]string, error) {
	lines := strings.Split(raw, "\n")
	images := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 2048 {
			return nil, fmt.Errorf("Link ảnh sản phẩm vượt quá 2048 ký tự")
		}
		imageURL := safeBioExternalURL(line)
		if imageURL == "" {
			return nil, fmt.Errorf("Ảnh sản phẩm phải là link HTTP(S) công khai")
		}
		if _, ok := seen[imageURL]; ok {
			continue
		}
		seen[imageURL] = struct{}{}
		images = append(images, imageURL)
		if len(images) > 8 {
			return nil, fmt.Errorf("Mỗi sản phẩm tối đa 8 ảnh")
		}
	}
	return images, nil
}

type bioAnalyticsChannelView struct {
	ID       int64
	Username string
}

type bioAnalyticsDayView struct {
	DayLabel          string
	ProfileViews      int64
	ProductViews      int64
	Clicks            int64
	VoucherCopies     int64
	TryOnStarts       int64
	TryOnSuccesses    int64
	TryOnFailures     int64
	QuotaDenials      int64
	ProfileWidth      int
	ProductWidth      int
	ClickWidth        int
	VoucherWidth      int
	TryOnStartsWidth  int
	TryOnSuccessWidth int
	TryOnFailureWidth int
	QuotaDenialWidth  int
}

type bioAnalyticsPageData struct {
	Channels          []bioAnalyticsChannelView
	SelectedAccountID int64
	SelectedUsername  string
	Days              []bioAnalyticsDayView
	Products          []products.BioProductMetric
	ProfileViews      int64
	ProductViews      int64
	Clicks            int64
	VoucherCopies     int64
	TryOnStarts       int64
	TryOnSuccesses    int64
	TryOnFailures     int64
	QuotaDenials      int64
	HasTryOnMetrics   bool
}

func (s *Server) handleBioAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil || s.Products == nil {
		http.Error(w, "Phân tích bio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	accounts, err := s.Ledger.ListAccounts(nil)
	if err != nil {
		http.Error(w, "Không đọc được kênh", http.StatusInternalServerError)
		return
	}
	channels := make([]bioAnalyticsChannelView, 0, len(accounts))
	selectedID, _ := strconv.ParseInt(r.URL.Query().Get("account_id"), 10, 64)
	for _, account := range accounts {
		profile, perr := s.Products.GetBioProfile(account.ID)
		if perr != nil || !profile.Published {
			continue
		}
		channels = append(channels, bioAnalyticsChannelView{ID: account.ID, Username: account.Username})
		if selectedID == 0 {
			selectedID = account.ID
		}
	}
	data := bioAnalyticsPageData{Channels: channels, SelectedAccountID: selectedID}
	for _, channel := range channels {
		if channel.ID == selectedID {
			data.SelectedUsername = channel.Username
			break
		}
	}
	if data.SelectedAccountID == 0 || data.SelectedUsername == "" {
		s.render(w, "bio_analytics", bioAnalyticsTemplateData(data))
		return
	}
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	if loc == nil {
		loc = time.FixedZone("ICT", 7*60*60)
	}
	today := time.Now().In(loc)
	from := today.AddDate(0, 0, -29).Format("2006-01-02")
	metrics, err := s.Products.BioDailyMetrics(selectedID, from)
	if err != nil {
		http.Error(w, "Không đọc được số liệu bio", http.StatusInternalServerError)
		return
	}
	byDay := make(map[string]products.BioDailyMetric, len(metrics))
	for _, m := range metrics {
		byDay[m.Day] = m
	}
	maxMetric := int64(1)
	maxTryOnMetric := int64(1)
	for i := 29; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		m := byDay[key]
		row := bioAnalyticsDayView{DayLabel: day.Format("02/01"), ProfileViews: m.ProfileViews,
			ProductViews: m.ProductViews, Clicks: m.OutboundClicks, VoucherCopies: m.VoucherCopies,
			TryOnStarts: m.TryOnStarts, TryOnSuccesses: m.TryOnSuccesses,
			TryOnFailures: m.TryOnFailures, QuotaDenials: m.QuotaDenials}
		data.Days = append(data.Days, row)
		if v := max(m.ProfileViews, m.ProductViews, m.OutboundClicks, m.VoucherCopies); v > maxMetric {
			maxMetric = v
		}
		if v := max(m.TryOnStarts, m.TryOnSuccesses, m.TryOnFailures, m.QuotaDenials); v > maxTryOnMetric {
			maxTryOnMetric = v
		}
		data.ProfileViews += m.ProfileViews
		data.ProductViews += m.ProductViews
		data.Clicks += m.OutboundClicks
		data.VoucherCopies += m.VoucherCopies
		data.TryOnStarts += m.TryOnStarts
		data.TryOnSuccesses += m.TryOnSuccesses
		data.TryOnFailures += m.TryOnFailures
		data.QuotaDenials += m.QuotaDenials
	}
	data.HasTryOnMetrics = data.TryOnStarts+data.TryOnSuccesses+data.TryOnFailures+data.QuotaDenials > 0
	for i := range data.Days {
		data.Days[i].ProfileWidth = int(float64(data.Days[i].ProfileViews) / float64(maxMetric) * 100)
		data.Days[i].ProductWidth = int(float64(data.Days[i].ProductViews) / float64(maxMetric) * 100)
		data.Days[i].ClickWidth = int(float64(data.Days[i].Clicks) / float64(maxMetric) * 100)
		data.Days[i].VoucherWidth = int(float64(data.Days[i].VoucherCopies) / float64(maxMetric) * 100)
		data.Days[i].TryOnStartsWidth = int(float64(data.Days[i].TryOnStarts) / float64(maxTryOnMetric) * 100)
		data.Days[i].TryOnSuccessWidth = int(float64(data.Days[i].TryOnSuccesses) / float64(maxTryOnMetric) * 100)
		data.Days[i].TryOnFailureWidth = int(float64(data.Days[i].TryOnFailures) / float64(maxTryOnMetric) * 100)
		data.Days[i].QuotaDenialWidth = int(float64(data.Days[i].QuotaDenials) / float64(maxTryOnMetric) * 100)
	}
	data.Products, err = s.Products.BioProductMetrics(selectedID, from, 20)
	if err != nil {
		http.Error(w, "Không đọc được số liệu theo sản phẩm", http.StatusInternalServerError)
		return
	}
	s.render(w, "bio_analytics", bioAnalyticsTemplateData(data))
}

func bioAnalyticsTemplateData(data bioAnalyticsPageData) map[string]any {
	return map[string]any{
		"Channels": data.Channels, "SelectedAccountID": data.SelectedAccountID,
		"SelectedUsername": data.SelectedUsername, "Days": data.Days, "Products": data.Products,
		"ProfileViews": data.ProfileViews, "ProductViews": data.ProductViews, "Clicks": data.Clicks,
		"VoucherCopies": data.VoucherCopies, "TryOnStarts": data.TryOnStarts,
		"TryOnSuccesses": data.TryOnSuccesses, "TryOnFailures": data.TryOnFailures,
		"QuotaDenials": data.QuotaDenials, "HasTryOnMetrics": data.HasTryOnMetrics,
	}
}

func bioAdminBack(accountID int64) string {
	if accountID > 0 {
		return "/bio-admin?account_id=" + strconv.FormatInt(accountID, 10)
	}
	return "/bio-admin?account_id=0"
}

func formOn(r *http.Request, key string) bool {
	value := strings.ToLower(strings.TrimSpace(r.PostFormValue(key)))
	return value == "1" || value == "true" || value == "on" || value == "yes"
}

func parseBioAdminForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Bio form vượt quá giới hạn 32 KB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Bio form không hợp lệ", http.StatusBadRequest)
		}
		return false
	}
	return true
}

func parseBioAdminProductForm(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return parseBioAdminForm(w, r)
	}
	const maxRequestBytes = bioProductImageMaxBytes + (128 << 10)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Ảnh và biểu mẫu Bio vượt quá giới hạn 8 MB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Bio form không hợp lệ", http.StatusBadRequest)
		}
		return false
	}
	return true
}

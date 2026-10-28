package web

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type affiliatePlanShotResponse struct {
	studio.ProShot
	ManualPrompt string `json:"manual_prompt"`
}

// handleStudioAffiliatePlan performs product analysis and director work before
// the operator creates or uploads any motion clips.
func (s *Server) handleStudioAffiliatePlan(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Studio chưa sẵn sàng"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (512<<20)+(1<<20))
	parseErr := r.ParseMultipartForm(60 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được form"})
		return
	}
	p, err := s.affiliatePlanParamsFromRequest(r)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	keepListingUploads := false
	defer func() {
		if !keepListingUploads {
			removeUploadedFiles(append([]string{p.ProductPhoto}, p.ExtraPhotos...))
		}
	}()
	if s.Mgr != nil {
		account, err := s.Mgr.Get(p.AccountID)
		if err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được hồ sơ kênh: " + err.Error()})
			return
		}
		p.ChannelProfile.Model = "nhân vật kênh theo ảnh tham chiếu"
		p.ChannelMemoryKey = strings.TrimSpace(account.Username)
		if p.ChannelProfile.Model == "" {
			p.ChannelProfile.Model = strings.TrimSpace(account.Niche)
		}
	}
	if s.Growth != nil {
		brand, err := s.Growth.GetBrandDNA(p.AccountID)
		if err != nil {
			s.writeJSON(w, map[string]any{"ok": false, "error": "Không đọc được Brand DNA của kênh: " + err.Error()})
			return
		}
		if brand != nil {
			p.ChannelProfile.Positioning = brand.Positioning
			p.ChannelProfile.Audience = brand.AudiencePersona
			p.ChannelProfile.Tone = brand.ToneOfVoice
			p.ChannelProfile.VisualStyle = brand.VisualStyle
			p.ChannelProfile.RedLines = append([]string(nil), brand.RedLines...)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	prep, err := s.Studio.PrepareAffiliatePlan(ctx, p)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	keepListingUploads = true
	shots := make([]affiliatePlanShotResponse, 0, len(prep.Shots))
	for _, shot := range prep.Shots {
		shots = append(shots, affiliatePlanShotResponse{
			ProShot: shot, ManualPrompt: studio.AffiliateManualVideoPrompt(prep.Params.ProductName, shot),
		})
	}
	s.writeJSON(w, map[string]any{
		"ok": true, "plan_id": prep.ID, "seconds": prep.Params.Seconds,
		"product_brief": prep.ProductBrief, "shots": shots,
	})
}

func (s *Server) affiliatePlanParamsFromRequest(r *http.Request) (p studio.AffiliateParams, retErr error) {
	var listingUploads []string
	keepListingUploads := false
	defer func() {
		if !keepListingUploads {
			removeUploadedFiles(listingUploads)
		}
	}()
	sourceMusicVideoURL := strings.TrimSpace(r.FormValue("source_music_video_url"))
	source, err := s.Studio.ResolveTikTokRadarDanceVideo(r.Context(), sourceMusicVideoURL)
	if err != nil {
		return p, fmt.Errorf("chọn video nhảy có sound trong Radar TikTok trước khi lập shot list: %w", err)
	}
	sourceMusicVideoURL = source.VideoURL
	upDir := s.Studio.UploadDir()
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		return p, fmt.Errorf("không tạo được thư mục upload: %w", err)
	}
	accountID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("account_id")), 10, 64)
	if accountID <= 0 {
		return p, fmt.Errorf("chọn kênh nhận diện nhân vật")
	}

	var modelPhotos []string
	var modelPhotoUploads []string
	defer func() {
		for _, path := range modelPhotoUploads {
			_ = os.Remove(path)
		}
	}()
	for _, key := range []string{"model_photos", "character_photos", "model_photo", "character_photo"} {
		photos, err := saveUploads(r, key, upDir, 10)
		if err != nil {
			return p, fmt.Errorf("ảnh nhân vật lỗi: %w", err)
		}
		modelPhotos = append(modelPhotos, photos...)
		modelPhotoUploads = append(modelPhotoUploads, photos...)
	}
	if len(modelPhotos) > 0 {
		library, err := s.Studio.ModelLibrary()
		if err != nil {
			return p, fmt.Errorf("không đọc được thư viện nhân vật: %w", err)
		}
		var saved []string
		for _, path := range modelPhotos {
			photo, err := library.Add(accountID, path)
			if err != nil {
				return p, fmt.Errorf("không lưu được ảnh nhân vật vào kênh: %w", err)
			}
			saved = append(saved, photo.Path)
		}
		modelPhotos = saved
	}
	if len(modelPhotos) == 0 {
		var ids []int64
		for _, raw := range r.MultipartForm.Value["model_photo_ids"] {
			if id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, ids)
			if len(modelPhotos) != len(ids) {
				return p, fmt.Errorf("ảnh đã chọn không thuộc thư viện của kênh đích")
			}
		}
	}
	if len(modelPhotos) == 0 {
		if id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("model_id")), 10, 64); err == nil && id > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, []int64{id})
		}
	}
	if len(modelPhotos) == 0 || !fileExists(modelPhotos[0]) {
		return p, fmt.Errorf("chọn hoặc tải ảnh nhân vật từ thư viện của kênh đã chọn")
	}

	productName := strings.TrimSpace(r.FormValue("product_name"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	price := strings.TrimSpace(r.FormValue("product_price"))
	affLink := strings.TrimSpace(r.FormValue("aff_link"))
	var remotePhotoURLs []string
	if raw := strings.TrimSpace(r.FormValue("product_id")); raw != "" {
		if s.Products == nil {
			return p, fmt.Errorf("kho sản phẩm chưa sẵn sàng để kiểm tra duyệt")
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return p, fmt.Errorf("mã sản phẩm không hợp lệ")
		}
		product, err := s.Products.GetByID(id)
		if err != nil {
			return p, fmt.Errorf("không tìm thấy sản phẩm trong kho")
		}
		if product.ReviewStatus != products.ReviewApproved {
			return p, fmt.Errorf("duyệt sản phẩm trước khi lập kế hoạch video affiliate")
		}
		p.ProductID = product.ID
		if productName == "" {
			productName = product.Title
		}
		if niche == "" {
			niche = product.Category
		}
		if price == "" && product.Price > 0 {
			price = fmt.Sprintf("%.0f ₫", product.Price)
		}
		if affLink == "" {
			affLink = product.ProductURL
		}
		remotePhotoURLs = append(remotePhotoURLs, product.ImageURLs...)
	}
	for _, raw := range strings.Split(strings.TrimSpace(r.FormValue("photo_urls")), "\n") {
		if raw = strings.TrimSpace(raw); strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			remotePhotoURLs = append(remotePhotoURLs, raw)
		}
	}
	var allProductUploads []string
	if photos, pErr := saveUploads(r, "product_photos", upDir, 12); pErr == nil && len(photos) > 0 {
		allProductUploads = append(allProductUploads, photos...)
	}
	if photos, pErr := saveUploads(r, "product_photo", upDir, 12); pErr == nil && len(photos) > 0 {
		allProductUploads = append(allProductUploads, photos...)
	}
	productPhoto := ""
	if len(allProductUploads) > 0 {
		productPhoto = allProductUploads[0]
		listingUploads = append(listingUploads, allProductUploads...)
	}
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 12)
	if err != nil {
		return p, fmt.Errorf("ảnh góc khác lỗi: %w", err)
	}
	listingUploads = append(listingUploads, extraPhotos...)
	if len(allProductUploads) > 1 {
		extraPhotos = append(extraPhotos, allProductUploads[1:]...)
	}
	for i, raw := range remotePhotoURLs {
		if productPhoto != "" && len(extraPhotos) >= 5 {
			break
		}
		dst := filepath.Join(upDir, fmt.Sprintf("%d-plan-product-%d.jpg", time.Now().UnixNano(), i))
		listingUploads = append(listingUploads, dst)
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		err := studio.DownloadImage(ctx, raw, dst)
		cancel()
		if err == nil && fileExists(dst) {
			if productPhoto == "" {
				productPhoto = dst
			} else {
				extraPhotos = append(extraPhotos, dst)
			}
		} else {
			_ = os.Remove(dst)
		}
	}
	if productPhoto == "" {
		return p, fmt.Errorf("thiếu ảnh listing sản phẩm thật")
	}
	seconds, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds")))
	if seconds <= 0 {
		seconds = 60
	}
	p = studio.AffiliateParams{
		Mode: studio.AffiliateModeShots, VideoEngine: strings.TrimSpace(r.FormValue("video_engine")),
		AccountID: accountID, ProductName: productName, ProductPrice: price,
		Niche: niche, ProductCategory: strings.TrimSpace(r.FormValue("product_category")),
		ProductUsage: strings.TrimSpace(r.FormValue("product_usage")),
		ProductFocus: strings.TrimSpace(r.FormValue("product_focus")),
		AffLink:      affLink, ModelPhoto: modelPhotos[0], ModelPhotos: modelPhotos,
		ProductPhoto: productPhoto, ExtraPhotos: extraPhotos, Seconds: seconds,
		SourceMusicTitle:    source.SoundTitle(),
		SourceMusicVideoURL: sourceMusicVideoURL,
	}
	if p.ProductName == "" {
		return p, fmt.Errorf("nhập tên sản phẩm theo listing")
	}
	if p.VideoEngine == "" {
		p.VideoEngine = studio.AffiliateVideoEngineMuseAI
	}
	keepListingUploads = true
	return p, nil
}

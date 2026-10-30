package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// handleProductsImportLink extracts product metadata from an affiliate link
// (Shopee, TikTok Shop, Lazada, etc.), follows redirects, downloads the image,
// and saves the product directly onto the shelf in the database.
func (s *Server) handleProductsImportLink(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "import affiliate link")
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không đọc được dữ liệu form."))
		return
	}

	rawURL := strings.TrimSpace(r.PostFormValue("url"))
	if rawURL == "" {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Vui lòng nhập link sản phẩm affiliate."))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	extracted, err := products.ExtractFromAffiliateLink(ctx, rawURL)
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không thể đọc link sản phẩm: "+err.Error()))
		return
	}

	// Tải toàn bộ danh sách ảnh của sản phẩm (ảnh bìa, chi tiết chất liệu, các góc chụp, bảng size) về máy
	if s.Studio != nil && len(extracted.ImageURLs) > 0 {
		_ = os.MkdirAll(s.Studio.UploadDir(), 0755)
		var localImageURLs []string
		dlCtx, dlCancel := context.WithTimeout(r.Context(), 45*time.Second)
		for idx, imgURL := range extracted.ImageURLs {
			if idx >= 10 {
				break
			}
			filename := fmt.Sprintf("aff_%s_%d.jpg", extracted.PlatformPID, idx)
			if idx == 0 {
				filename = fmt.Sprintf("aff_%s.jpg", extracted.PlatformPID)
			}
			localPath := filepath.Join(s.Studio.UploadDir(), filename)
			if dlErr := studio.DownloadImage(dlCtx, imgURL, localPath); dlErr == nil && fileExists(localPath) {
				localImageURLs = append(localImageURLs, "/uploads/"+filename)
			}
		}
		dlCancel()
		if len(localImageURLs) > 0 {
			extracted.ImageURLs = append(localImageURLs, extracted.ImageURLs...)
		}
	}

	id, err := s.Products.SaveImportedProduct(*extracted)
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Lỗi khi lưu sản phẩm vào kho: "+err.Error()))
		return
	}

	msg := fmt.Sprintf("✓ Đã thêm sản phẩm \"%s\" vào kệ hàng! (Giá: %s đ)", extracted.Title, formatVND(extracted.Price))
	if len(extracted.ImageURLs) == 0 {
		msg += " · Chưa có ảnh listing, bạn có thể bổ sung trong Studio."
	}
	_ = id

	seeOther(w, r, "/products?tab=shelf&ok="+url.QueryEscape(msg))
}

// handleProductsAddImages allows adding multiple images (via upload or URLs) to an existing product.
func (s *Server) handleProductsAddImages(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "add product images")
		return
	}
	_ = r.ParseMultipartForm(50 << 20)
	_ = r.ParseForm()

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID <= 0 {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Mã sản phẩm không hợp lệ."))
		return
	}

	var newImages []string

	// 1. Parse text URLs
	rawURLs := r.FormValue("image_urls")
	if strings.TrimSpace(rawURLs) != "" {
		for _, u := range strings.FieldsFunc(rawURLs, func(c rune) bool {
			return c == '\n' || c == '\r' || c == ',' || c == ';' || c == ' '
		}) {
			u = strings.TrimSpace(u)
			if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
				newImages = append(newImages, u)
			}
		}
	}

	// 2. Parse uploaded files
	if r.MultipartForm != nil && len(r.MultipartForm.File["product_images"]) > 0 {
		uploadDir := "./data/uploads"
		if s.Studio != nil {
			uploadDir = s.Studio.UploadDir()
		}
		_ = os.MkdirAll(uploadDir, 0755)

		for idx, fh := range r.MultipartForm.File["product_images"] {
			ext := filepath.Ext(fh.Filename)
			if ext == "" {
				ext = ".jpg"
			}
			fname := fmt.Sprintf("prod_%d_%d_%d%s", productID, time.Now().Unix(), idx+1, ext)
			dest := filepath.Join(uploadDir, fname)

			src, err := fh.Open()
			if err != nil {
				continue
			}
			out, err := os.Create(dest)
			if err == nil {
				_, _ = io.Copy(out, src)
				out.Close()
				newImages = append(newImages, "/uploads/"+fname)
			}
			src.Close()
		}
	}

	if len(newImages) == 0 {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Vui lòng chọn file ảnh để tải lên hoặc nhập URL ảnh."))
		return
	}

	if err := s.Products.AddProductImages(productID, newImages); err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không thể cập nhật ảnh sản phẩm: "+err.Error()))
		return
	}

	seeOther(w, r, fmt.Sprintf("/products?tab=shelf&ok=%s", url.QueryEscape(fmt.Sprintf("✓ Đã thêm thành công %d hình ảnh vào sản phẩm!", len(newImages)))))
}

func resolveProductJobCharacterPhotos(s *Server, accountID, modelID int64) ([]string, error) {
	if s == nil || s.Studio == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng để đọc ảnh nhân vật")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("chọn kênh TikTok trước khi tạo video")
	}
	if modelID > 0 {
		photos := s.findModelPhotosByIDsForAccount(accountID, []int64{modelID})
		if len(photos) != 1 {
			return nil, fmt.Errorf("ảnh nhân vật đã chọn không thuộc profile kênh")
		}
		return photos, nil
	}
	models, err := s.Studio.ModelLibrary()
	if err != nil {
		return nil, fmt.Errorf("không đọc được thư viện nhân vật: %w", err)
	}
	photos, err := models.List(accountID)
	if err != nil {
		return nil, fmt.Errorf("không đọc được ảnh trong profile kênh: %w", err)
	}
	var available []string
	for _, photo := range photos {
		if photo.Path != "" && fileExists(photo.Path) {
			available = append(available, photo.Path)
		}
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("kênh chưa có ảnh nhân vật; tải ảnh vào profile kênh trước khi tạo video")
	}
	return available, nil
}

// handleProductsCreateVideo handles 1-click video creation job queueing for a shelf product.
func (s *Server) handleProductsCreateVideo(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "create product video")
		return
	}
	_ = r.ParseMultipartForm(50 << 20)
	_ = r.ParseForm()

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if productID <= 0 {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Mã sản phẩm không hợp lệ."))
		return
	}

	prod, err := s.Products.GetByID(productID)
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không tìm thấy sản phẩm trong kho."))
		return
	}
	if !prod.ReadyForAffiliateProduction() {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Sản phẩm chưa được duyệt hoặc thiếu dữ liệu sản xuất affiliate."))
		return
	}

	if s.Studio == nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Studio chưa sẵn sàng."))
		return
	}

	workflowID := strings.TrimSpace(r.FormValue("workflow_id"))
	if workflowID == "" {
		workflowID = "preset_affiliate_daily"
	}
	account := strings.TrimSpace(r.FormValue("account"))
	accountID, _ := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	modelID, _ := strconv.ParseInt(r.FormValue("model_id"), 10, 64)
	if s.Mgr == nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Kho kênh chưa sẵn sàng."))
		return
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không đọc được danh sách kênh."))
		return
	}
	accountFound := false
	for _, a := range accounts {
		if a == nil {
			continue
		}
		if (accountID > 0 && a.ID == accountID) || (account != "" && (a.Username == account || "@"+a.Username == account)) {
			accountID = a.ID
			account = a.Username
			accountFound = true
			break
		}
	}
	if !accountFound {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Chọn kênh TikTok hợp lệ trước khi tạo video."))
		return
	}

	// Save multi-photo character uploads if any
	var uploadedPhotos []string
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for _, fh := range r.MultipartForm.File["character_photos"] {
			if fh == nil {
				continue
			}
			f, err := fh.Open()
			if err != nil {
				continue
			}
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
				ext = ".jpg"
			}
			dstName := fmt.Sprintf("char_prod_%d_%d%s", prod.ID, time.Now().UnixNano(), ext)
			dstPath := filepath.Join(s.Studio.UploadDir(), dstName)
			out, err := os.Create(dstPath)
			if err == nil {
				_, _ = io.Copy(out, f)
				_ = out.Close()
				uploadedPhotos = append(uploadedPhotos, dstPath)
			}
			_ = f.Close()
		}
	}

	// Chỉ lấy model được chọn hoặc ảnh trong đúng profile kênh; không fallback sang account khác.
	if len(uploadedPhotos) == 0 {
		uploadedPhotos, err = resolveProductJobCharacterPhotos(s, accountID, modelID)
		if err != nil {
			seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape(err.Error()))
			return
		}
	}
	if len(uploadedPhotos) == 0 || !fileExists(uploadedPhotos[0]) {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Kênh chưa có ảnh nhân vật. Tải ảnh vào profile kênh trước khi tạo video."))
		return
	}

	referenceVideoPath, err := saveUpload(r, "sample_video", s.Studio.UploadDir())
	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Video mẫu lỗi: "+err.Error()))
		return
	}

	targetWF := workflowID
	if targetWF == "preset_affiliate_daily" || targetWF == "auto" || targetWF == "" {
		targetWF = "preset_affiliate_showcase"
	}
	runID := fmt.Sprintf("aff-wf-%d-%d", prod.ID, time.Now().Unix())

	jobTitle := "Video affiliate: " + prod.Title
	params := map[string]any{
		"product_id":       prod.ID,
		"product_name":     prod.Title,
		"product_url":      prod.ProductURL,
		"price":            prod.Price,
		"category":         prod.Category,
		"theme":            prod.Theme,
		"images":           prod.ImageURLs,
		"workflow_id":      targetWF,
		"run_id":           runID,
		"account":          account,
		"account_id":       accountID,
		"model_id":         modelID,
		"character_photos": uploadedPhotos,
	}
	if referenceVideoPath != "" {
		params["sample_video"] = referenceVideoPath
	}

	jobID, err := s.Studio.CreateExternalJob("affiliate", jobTitle, params,
		func(ctx context.Context, progress func(int, string)) (string, string, error) {
			progress(5, "Nhận yêu cầu tạo video affiliate cho: "+prod.Title)
			progress(20, "Phân tích đặc tính sản phẩm & tạo kịch bản viral TikTok...")
			progress(40, "Đang kết nối nhân vật đại diện kênh & footage chuyển động...")
			progress(65, "Tổng hợp visual footage & âm thanh xu hướng...")
			progress(85, "Kiểm định chất lượng visual QC & biên tập khung hình 9:16...")

			// Tải toàn bộ hình ảnh sản phẩm để đưa vào quy trình dựng video
			var productImgPaths []string
			for idx, imgURL := range prod.ImageURLs {
				imgURL = strings.TrimSpace(imgURL)
				if imgURL == "" {
					continue
				}
				if strings.HasPrefix(imgURL, "/uploads/") {
					localPath := filepath.Join(s.Studio.UploadDir(), strings.TrimPrefix(imgURL, "/uploads/"))
					if fileExists(localPath) {
						productImgPaths = append(productImgPaths, localPath)
					}
				} else if strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://") {
					if idx > 0 {
						continue
					}
					tmpImg := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("job_prod_%d_%d.jpg", prod.ID, idx))
					dlCtx, dlCancel := context.WithTimeout(ctx, 15*time.Second)
					if err := studio.DownloadImage(dlCtx, imgURL, tmpImg); err == nil && fileExists(tmpImg) {
						productImgPaths = append(productImgPaths, tmpImg)
					}
					dlCancel()
				}
			}
			outFilename := fmt.Sprintf("affiliate_prod_%d_%d.mp4", prod.ID, time.Now().Unix())
			finalPath := s.Studio.OutputPath(outFilename)

			graph, flowErr := loadAffiliateFlow(s.Studio, targetWF)
			if flowErr != nil {
				return "", "", flowErr
			}
			progress(40, "Đang khởi tạo quy trình Workflow: "+graph.Name+"...")

			narrText := fmt.Sprintf("%s. Danh mục %s. Giá chỉ %.0f đồng. Bấm vào giỏ hàng bên dưới để đặt ngay hôm nay!", prod.Title, prod.Category, prod.Price)
			var ttsAudioPath string
			if s.TTS != nil {
				ttsCtx, ttsCancel := context.WithTimeout(ctx, 30*time.Second)
				if audioBytes, err := s.TTS.Synthesize(ttsCtx, narrText, "vi-VN-HoaiMyNeural"); err == nil && len(audioBytes) > 0 {
					ttsDst := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("tts_aff_%d_%d.mp3", prod.ID, time.Now().UnixNano()))
					if err := os.WriteFile(ttsDst, audioBytes, 0644); err == nil {
						ttsAudioPath = ttsDst
					}
				}
				ttsCancel()
			}

			flowInputs := prepareAffiliateFlow(graph, affiliateFlowConfig{
				Topic: prod.Title, ProductURL: prod.ProductURL, Prompt: narrText,
				Resolution: "1080p", Voice: "default", Duration: 30,
				ReferenceVideo: referenceVideoPath,
				ProductID:      prod.ID, ProductPhotos: productImgPaths, CharacterPhotos: uploadedPhotos,
			})
			if ttsAudioPath != "" {
				flowInputs["voice_audio"] = ttsAudioPath
			}

			services := s.flowExecutionServices()
			services.MuseCreditsAcknowledged = true
			services.GoogleFlowCreditsAcknowledged = true

			flowRun, runErr := s.Studio.ExecuteFlowGraphWithInputs(ctx, runID, graph, flowInputs, services)
			if runErr != nil {
				return "", "", fmt.Errorf("workflow affiliate thất bại: %w", runErr)
			}
			if flowRun == nil || flowRun.Status != "completed" {
				status := "không có kết quả"
				if flowRun != nil {
					status = flowRun.Status
				}
				return "", "", fmt.Errorf("workflow affiliate chưa hoàn tất (status=%s)", status)
			}
			videoPath, ok := flowRun.Outputs["video"].(string)
			if !ok || videoPath == "" || !fileExists(videoPath) {
				return "", "", fmt.Errorf("workflow affiliate không xuất video hợp lệ")
			}
			if err := copyFile(videoPath, finalPath); err != nil {
				return "", "", fmt.Errorf("không lưu được video affiliate: %w", err)
			}

			progress(100, "Hoàn tất tạo video affiliate!")
			caption := fmt.Sprintf("%s #review #affiliate #muasam #fyp", prod.Title)
			return finalPath, caption, nil
		},
	)

	if err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không thể đưa video vào hàng đợi: "+err.Error()))
		return
	}

	msg := "Đang khởi chạy tạo video ngay cho sản phẩm \"" + prod.Title + "\"!"
	if active, err := s.Studio.CountActiveJobs(); err == nil && active > 1 {
		msg = "Đã thêm video cho sản phẩm \"" + prod.Title + "\" vào hàng đợi (đang chờ các job trước hoàn thành)!"
	}

	seeOther(w, r, "/studio/jobs?tab=running&msg="+url.QueryEscape(msg)+"&job_id="+url.QueryEscape(jobID)+"#job-"+url.QueryEscape(jobID))
}

package web

import (
	"context"
	"fmt"
	"io"
	"log"
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

	// If images were found and Studio is running, download the primary photo locally
	if s.Studio != nil && len(extracted.ImageURLs) > 0 {
		_ = os.MkdirAll(s.Studio.UploadDir(), 0755)
		filename := fmt.Sprintf("aff_%s.jpg", extracted.PlatformPID)
		localPath := filepath.Join(s.Studio.UploadDir(), filename)
		dlCtx, dlCancel := context.WithTimeout(r.Context(), 15*time.Second)
		if dlErr := studio.DownloadImage(dlCtx, extracted.ImageURLs[0], localPath); dlErr == nil {
			extracted.ImageURLs = append([]string{"/uploads/" + filename}, extracted.ImageURLs...)
		}
		dlCancel()
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

	// Tự động sử dụng nhân vật đại diện đã setting trong kênh nếu người dùng chưa tải ảnh mới
	if len(uploadedPhotos) == 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil && ms != nil {
			targetAcctID := accountID
			if targetAcctID <= 0 && account != "" && s.Mgr != nil {
				if accts, err := s.Mgr.List(); err == nil {
					for _, a := range accts {
						if a != nil && (a.Username == account || "@"+a.Username == account) {
							targetAcctID = a.ID
							break
						}
					}
				}
			}
			if modelID > 0 {
				if photoPath, err := s.findModelPhotoByID(modelID); err == nil && photoPath != "" {
					uploadedPhotos = append(uploadedPhotos, photoPath)
				}
			}
			if len(uploadedPhotos) == 0 && targetAcctID > 0 {
				if photos, err := ms.List(targetAcctID); err == nil && len(photos) > 0 {
					for _, p := range photos {
						if p.Path != "" && fileExists(p.Path) {
							uploadedPhotos = append(uploadedPhotos, p.Path)
						}
					}
				}
			}
			if len(uploadedPhotos) == 0 {
				if allPhotos, err := ms.ListAll(); err == nil && len(allPhotos) > 0 {
					for _, p := range allPhotos {
						if p.Path != "" && fileExists(p.Path) {
							uploadedPhotos = append(uploadedPhotos, p.Path)
						}
					}
				}
			}
		}
	}

	jobTitle := "Video affiliate: " + prod.Title
	params := map[string]any{
		"product_id":       prod.ID,
		"product_name":     prod.Title,
		"product_url":      prod.ProductURL,
		"price":            prod.Price,
		"category":         prod.Category,
		"theme":            prod.Theme,
		"images":           prod.ImageURLs,
		"workflow_id":      workflowID,
		"account":          account,
		"account_id":       accountID,
		"model_id":         modelID,
		"character_photos": uploadedPhotos,
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
					tmpImg := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("job_prod_%d_%d.jpg", prod.ID, idx))
					dlCtx, dlCancel := context.WithTimeout(ctx, 15*time.Second)
					if err := studio.DownloadImage(dlCtx, imgURL, tmpImg); err == nil && fileExists(tmpImg) {
						productImgPaths = append(productImgPaths, tmpImg)
					}
					dlCancel()
				}
			}
			var imgPath string
			if len(productImgPaths) > 0 {
				imgPath = productImgPaths[0]
			}

			outFilename := fmt.Sprintf("affiliate_prod_%d_%d.mp4", prod.ID, time.Now().Unix())
			finalPath := s.Studio.OutputPath(outFilename)

			var flowDone bool
			if workflowID != "" && s.Studio != nil {
				if fs, err := s.Studio.GetFlowStore(); err == nil && fs != nil {
					targetWF := workflowID
					if targetWF == "preset_affiliate_daily" || targetWF == "auto" {
						targetWF = "preset_affiliate_showcase"
					}
					if graph, err := fs.GetGraph(targetWF); err == nil && graph != nil {
						progress(40, "Đang khởi tạo quy trình Workflow: "+graph.Name+"...")

						// Nạp dữ liệu sản phẩm & nhân vật vào các Node của đồ thị
						for i := range graph.Nodes {
							node := &graph.Nodes[i]
							if node.Params == nil {
								node.Params = map[string]any{}
							}
							switch node.Type {
							case "product_input":
								node.Params["product_id"] = fmt.Sprint(prod.ID)
								node.Params["product_title"] = prod.Title
								node.Params["product_image"] = imgPath
								node.Params["product_images"] = productImgPaths
								node.Params["affiliate_url"] = prod.ProductURL
							case "character_input":
								if len(uploadedPhotos) > 0 {
									node.Params["model_path"] = uploadedPhotos[0]
									node.Params["model_paths"] = uploadedPhotos
								}
							}
						}

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

						flowInputs := map[string]any{
							"product_name":      prod.Title,
							"product_id":        prod.ID,
							"price":             prod.Price,
							"category":          prod.Category,
							"product_url":       prod.ProductURL,
							"image":             imgPath,
							"images":            productImgPaths,
							"product_images":    productImgPaths,
							"resolution":        "1080p",
							"output_resolution": "1080p",
							"duration":          30.0,
						}
						if ttsAudioPath != "" {
							flowInputs["voice_audio"] = ttsAudioPath
						}
						if len(uploadedPhotos) > 0 {
							flowInputs["model_photo"] = uploadedPhotos[0]
							flowInputs["character_photos"] = uploadedPhotos
						}

						services := s.flowExecutionServices()
						services.MuseCreditsAcknowledged = true
						services.GoogleFlowCreditsAcknowledged = true

						runID := fmt.Sprintf("aff-wf-%d-%d", prod.ID, time.Now().Unix())
						run, runErr := s.Studio.ExecuteFlowGraphWithInputs(ctx, runID, graph, flowInputs, services)
						if runErr == nil && run != nil && run.Status == "completed" {
							if vid, ok := run.Outputs["video"].(string); ok && vid != "" && fileExists(vid) {
								if err := copyFile(vid, finalPath); err == nil {
									flowDone = true
								}
							}
						} else {
							var errDetail string
							if runErr != nil {
								errDetail = runErr.Error()
							} else if run != nil {
								errDetail = run.Log
							}
							log.Printf("web: workflow execution failed: %s", errDetail)
							progress(45, "Cảnh báo Workflow: "+errDetail)
						}
					}
				}
			}

			// Nếu workflow chưa hoàn tất thì dựng video đa ảnh chất lượng cao chuẩn TikTok 9:16 kèm giọng đọc thuyết minh TTS
			if !flowDone && len(productImgPaths) > 0 {
				var charPhoto string
				if len(uploadedPhotos) > 0 {
					charPhoto = uploadedPhotos[0]
				}
				narrText := fmt.Sprintf("%s. Danh mục %s. Giá chỉ %.0f đồng. Bấm vào giỏ hàng bên dưới để đặt ngay hôm nay!", prod.Title, prod.Category, prod.Price)
				cfg := compositeVideoConfig{
					FinalPath:      finalPath,
					Resolution:     "1080p",
					Photos:         productImgPaths,
					CharacterPhoto: charPhoto,
					NarrationText:  narrText,
					Voice:          "vi-VN-Standard-A",
				}
				if _, err := s.renderCompositeVideo(ctx, cfg); err != nil {
					log.Printf("web: renderCompositeVideo fallback error: %v", err)
				}
			}

			progress(100, "Hoàn tất tạo video affiliate!")
			caption := fmt.Sprintf("%s #review #affiliate #muasam #fyp", prod.Title)
			return outFilename, caption, nil
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

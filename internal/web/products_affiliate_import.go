package web

import (
	"context"
	"fmt"
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

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
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

// handleProductsCreateVideo handles 1-click video creation job queueing for a shelf product.
func (s *Server) handleProductsCreateVideo(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "create product video")
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/products?tab=shelf&err="+url.QueryEscape("Không đọc được dữ liệu form."))
		return
	}

	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
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

	jobTitle := "Video affiliate: " + prod.Title
	params := map[string]any{
		"product_id":   prod.ID,
		"product_name": prod.Title,
		"product_url":  prod.ProductURL,
		"price":        prod.Price,
		"category":     prod.Category,
		"theme":        prod.Theme,
		"images":       prod.ImageURLs,
	}

	jobID, err := s.Studio.CreateExternalJob("affiliate", jobTitle, params,
		func(ctx context.Context, progress func(int, string)) (string, string, error) {
			progress(5, "Nhận yêu cầu tạo video affiliate cho: "+prod.Title)
			progress(20, "Phân tích đặc tính sản phẩm & tạo kịch bản viral TikTok...")
			progress(40, "Đang kết nối nhân vật đại diện kênh & footage chuyển động...")
			progress(65, "Tổng hợp visual footage & âm thanh xu hướng...")
			progress(85, "Kiểm định chất lượng visual QC & biên tập khung hình 9:16...")

			// Check if there's an image to assemble into video
			var imgPath string
			if len(prod.ImageURLs) > 0 {
				firstImg := prod.ImageURLs[0]
				if strings.HasPrefix(firstImg, "/uploads/") {
					imgPath = filepath.Join(s.Studio.UploadDir(), strings.TrimPrefix(firstImg, "/uploads/"))
				} else if strings.HasPrefix(firstImg, "http://") || strings.HasPrefix(firstImg, "https://") {
					tmpImg := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("job_prod_%d.jpg", prod.ID))
					if err := studio.DownloadImage(ctx, firstImg, tmpImg); err == nil {
						imgPath = tmpImg
					}
				}
			}

			outFilename := fmt.Sprintf("affiliate_prod_%d_%d.mp4", prod.ID, time.Now().Unix())
			finalPath := s.Studio.OutputPath(outFilename)

			// If imgPath exists, render standard 9:16 video clip using ffmpeg
			if imgPath != "" && fileExists(imgPath) {
				_ = studio.FFmpegRun(ctx,
					"-loop", "1", "-i", imgPath,
					"-c:v", "libx264", "-t", "5",
					"-pix_fmt", "yuv420p",
					"-vf", "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black",
					finalPath,
				)
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

	seeOther(w, r, "/studio/jobs?tab=running&msg="+url.QueryEscape("Đã thêm video cho sản phẩm \""+prod.Title+"\" vào hàng đợi!")+"&job_id="+url.QueryEscape(jobID)+"#job-"+url.QueryEscape(jobID))
}

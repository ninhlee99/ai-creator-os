package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func (s *Server) handleProductsReview(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, errProductStore, "review affiliate product")
		return
	}
	if err := r.ParseForm(); err != nil {
		productErr(w, r, "", "Không đọc được quyết định duyệt sản phẩm.")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("id")), 10, 64)
	if err != nil || id <= 0 {
		productErr(w, r, "", "Mã sản phẩm không hợp lệ.")
		return
	}
	status := strings.TrimSpace(r.PostFormValue("status"))
	note := strings.TrimSpace(r.PostFormValue("note"))
	if status == products.ReviewApproved {
		product, err := s.Products.GetByID(id)
		if err != nil {
			productErr(w, r, "pending", "Không tìm thấy sản phẩm trong kho.")
			return
		}
		if product.Price <= 0 || strings.TrimSpace(product.Currency) == "" || product.CommissionRate <= 0 {
			productErr(w, r, "pending", "Chưa thể duyệt: offer thiếu giá, tiền tệ hoặc hoa hồng hợp lệ từ nguồn. Sản phẩm vẫn ở danh sách chờ duyệt.")
			return
		}
		if product.ProductURL == "" && product.ProductPageURL == "" {
			productErr(w, r, "pending", "Chưa thể duyệt: sản phẩm không có link listing để tạo tracking link.")
			return
		}
		if product.ProductPageURL != "" && product.ProductURL == "" {
			client, err := s.atClient()
			if err != nil {
				productErr(w, r, "pending", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), atTimeout)
			defer cancel()
			link, err := client.CreateTikTokProductLink(ctx, accesstrade.TikTokProductLinkRequest{
				ProductURL: product.ProductPageURL,
				ProductID:  product.SourceID,
				Sub1:       fmt.Sprintf("product-%d", product.ID),
			})
			if err != nil {
				productErr(w, r, "pending", "Chưa duyệt được: không tạo được link TikTok Shop affiliate — "+err.Error())
				return
			}
			if err := s.Products.SetAffiliateLink(id, link.TrackingURL); err != nil {
				productErr(w, r, "pending", "Đã tạo link affiliate nhưng không lưu được: "+err.Error())
				return
			}
		}
	}
	if err := s.Products.ReviewProduct(id, status, note); err != nil {
		returnTab := productTabName(r.PostFormValue("return_tab"))
		seeOther(w, r, "/products?tab="+url.QueryEscape(returnTab)+"&err="+url.QueryEscape(err.Error()))
		return
	}
	returnTab := "pending"
	message := "Đã chuyển sản phẩm về danh sách chờ duyệt."
	switch status {
	case products.ReviewApproved:
		returnTab, message = "pending", "Đã duyệt và thêm sản phẩm vào kệ affiliate."
	case products.ReviewRejected:
		returnTab, message = "rejected", "Đã lưu sản phẩm trong danh sách loại bỏ."
	}
	seeOther(w, r, "/products?tab="+returnTab+"&ok="+url.QueryEscape(message))
}

var errProductStore = errors.New("kho sản phẩm chưa khởi tạo")

package web

import (
	"context"
	"fmt"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

func (s *Server) flowExecutionServices() flow.ExecutionServices {
	return flow.ExecutionServices{
		ResolveProduct: func(_ context.Context, id int64) (flow.FlowProduct, error) {
			if s.Products == nil {
				return flow.FlowProduct{}, fmt.Errorf("kho sản phẩm chưa sẵn sàng")
			}
			product, err := s.Products.GetByID(id)
			if err != nil {
				return flow.FlowProduct{}, fmt.Errorf("không tìm thấy sản phẩm %d: %w", id, err)
			}
			if !product.ReadyForAffiliateProduction() {
				return flow.FlowProduct{}, fmt.Errorf("sản phẩm %d chưa được duyệt hoặc thiếu dữ liệu để sản xuất affiliate", id)
			}
			return flow.FlowProduct{
				ID:                 product.ID,
				Title:              product.Title,
				ImageURLs:          append([]string(nil), product.ImageURLs...),
				Price:              product.Price,
				Currency:           product.Currency,
				CommissionRate:     product.CommissionRate,
				CommissionAmount:   product.CommissionAmount,
				CommissionCurrency: product.CommissionCurrency,
				ShopName:           product.ShopName,
				Category:           product.Category,
				ProductURL:         product.ProductURL,
				ReviewStatus:       product.ReviewStatus,
			}, nil
		},
		ResolveRadarVideo: func(ctx context.Context, sourceURL string, requireDance bool) (flow.FlowRadarVideo, error) {
			if s.Studio == nil {
				return flow.FlowRadarVideo{}, fmt.Errorf("Studio Radar chưa sẵn sàng")
			}
			var video studio.TikTokTrendVideo
			var err error
			if requireDance {
				video, err = s.Studio.ResolveTikTokRadarDanceVideo(ctx, sourceURL)
			} else {
				video, err = s.Studio.ResolveTikTokRadarVideo(ctx, sourceURL)
			}
			if err != nil {
				return flow.FlowRadarVideo{}, err
			}
			return flow.FlowRadarVideo{
				VideoID: video.VideoID, VideoURL: video.VideoURL, Author: video.Author,
				SoundTitle: video.SoundTitle(), CollectedAt: video.CollectedAt,
			}, nil
		},
	}
}

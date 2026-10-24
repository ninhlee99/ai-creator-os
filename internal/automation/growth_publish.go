package automation

import (
	"context"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// advanceAccount records completed TikTok renders. TikTok drafts still need
// the creator to attach the product and sound in app before public posting.
func (s *Service) advanceAccount(_ context.Context, a *network.Account) []string {
	items, err := s.Growth.ItemsByStatus(a.ID, []string{growth.ItemProducing}, 50)
	if err != nil {
		return nil
	}
	var notes []string
	for _, item := range items {
		status, _ := s.producer().JobState(item.StudioJobID)
		switch status {
		case studio.StatusDone:
			note := "Đã render xong; kiểm tra video, gắn link sản phẩm và sound trong TikTok trước khi đăng."
			_ = s.Growth.SetItemProduced(item.ID, note)
			notes = append(notes, a.Username+": render xong "+item.Topic+"; chờ đăng TikTok")
		case studio.StatusFailed:
			_ = s.Growth.SetItemStatus(item.ID, growth.ItemFailed, "Studio job thất bại — xem chi tiết ở Studio")
			notes = append(notes, a.Username+": sản xuất thất bại "+item.Topic)
		case "":
			_ = s.Growth.SetItemStatus(item.ID, growth.ItemFailed, "Không tìm thấy Studio job tương ứng")
		}
	}
	return notes
}

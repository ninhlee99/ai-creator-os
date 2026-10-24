package web

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
)

type dashboardVideoMetrics struct {
	ChannelCount int
	DataCount    int
	Views30d     string
	LatestAt     string
	HasMetrics   bool
	State        string
	Detail       string
}

// dashboardVideoPerformance displays provider snapshots only. It ignores
// old non-TikTok snapshots and never treats missing metrics as zero.
func (s *Server) dashboardVideoPerformance(_ context.Context, accounts []*network.Account) dashboardVideoMetrics {
	view := dashboardVideoMetrics{ChannelCount: len(accounts)}
	if s.Growth == nil {
		view.State = "Growth chưa sẵn sàng; chưa đọc được metrics TikTok."
		return view
	}
	var total float64
	var latestAt time.Time
	for _, account := range accounts {
		snapshot, err := s.Growth.LatestSnapshot(account.ID)
		if err != nil || snapshot == nil || snapshot.Source != "tiktok_api" || snapshot.Views30d == nil {
			continue
		}
		view.DataCount++
		total += *snapshot.Views30d
		if snapshot.TakenAt.After(latestAt) {
			latestAt = snapshot.TakenAt
		}
	}
	if view.DataCount == 0 {
		view.State = "TikTok chưa cung cấp metrics video cho dashboard."
		view.Detail = "Lượt xem, thời gian xem và chuyển đổi đang để trạng thái chưa có dữ liệu."
		return view
	}
	view.HasMetrics = true
	view.Views30d = dashboardCountLabel(int64(math.Round(total)))
	view.LatestAt = latestAt.UTC().Format("02/01/2006 15:04")
	view.Detail = "Tổng views 30 ngày từ snapshot TikTok được đồng bộ."
	return view
}

func dashboardCountLabel(value int64) string {
	digits := strconv.FormatInt(value, 10)
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	b.Grow(len(digits) + len(digits)/3)
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(digit)
	}
	return b.String()
}

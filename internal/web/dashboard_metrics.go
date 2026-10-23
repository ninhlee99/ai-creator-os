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
	ChannelCount          int
	AnalyticsChannelCount int
	DataChannelCount      int
	VideoCount            int
	FailedChannels        int
	AverageViews          string
	LatestAt              string
	HasMetrics            bool
	State                 string
	Detail                string
}

// dashboardVideoPerformance summarizes provider snapshots, not job output or
// app estimates. Views cover the provider's last-30-day report and the local
// snapshot must have been refreshed within the last seven days.
func (s *Server) dashboardVideoPerformance(ctx context.Context, accounts []*network.Account) dashboardVideoMetrics {
	view := dashboardVideoMetrics{}
	if s.Growth == nil {
		view.State = "Growth chưa sẵn sàng; chưa đọc được metrics video."
		return view
	}
	var weightedViews float64
	var latestAt time.Time
	for _, account := range accounts {
		if !hasYouTube(account) {
			continue
		}
		view.ChannelCount++
		if !strings.HasPrefix(strings.TrimSpace(account.YoutubeChannel), "UC") {
			continue
		}
		view.AnalyticsChannelCount++
		summary, err := s.Growth.RecentVideoMetrics(ctx, account.ID, time.Now().Add(-7*24*time.Hour))
		if err != nil {
			view.FailedChannels++
			continue
		}
		if summary.VideoCount == 0 || summary.AverageViews == nil {
			continue
		}
		view.DataChannelCount++
		view.VideoCount += summary.VideoCount
		weightedViews += *summary.AverageViews * float64(summary.VideoCount)
		if summary.LatestAt.After(latestAt) {
			latestAt = summary.LatestAt
		}
	}
	if view.ChannelCount == 0 {
		view.State = "Chưa có kênh YouTube để đọc metrics video."
		view.Detail = "TikTok, Instagram, Facebook và Threads chưa có nguồn metrics video-level trong dashboard."
		return view
	}
	if view.AnalyticsChannelCount == 0 {
		view.State = "Chưa có channel ID YouTube phù hợp cho Analytics."
		view.Detail = "YouTube Analytics cần channel ID bắt đầu bằng UC; account chỉ có handle chưa đủ."
		return view
	}
	if view.VideoCount == 0 {
		view.State = "Chưa có metrics video được đồng bộ trong 7 ngày gần đây."
		view.Detail = "Không có dữ liệu không đồng nghĩa 0 lượt xem. Kiểm tra OAuth Analytics và đồng bộ tại Growth."
		if view.FailedChannels > 0 {
			view.State = "Không đọc được metrics từ một số kênh YouTube."
		}
		return view
	}
	view.HasMetrics = true
	view.AverageViews = dashboardCountLabel(int64(math.Round(weightedViews / float64(view.VideoCount))))
	view.LatestAt = latestAt.UTC().Format("02/01/2006 15:04")
	view.Detail = "Views trong báo cáo YouTube Analytics 30 ngày; snapshot được lưu trong 7 ngày gần đây."
	if view.FailedChannels > 0 {
		view.Detail += " Một số kênh chưa đọc được; tổng hợp chỉ gồm kênh có dữ liệu."
	}
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

package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"log"
	"math"
	"time"
)

func (s *Service) ATOrderSyncTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyATOrderSyncEnabled, true) {
		return nil
	}
	cl, ok := s.atClient()
	if !ok {
		log.Printf("accesstrade order sync: chưa có access_key — bỏ qua")
		return nil
	}
	interval := time.Duration(atInt(s.Settings, KeyATOrderSyncIntervalM, defaultATOrderSyncMins)) * time.Minute
	if interval <= 0 {
		interval = defaultATOrderSyncMins * time.Minute
	}
	if !dueSince(s.Settings, keyATOrderSyncLastRun, interval) {
		return nil
	}
	until := time.Now()
	since := until.Add(-orderSyncWindowDays * 24 * time.Hour)
	orders, truncated, err := cl.ListOrders(ctx, since, until)
	if err != nil {
		return []string{"đồng bộ đơn thất bại: " + err.Error()}
	}
	if truncated {
		s.alert("at_orders", "warn",
			"Đồng bộ đơn Accesstrade bị cắt ở 10 trang (2000 đơn) — một số đơn có thể chưa về. Thu hẹp khoảng đồng bộ hoặc kiểm tra sau đợt sale lớn.",
			"đã log cảnh báo, chu kỳ sau đồng bộ tiếp")
	}
	added, updated, uerr := s.AT.UpsertOrders(orders)
	if uerr != nil {
		return []string{"lưu đơn thất bại: " + uerr.Error()}
	}
	_ = s.AT.SetSyncState(keyATOrdersUntil, until.UTC().Format(time.RFC3339))
	stampRun(s.Settings, keyATOrderSyncLastRun)
	st, _ := s.AT.GetOrderStats()
	return []string{fmt.Sprintf(
		"đồng bộ đơn: %d đơn (%d mới, %d cập nhật) — đã duyệt %d (%s₫), chờ duyệt %d",
		len(orders), added, updated,
		st.ApprovedCount, accesstrade.Thousands(int64(math.Round(st.ApprovedTotal))), st.PendingCount)}
}

// ----------------------------------------------------- campaign check tick

// ATCampaignCheckTick daily: làm mới cache campaign, campaign nào chưa
// duyệt (pending/unregistered) → ghi decision log làm alert.
func (s *Service) ATCampaignCheckTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyATCampaignCheckOn, true) {
		return nil
	}
	cl, ok := s.atClient()
	if !ok {
		log.Printf("accesstrade campaign check: chưa có access_key — bỏ qua")
		return nil
	}
	if !dueSince(s.Settings, keyATCampaignCheckLast,
		defaultATCampaignHours*time.Hour) {
		return nil
	}
	// API chỉ lọc được approval=successful (đo thật 12/10/2026); danh
	// sách công khai ~10.000 chiến dịch chưa đăng ký — không phải "chờ
	// duyệt" nên không cảnh báo theo đó.
	cs, err := cl.ListCampaigns(ctx, accesstrade.CampaignFilter{Approval: "successful"})
	if err != nil {
		return []string{"kiểm tra chiến dịch thất bại: " + err.Error()}
	}
	if uerr := s.AT.UpsertCampaigns(cs); uerr != nil {
		log.Printf("accesstrade: lưu cache campaign: %v", uerr)
	}
	stampRun(s.Settings, keyATCampaignCheckLast)
	if len(cs) == 0 {
		_ = s.decide("at_hunter", "campaign_pending", nil,
			"Chưa có chiến dịch Accesstrade nào được duyệt — đăng ký chiến dịch trên pub.accesstrade.vn", nil)
		return []string{"cảnh báo: chưa có chiến dịch nào được duyệt"}
	}
	return []string{fmt.Sprintf("kiểm tra chiến dịch: %d chiến dịch đã duyệt", len(cs))}
}

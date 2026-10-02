package web

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// commissionStateSetting tracks the last reconcile outcome so the honest
// fail-closed decision is written once per state change — not on every
// hourly tick.
const commissionStateSetting = "commission.source_state"

// affiliateOrdersSource is the commission-data boundary for money
// reconciliation. The real *tiktok.ShopClient satisfies it once its
// endpoint paths are verified in the Partner Center sandbox; tests inject
// a fake. Payload shape mirrors the TikTok Shop affiliate orders API.
type affiliateOrdersSource interface {
	AffiliateOrders(startTS, endTS int64, page, pageSize int) (map[string]any, error)
}

// commissionSource returns the real affiliate-orders feed when one can
// actually flow. Credentials missing OR endpoint path unverified both mean
// fail-closed: no fabricated money, ever.
func (s *Server) commissionSource() (affiliateOrdersSource, string) {
	if s.CommissionSource != nil {
		return s.CommissionSource, ""
	}
	if s.Cfg == nil {
		return nil, "chưa có cấu hình"
	}
	if s.Cfg.TiktokShopAppKey == "" || s.Cfg.TiktokShopAppSecret == "" ||
		s.Cfg.TiktokShopAccessToken == "" {
		return nil, "chưa kết nối TikTok Shop (thiếu app key/secret/access token trong Cài đặt)"
	}
	if tiktok.Endpoints["affiliate_orders_search"] == "" {
		return nil, "endpoint affiliate_orders_search chưa được xác thực " +
			"trong Partner Center sandbox — app không đoán URL"
	}
	return &tiktok.ShopClient{
		AppKey:      s.Cfg.TiktokShopAppKey,
		AppSecret:   s.Cfg.TiktokShopAppSecret,
		AccessToken: s.Cfg.TiktokShopAccessToken,
		ShopCipher:  s.Cfg.TiktokShopCipher,
	}, ""
}

// noteCommissionState records the fail-closed reason in the decision log
// once per state change (not on every tick).
func (s *Server) noteCommissionState(state, reason string) {
	if s.growthSetting(commissionStateSetting) == state {
		return
	}
	s.setGrowthSetting(commissionStateSetting, state)
	_ = s.Ledger.Decide("system", "commission_reconcile", nil, reason,
		map[string]any{"state": state})
}

// reconcileCommissions pulls real affiliate orders (last 24h) into the
// append-only ledger and records newly-seen commission as one settlement
// row — feeding the "Doanh thu affiliate" and "Hoa hồng đã ghi nhận" cards
// on the dashboard. FAIL-CLOSED: without a verified source nothing is
// written; the cards stay 0 and the honest reason lands in the decision
// log. Idempotent: re-running the same orders writes nothing twice.
func (s *Server) reconcileCommissions(_ context.Context) []string {
	if s.Ledger == nil {
		return nil
	}
	src, why := s.commissionSource()
	if src == nil {
		s.noteCommissionState("no_source",
			"Không đối soát được hoa hồng: "+why+". Số tiền giữ 0 — không tự ghi số giả.")
		return nil
	}
	now := time.Now()
	data, err := src.AffiliateOrders(now.Add(-24*time.Hour).Unix(), now.Unix(), 1, 50)
	if err != nil {
		s.noteCommissionState("error",
			"Đối soát hoa hồng lỗi: "+err.Error()+" — giữ số cũ, không ghi thêm.")
		return nil
	}
	newOrders, newCommission := 0, 0.0
	for _, o := range affiliateOrderList(data) {
		oid := affiliateOrderStr(o, "order_id", "id")
		if oid == "" {
			continue
		}
		comm := affiliateOrderFloat(o, "commission", "commission_amount",
			"estimated_commission")
		orderedAt := affiliateOrderStr(o, "ordered_at", "create_time")
		if orderedAt == "" {
			orderedAt = s.nowISO()
		}
		rid, err := s.Ledger.RecordOrder(oid, map[string]any{
			"amount":     affiliateOrderFloat(o, "amount"),
			"commission": comm,
			"ordered_at": orderedAt,
		})
		if err != nil {
			s.noteCommissionState("error",
				"Đối soát hoa hồng lỗi ghi sổ: "+err.Error())
			return nil
		}
		if rid != 0 {
			newOrders++
			newCommission += comm
		}
	}
	if newCommission > 0 {
		if _, err := s.Ledger.RecordCommission(now.Format("2006-01"),
			newCommission, "tiktok_shop_api"); err != nil {
			log.Printf("reconcile: record commission: %v", err)
		}
	}
	if s.growthSetting(commissionStateSetting) != "ok" {
		s.setGrowthSetting(commissionStateSetting, "ok")
	}
	if newOrders == 0 {
		return nil
	}
	return []string{fmt.Sprintf("Đối soát tiền: +%d đơn affiliate mới, %s hoa hồng đã ghi vào sổ.",
		newOrders, formatVND(newCommission))}
}

// ---------------------------------------------------------------- payload

// affiliateOrderList extracts order maps from the API payload; unknown
// shapes are skipped rather than misread (mirrors analyst.orderList, which
// lives in the parked agent universe — R2-W6 will resolve the duplication).
func affiliateOrderList(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	items, ok := data["orders"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// affiliateOrderStr reads the first present string key (keys are tried in
// order because the sandbox docs and production payload disagree on names).
func affiliateOrderStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// affiliateOrderFloat reads the first present numeric key.
func affiliateOrderFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch n := m[k].(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}

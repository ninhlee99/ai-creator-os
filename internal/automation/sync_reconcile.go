package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
)

// ------------------------------------------------------- sync + decide

// SyncOneAccount pulls fresh metrics for one account from every wired
// source, persists what came back, and runs the growth decision loop.
// It returns honest status lines (Vietnamese) for the UI — a source that
// is not connected is reported as such, never silently skipped.
//
// The governance thresholds are the UI-tuned ones (LoadThresholds),
// never the hard-coded growth.DefaultConfig() — R2-W4 (R2-05): mọi knob
// vận hành phải chỉnh được, không hardcode trong code.
func (s *Service) SyncOneAccount(ctx context.Context, a *network.Account) []string {
	if s.Growth == nil {
		return []string{a.Username + ": module growth chưa sẵn sàng"}
	}
	var notes []string
	if _, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a)); err != nil {
		return append(notes, a.Username+": "+err.Error())
	}
	srcAcct := growth.SourceAccount{ID: a.ID, Username: a.Username, YoutubeChannel: a.YoutubeChannel}
	ytKeyVal := s.env("YOUTUBE_API_KEY")
	sources := []growth.MetricsSource{
		growth.NewYouTubeSource(ytKeyVal),
		growth.TikTokSource{},
	}
	gotSnapshot := false
	for _, src := range sources {
		snap, err := growth.RecordSnapshot(ctx, s.Growth, src, srcAcct)
		if err != nil {
			notes = append(notes, a.Username+": "+err.Error())
			continue
		}
		gotSnapshot = true
		notes = append(notes, fmt.Sprintf("%s: đã đồng bộ số liệu từ %s", a.Username, src.Name()))
		if snap.Followers != nil {
			_ = s.Ledger.SetFollowers(a.ID, *snap.Followers)
			cur := map[string]float64{"followers": float64(*snap.Followers), "subs": float64(*snap.Followers)}
			if reached, err := s.Growth.MarkTargetsReached(a.ID, cur); err == nil {
				for _, label := range reached {
					target := a.Username
					_ = s.decide("growth_analyst", "milestone_reached", &target,
						"Đạt mốc "+label, nil)
					notes = append(notes, a.Username+": đạt mốc "+label)
				}
			}
		}
	}
	if !gotSnapshot {
		return notes // nothing real came back; decide nothing on stale air
	}
	res, err := s.Growth.Evaluate(a.ID, a.Username, LoadThresholds(s.Settings), time.Now())
	if err != nil {
		return append(notes, a.Username+": lỗi đánh giá growth — "+err.Error())
	}
	for _, act := range res.Actions {
		notes = append(notes, a.Username+": "+act)
	}
	if res.PauseAccount {
		if _, err := s.Accounts.Transition(a.ID, "paused", map[string]any{
			"reason": "growth: " + res.PauseReason,
		}); err != nil {
			notes = append(notes, a.Username+": growth đề xuất tạm dừng nhưng không chuyển được trạng thái — "+err.Error())
		} else {
			notes = append(notes, a.Username+": ĐÃ TỰ TẠM DỪNG — "+res.PauseReason)
		}
	}
	return notes
}

// env resolves one variable through the EnvProvider (nil-safe).
func (s *Service) env(name string) string {
	if s.Env == nil {
		return ""
	}
	v, _ := s.Env.EffectiveEnv(name)
	return v
}

// ------------------------------------------------- money reconciliation

// commissionStateSetting tracks the last reconcile outcome so the honest
// fail-closed decision is written once per state change — not on every
// hourly tick.
const commissionStateSetting = "commission.source_state"

// commissionSource returns the real affiliate-orders feed when one can
// actually flow. Fail-closed: no fabricated money, ever.
//
// Đợt H1 (2026-10-04): TikTok Shop đã loại bỏ hoàn toàn (Accesstrade-only
// theo quyết định của Ninh). Đối soát hoa hồng chạy qua Accesstrade
// (ATOrderSyncTick → /at/orders/sync); nguồn legacy này luôn fail-closed.
// CommissionSource vẫn là điểm tiêm cho test.
func (s *Service) commissionSource() (OrdersSource, string) {
	if s.CommissionSource != nil {
		return s.CommissionSource, ""
	}
	return nil, "TikTok Shop đã loại bỏ (Accesstrade-only từ Đợt H1) — " +
		"đối soát hoa hồng chạy qua Accesstrade: /at/orders/sync"
}

// noteCommissionState records the current source state only when it
// changes, so the decision log isn't flooded by hourly repeats. The
// decision action is "commission_reconcile" (kept from the web era).
func (s *Service) noteCommissionState(status, msg string) {
	if s.Settings == nil {
		return
	}
	prev, _ := s.Settings.Get(commissionStateSetting)
	if prev == status {
		return
	}
	_ = s.Settings.Set(commissionStateSetting, status)
	_ = s.Settings.Set("commission.note", msg)
	target := "reconcile"
	_ = s.decide("system", "commission_reconcile", &target, msg,
		map[string]any{"source_state": status})
}

// ReconcileCommissions pulls real affiliate orders (never synthetic) and
// reconciles them into the ledger: RecordOrder is idempotent per
// external_id, commissions land in chronological order. Rides the hourly
// growth tick; fail-closed when no source is connected.
func (s *Service) ReconcileCommissions(ctx context.Context) []string {
	if s.Ledger == nil {
		return []string{"Đối soát hoa hồng: kho ledger chưa sẵn sàng"}
	}
	src, reason := s.commissionSource()
	if src == nil {
		s.noteCommissionState("no_source", reason)
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
			newCommission, "orders_api"); err != nil {
			s.noteCommissionState("error",
				"Đối soát hoa hồng lỗi ghi sổ: "+err.Error())
			return nil
		}
	}
	// State flips silently on an empty success — an hourly tick with no
	// new orders is not a decision (kept from the web era).
	if s.Settings != nil {
		if prev, _ := s.Settings.Get(commissionStateSetting); prev != "ok" {
			_ = s.Settings.Set(commissionStateSetting, "ok")
		}
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
		switch v := m[k].(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// formatVND renders v as a Vietnamese dong amount: 1234500 → "1.234.500 ₫".
func formatVND(v float64) string {
	n := int64(v + 0.5)
	if v < 0 {
		n = int64(v - 0.5)
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String() + " ₫"
	}
	return b.String() + " ₫"
}

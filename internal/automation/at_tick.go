package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Accesstrade automation ticks (Đợt C) — zero-touch, Ninh không chạm.
//
// Ba tick, cùng một cổng an toàn như AutopilotTick: kill switch + DRY-RUN
// chặn mọi tick; chưa có access_key → bỏ qua im lặng + log (không spam
// alert). Cadence và công tắc đều chỉnh từ UI (ledger settings).
//
//   - ATHunterTick: daily — quét datafeed sort hoa hồng cao → products.Store
//     → top N sản phẩm mới tạo studio job affiliate (ảnh + nhạc).
//   - ATOrderSyncTick: 30 phút — order-list (10 req/phút nội bộ) → upsert +
//     update pending tại chỗ → đối soát.
//   - ATCampaignCheckTick: daily — phát hiện campaign chưa duyệt → ghi
//     decision log (alert).
// ---------------------------------------------------------------------------

// Setting keys cho Accesstrade automation (ledger settings; unset = mặc
// định zero-touch BẬT — tiền lệ Đợt 3).
const (
	KeyATHunterEnabled      = "at.hunter_enabled"
	KeyATHunterIntervalHrs  = "at.hunter_interval_hours"
	KeyATHunterVideos       = "at.hunter_videos"
	KeyATHunterAccountID    = "at.hunter_account_id"
	KeyATHunterFlowCredits  = "at.hunter_flow_credits"
	KeyATOrderSyncEnabled   = "at.ordersync_enabled"
	KeyATOrderSyncIntervalM = "at.ordersync_interval_minutes"
	KeyATCampaignCheckOn    = "at.campaigncheck_enabled"
	keyATHunterLastRun      = "at.hunter_last_run"
	keyATOrderSyncLastRun   = "at.ordersync_last_run"
	keyATCampaignCheckLast  = "at.campaigncheck_last_run"
	keyATOrdersUntil        = "orders.until"
	defaultATHunterVideos   = 3
	defaultATHunterHours    = 24
	defaultATOrderSyncMins  = 30
	defaultATCampaignHours  = 24
	orderSyncWindowDays     = 7 // cửa sổ quét rolling: bắt kịp đơn pending→approved
)

// StudioRunner là biên render affiliate mà tick AT cần.
// *studio.Studio thỏa mãn; test inject fake.
type StudioRunner interface {
	CreateAffiliateJob(p studio.AffiliateParams) (string, error)
}

// ATTick chạy mọi pass Accesstrade đến hạn. main.go gọi mỗi 5 phút.
func (s *Service) ATTick(ctx context.Context) []string {
	var notes []string
	notes = append(notes, s.ATHunterTick(ctx)...)
	notes = append(notes, s.ATOrderSyncTick(ctx)...)
	notes = append(notes, s.ATCampaignCheckTick(ctx)...)
	return notes
}

// atClient dựng client từ key đang lưu; ok=false khi thiếu key/store
// (tick bỏ qua im lặng, không spam alert — đúng yêu cầu).
func (s *Service) atClient() (*accesstrade.Client, bool) {
	if s.AT == nil || s.Settings == nil {
		return nil, false
	}
	key, ok := s.Settings.Get(accesstrade.KeySetting)
	if !ok || strings.TrimSpace(key) == "" {
		return nil, false
	}
	c := accesstrade.NewClient(strings.TrimSpace(key))
	if s.ATBaseURL != "" {
		c.BaseURL = s.ATBaseURL // test hook
	}
	return c, true
}

// atOn đọc công tắc UI; unset = def (zero-touch mặc định bật).
func atOn(st Settings, key string, def bool) bool {
	if st == nil {
		return def
	}
	v, ok := st.Get(key)
	if !ok {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
}

// atInt đọc số nguyên từ settings; sai/thiếu → def.
func atInt(st Settings, key string, def int) int {
	if st == nil {
		return def
	}
	if v, ok := st.Get(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// HunterAccountID requires an explicit channel target; a global/default face
// photo cannot safely stand in for a channel identity.
func (s *Service) HunterAccountID() (int64, error) {
	if s.Settings == nil {
		return 0, fmt.Errorf("chưa cấu hình kênh đích cho video hunter")
	}
	raw, ok := s.Settings.Get(KeyATHunterAccountID)
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("chưa chọn kênh đích cho video hunter")
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || accountID <= 0 {
		return 0, fmt.Errorf("kênh đích hunter không hợp lệ")
	}
	if s.Accounts == nil {
		return 0, fmt.Errorf("không thể xác minh kênh đích hunter")
	}
	account, err := s.Accounts.Get(accountID)
	if err != nil || account == nil {
		return 0, fmt.Errorf("không tìm thấy kênh đích hunter")
	}
	return accountID, nil
}

// dueSince kiểm tra watermark lastRun đã quá interval chưa.
func dueSince(st Settings, lastKey string, interval time.Duration) bool {
	if st == nil {
		return true
	}
	v, ok := st.Get(lastKey)
	if !ok || v == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return true
	}
	return time.Since(t) >= interval
}

func stampRun(st Settings, lastKey string) {
	if st == nil {
		return
	}
	_ = st.Set(lastKey, time.Now().Format(time.RFC3339))
}

// ------------------------------------------------------------ hunter tick

// ATHunterTick quét datafeed → kho sản phẩm → top N mới → video affiliate.
// Gate: kill switch + DRY-RUN (tạo video đốt quota Gemini nên phải qua cổng).

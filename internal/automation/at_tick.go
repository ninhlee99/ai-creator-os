package automation

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
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
	KeyATHunterEnabled       = "at.hunter_enabled"
	KeyATHunterIntervalHrs   = "at.hunter_interval_hours"
	KeyATHunterVideos        = "at.hunter_videos"
	KeyATOrderSyncEnabled    = "at.ordersync_enabled"
	KeyATOrderSyncIntervalM  = "at.ordersync_interval_minutes"
	KeyATCampaignCheckOn     = "at.campaigncheck_enabled"
	keyATHunterLastRun       = "at.hunter_last_run"
	keyATOrderSyncLastRun    = "at.ordersync_last_run"
	keyATCampaignCheckLast   = "at.campaigncheck_last_run"
	keyATOrdersUntil         = "orders.until"
	defaultATHunterVideos    = 3
	defaultATHunterHours     = 24
	defaultATOrderSyncMins   = 30
	defaultATCampaignHours   = 24
	orderSyncWindowDays      = 7 // cửa sổ quét rolling: bắt kịp đơn pending→approved
	hunterDirectAccountID    = 0 // usage account 0 = video trực tiếp từ hunter
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
func (s *Service) ATHunterTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyATHunterEnabled, true) {
		return nil
	}
	cl, ok := s.atClient()
	if !ok {
		log.Printf("accesstrade hunter: chưa có access_key — bỏ qua")
		return nil
	}
	if s.Products == nil {
		return nil
	}
	interval := time.Duration(atInt(s.Settings, KeyATHunterIntervalHrs, defaultATHunterHours)) * time.Hour
	if interval <= 0 {
		interval = defaultATHunterHours * time.Hour
	}
	if !dueSince(s.Settings, keyATHunterLastRun, interval) {
		return nil
	}
	res, err := cl.Hunt(ctx, s.Products, accesstrade.HunterFilter{})
	if err != nil {
		return []string{"săn sản phẩm thất bại: " + err.Error()}
	}
	videos := 0
	if s.StudioRunner != nil {
		want := atInt(s.Settings, KeyATHunterVideos, defaultATHunterVideos)
		for _, p := range res.Products {
			if videos >= want {
				break
			}
			jobID, jerr := s.MakeHunterVideo(ctx, p)
			if jerr != nil {
				log.Printf("accesstrade hunter: tạo video cho %q thất bại: %v", p.Title, jerr)
				continue
			}
			_ = s.Products.RecordUse(p.ID, hunterDirectAccountID, jobID)
			videos++
		}
	}
	stampRun(s.Settings, keyATHunterLastRun)
	return []string{fmt.Sprintf("săn sản phẩm: %d mới, %d cập nhật, %d bỏ qua → %d video",
		res.New, res.Updated, res.Skipped, videos)}
}

// MakeHunterVideo tạo 1 studio job affiliate từ sản phẩm hunter:
// ảnh listing thật (product lock) + nhạc nền, không chữ không voiceover
// (đúng format Ninh chốt). Không viết lại renderer — gọi CreateAffiliateJob.
// Export để web handler "Săn sản phẩm" dùng chung logic với tick.
func (s *Service) MakeHunterVideo(ctx context.Context, p products.Product) (string, error) {
	if len(p.ImageURLs) == 0 {
		return "", fmt.Errorf("sản phẩm %q không có ảnh", p.Title)
	}
	work := s.ATWorkDir
	if work == "" {
		work = filepath.Join(os.TempDir(), "aicos-at-hunter")
	}
	dir := filepath.Join(work, fmt.Sprint(p.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "prod.jpg")
	if err := studio.DownloadImage(ctx, p.ImageURLs[0], dst); err != nil {
		return "", fmt.Errorf("tải ảnh: %w", err)
	}
	niche := p.Category
	if niche == "" {
		niche = p.Theme
	}
	return s.StudioRunner.CreateAffiliateJob(studio.AffiliateParams{
		Mode:         studio.AffiliateModePhoto,
		Niche:        niche,
		ProductName:  p.Title,
		ProductPhoto: dst,
		Seconds:      30,
		MusicPath:    s.ATMusicPath, // "" = video câm (job log ghi rõ)
	})
}

// -------------------------------------------------------- order sync tick

// ATOrderSyncTick đồng bộ đơn 30 phút/lần: order-list trong cửa sổ rolling
// 7 ngày → upsert + UPDATE pending tại chỗ → ghi watermark.
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
	orders, err := cl.ListOrders(ctx, since, until)
	if err != nil {
		return []string{"đồng bộ đơn thất bại: " + err.Error()}
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
		st.ApprovedCount, accesstrade.Thousands(int64(st.ApprovedTotal)), st.PendingCount)}
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
	cs, err := cl.ListCampaigns(ctx, accesstrade.CampaignFilter{})
	if err != nil {
		return []string{"kiểm tra chiến dịch thất bại: " + err.Error()}
	}
	if uerr := s.AT.UpsertCampaigns(cs); uerr != nil {
		log.Printf("accesstrade: lưu cache campaign: %v", uerr)
	}
	var pending []string
	for _, c := range cs {
		if c.Approval != "successful" {
			pending = append(pending, c.Name+" ("+c.ApprovalLabel()+")")
		}
	}
	stampRun(s.Settings, keyATCampaignCheckLast)
	if len(pending) > 0 {
		target := strings.Join(pending, "; ")
		_ = s.decide("at_hunter", "campaign_pending", &target,
			fmt.Sprintf("%d chiến dịch Accesstrade chưa duyệt — đăng ký/duyệt tay trên pub.accesstrade.vn", len(pending)), nil)
		return []string{fmt.Sprintf("cảnh báo: %d chiến dịch chưa duyệt: %s",
			len(pending), strings.Join(pending, ", "))}
	}
	return []string{fmt.Sprintf("kiểm tra chiến dịch: %d chiến dịch đều đã duyệt", len(cs))}
}

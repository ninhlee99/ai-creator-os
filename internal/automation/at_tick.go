package automation

import (
	"context"
	"fmt"
	"log"
	"math"
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
	KeyATHunterEnabled      = "at.hunter_enabled"
	KeyATHunterIntervalHrs  = "at.hunter_interval_hours"
	KeyATHunterVideos       = "at.hunter_videos"
	KeyATHunterAccountID    = "at.hunter_account_id"
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
	res, err := cl.Hunt(ctx, s.Products, accesstrade.HunterFilter{ApprovedOnly: true, MaxAgeDays: 180})
	if err != nil {
		return []string{"săn sản phẩm thất bại: " + err.Error()}
	}
	if res.Note != "" && res.New == 0 {
		stampRun(s.Settings, keyATHunterLastRun)
		_ = s.decide("at_hunter", "hunt_blocked", nil, res.Note, nil)
		return []string{"săn sản phẩm: " + res.Note}
	}
	videos := 0
	want := atInt(s.Settings, KeyATHunterVideos, defaultATHunterVideos)
	if s.StudioRunner != nil && want > 0 {
		if readiness, ok := s.StudioRunner.(interface{ AutoAffiliateProductionReady() (bool, string) }); ok {
			ready, reason := readiness.AutoAffiliateProductionReady()
			if !ready {
				stampRun(s.Settings, keyATHunterLastRun)
				return []string{fmt.Sprintf("săn sản phẩm: %d mới, %d cập nhật, %d bỏ qua → 0 video; %s", res.New, res.Updated, res.Skipped, reason)}
			}
		}
	}
	var accountID int64
	var accountErr error
	if s.StudioRunner != nil && want > 0 {
		accountID, accountErr = s.HunterAccountID()
	}
	if s.StudioRunner != nil && want > 0 && accountErr == nil {
		for _, p := range res.Products {
			if videos >= want {
				break
			}
			// Đợt Y: claim trước khi dựng video (như Autopilot.Run) —
			// nút "Săn ngay" bấm tay có thể đua với tick định kỳ.
			claimed, cerr := s.Products.RecordUse(p.ID, accountID, "")
			if cerr != nil || !claimed {
				continue
			}
			jobID, jerr := s.MakeHunterVideoForAccount(ctx, p, accountID)
			if jerr != nil {
				_ = s.Products.ReleaseUseClaim(p.ID, accountID)
				log.Printf("accesstrade hunter: tạo video cho %q thất bại: %v", p.Title, jerr)
				continue
			}
			_ = s.Products.SetUsageJobID(p.ID, accountID, jobID)
			videos++
		}
	}
	stampRun(s.Settings, keyATHunterLastRun)
	note := fmt.Sprintf("săn sản phẩm: %d mới, %d cập nhật, %d bỏ qua → %d video",
		res.New, res.Updated, res.Skipped, videos)
	if accountErr != nil && s.StudioRunner != nil && want > 0 {
		note += "; chưa tạo video: " + accountErr.Error()
	}
	return []string{note}
}

// MakeHunterVideo tạo 1 studio job affiliate từ sản phẩm hunter:
// ảnh listing thật (product lock) + nhạc nền, không chữ không voiceover
// (đúng format Ninh chốt). Không viết lại renderer — gọi CreateAffiliateJob.
// Export để web handler "Săn sản phẩm" dùng chung logic với tick.
func (s *Service) MakeHunterVideo(ctx context.Context, p products.Product) (string, error) {
	accountID, err := s.HunterAccountID()
	if err != nil {
		return "", err
	}
	return s.MakeHunterVideoForAccount(ctx, p, accountID)
}

// MakeHunterVideoForAccount renders with identity photos owned by accountID.
func (s *Service) MakeHunterVideoForAccount(ctx context.Context, p products.Product, accountID int64) (string, error) {
	if accountID <= 0 {
		return "", fmt.Errorf("cần chọn kênh đích cho video hunter")
	}
	if s.StudioRunner == nil {
		return "", fmt.Errorf("Studio chưa sẵn sàng")
	}
	if readiness, ok := s.StudioRunner.(interface{ AutoAffiliateProductionReady() (bool, string) }); ok {
		ready, reason := readiness.AutoAffiliateProductionReady()
		if !ready {
			return "", fmt.Errorf("%s", reason)
		}
	}
	modelSource, ok := s.StudioRunner.(interface{ ModelPhotosForAccount(int64) []string })
	if !ok {
		return "", fmt.Errorf("Studio chưa hỗ trợ thư viện nhân vật theo kênh")
	}
	modelPhotos := modelSource.ModelPhotosForAccount(accountID)
	if len(modelPhotos) == 0 {
		return "", fmt.Errorf("kênh %d chưa có ảnh nhân vật riêng", accountID)
	}
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
	// Ảnh listing thêm (góc khác) → video nhiều ảnh thật, không lặp 1 ảnh.
	var extra []string
	for i, u := range p.ImageURLs[1:] {
		if len(extra) >= 5 {
			break
		}
		e := filepath.Join(dir, fmt.Sprintf("prod-%d.jpg", i+1))
		if studio.DownloadImage(ctx, u, e) == nil {
			extra = append(extra, e)
		}
	}
	niche := p.Category
	if niche == "" {
		niche = p.Theme
	}
	return s.StudioRunner.CreateAffiliateJob(studio.AffiliateParams{
		Mode:         studio.AffiliateModeShots,
		AccountID:    accountID,
		Niche:        niche,
		ProductName:  p.Title,
		ModelPhoto:   modelPhotos[0],
		ModelPhotos:  modelPhotos,
		ProductPhoto: dst,
		ExtraPhotos:  extra,
		Seconds:      30,
		MusicPath:    s.ATMusicPath, // "" = video câm (job log ghi rõ)
		AffLink:      p.ProductURL,  // đợt Q: link Accesstrade vào mô tả YouTube
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
